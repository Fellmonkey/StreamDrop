//go:build windows

package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"streamdrop/internal/security"
)

// ChromiumCookieHeader extracts the Cookie header for a host from a Chromium
// profile.
//
// Supported schemes:
//   - Chromium below 127: AES-256-GCM with a key stored in Local State and
//     unwrapped with DPAPI (this is what the app implements).
//   - Chromium 127+: App-Bound Encryption. The key is wrapped for the browser's
//     elevation service and cannot be unwrapped by a third-party process, so
//     the function returns ErrUnsupported and the UI offers cookies.txt or a
//     manual Cookie header instead.
func ChromiumCookieHeader(profileDir string) (string, error) {
	return chromiumCookieHeader(profileDir, "")
}

// ChromiumCookieHeaderFor extracts cookies for one host.
func ChromiumCookieHeaderFor(profileDir, host string) (string, error) {
	return chromiumCookieHeader(profileDir, host)
}

func chromiumCookieHeader(profileDir, host string) (string, error) {
	if profileDir == "" {
		return "", ErrUnsupported
	}
	// Local State lives in the user-data root; Opera keeps it one level up.
	statePath := filepath.Join(profileDir, "Local State")
	if _, err := os.Stat(statePath); err != nil {
		statePath = filepath.Join(filepath.Dir(profileDir), "Local State")
	}
	raw, err := os.ReadFile(statePath)
	if err != nil {
		return "", ErrUnsupported
	}
	var state localState
	if err := json.Unmarshal(raw, &state); err != nil {
		return "", ErrUnsupported
	}
	if state.OSCrypt.AppBoundEncrypted != "" || state.OSCrypt.EncryptedKey == "" {
		return "", ErrUnsupported
	}
	keyBlob, err := base64.StdEncoding.DecodeString(state.OSCrypt.EncryptedKey)
	if err != nil {
		return "", ErrUnsupported
	}
	// The blob starts with the ASCII marker "DPAPI".
	if len(keyBlob) <= 5 || string(keyBlob[:5]) != "DPAPI" {
		return "", ErrUnsupported
	}
	key, err := security.New().Decrypt(keyBlob[5:])
	if err != nil {
		return "", fmt.Errorf("chromium: unwrap key: %w", err)
	}

	cookiesPath := filepath.Join(profileDir, "Cookies")
	if _, err := os.Stat(cookiesPath); err != nil {
		cookiesPath = filepath.Join(profileDir, "Network", "Cookies")
	}
	db, cleanup, err := openReadOnlyCopy(cookiesPath)
	if err != nil {
		return "", err
	}
	defer cleanup()

	query := `SELECT host_key, name, value, encrypted_value FROM cookies`
	args := []any{}
	if host != "" {
		query += ` WHERE host_key LIKE ?`
		args = append(args, "%"+host+"%")
	}
	query += ` ORDER BY host_key, name`

	rows, err := db.Query(query, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var parts []string
	for rows.Next() {
		var hostKey, name, value string
		var enc []byte
		if err := rows.Scan(&hostKey, &name, &value, &enc); err != nil {
			return "", err
		}
		if host != "" && !hostMatches(hostKey, host) && !hostMatches(host, strings.TrimPrefix(hostKey, ".")) {
			continue
		}
		if value == "" && len(enc) > 0 {
			dec, err := decryptChromiumValue(enc, key)
			if err != nil {
				continue
			}
			value = dec
		}
		if name == "" {
			continue
		}
		parts = append(parts, name+"="+value)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(parts) == 0 {
		return "", ErrUnsupported
	}
	return strings.Join(parts, "; "), nil
}

// decryptChromiumValue decrypts a v10/v11 cookie value (AES-256-GCM).
func decryptChromiumValue(encrypted, key []byte) (string, error) {
	if len(encrypted) < 3 {
		return "", errors.New("chromium: short ciphertext")
	}
	prefix := string(encrypted[:3])
	if prefix != "v10" && prefix != "v11" {
		return "", ErrUnsupported
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	payload := encrypted[3:]
	if len(payload) < gcm.NonceSize() {
		return "", errors.New("chromium: bad payload")
	}
	nonce, ciphertext := payload[:gcm.NonceSize()], payload[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// openReadOnlyCopy copies the SQLite file (and its WAL) to a temp directory so
// a running browser does not block reading a locked database.
func openReadOnlyCopy(path string) (*sql.DB, func(), error) {
	if _, err := os.Stat(path); err != nil {
		return nil, func() {}, ErrUnsupported
	}
	tmp, err := os.MkdirTemp("", "streamdrop-cookies-")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }

	dest := filepath.Join(tmp, "Cookies")
	if err := copyFile(path, dest); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(path + suffix); err == nil {
			_ = copyFile(path+suffix, dest+suffix)
		}
	}
	db, err := sql.Open("sqlite", dest+"?mode=ro")
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return db, func() { db.Close(); cleanup() }, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
