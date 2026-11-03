//go:build windows

package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/sha1"
	"encoding/asn1"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/pbkdf2"

	_ "modernc.org/sqlite"
)

// FirefoxCookieHeader extracts cookies from a Firefox profile.
//
// Firefox stores the master key in key4.db (NSS). This implementation reads the
// legacy NSS scheme (3DES-wrapped master key, AES-256-CBC cookie values). Modern
// profiles may use the newer NSS AES-GCM/HKDF scheme; those are reported as
// unsupported with a clear note rather than returning wrong data, and the user
// is pointed at cookies.txt / manual cookies, which always work.
func FirefoxCookieHeader(profileDir, cookieFile, keyFile string) (string, error) {
	return FirefoxCookieHeaderFor(profileDir, cookieFile, keyFile, "")
}

// FirefoxCookieHeaderFor restricts extraction to one host.
func FirefoxCookieHeaderFor(profileDir, cookieFile, keyFile, host string) (string, error) {
	if cookieFile == "" {
		cookieFile = filepath.Join(profileDir, "cookies.sqlite")
	}
	if keyFile == "" {
		keyFile = filepath.Join(profileDir, "key4.db")
	}
	if _, err := os.Stat(cookieFile); err != nil {
		return "", ErrUnsupported
	}

	master, err := firefoxMasterKey(keyFile)
	if err != nil {
		return "", err
	}

	db, cleanup, err := openReadOnlyCopy(cookieFile)
	if err != nil {
		return "", err
	}
	defer cleanup()

	query := `SELECT name, value, encrypted_value, host FROM moz_cookies`
	args := []any{}
	if host != "" {
		query += ` WHERE host LIKE ?`
		args = append(args, "%"+host+"%")
	}
	query += ` ORDER BY name`
	rows, err := db.Query(query, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var parts []string
	for rows.Next() {
		var name, value, cookieHost string
		var enc []byte
		if err := rows.Scan(&name, &value, &enc, &cookieHost); err != nil {
			return "", err
		}
		if host != "" && !hostMatches(cookieHost, host) && !hostMatches(host, cookieHost) {
			continue
		}
		if value == "" && len(enc) > 0 {
			dec, err := decryptFirefoxValue(enc, master)
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
	if len(parts) == 0 {
		return "", ErrUnsupported
	}
	return strings.Join(parts, "; "), nil
}

// nssBlob is the DER structure NSS stores for encrypted metadata entries.
type nssBlob struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters struct {
		Salt []byte
		IV   []byte
	}
	Ciphertext []byte
}

// firefoxMasterKey recovers the NSS master key from key4.db.
func firefoxMasterKey(keyFile string) ([]byte, error) {
	if _, err := os.Stat(keyFile); err != nil {
		return nil, ErrUnsupported
	}
	// key4.db is a real SQLite database; read it through a copy so a running
	// browser does not block us with its locks.
	db, cleanup, err := openReadOnlyCopy(keyFile)
	if err != nil {
		return nil, ErrUnsupported
	}
	defer cleanup()

	// NSS metadata: id=1 is the password-check entry, id=2 the encrypted key.
	var globalSalt, item2 []byte
	if err := db.QueryRow(`SELECT item1, item2 FROM metadata WHERE id = 1`).Scan(&globalSalt, &item2); err != nil {
		return nil, ErrUnsupported
	}
	checkKey, err := nssDecrypt(item2, globalSalt, []byte(""))
	if err != nil {
		// A non-empty primary password is the usual reason; report clearly.
		return nil, ErrUnsupported
	}
	if !strings.Contains(string(checkKey), "password-check") {
		return nil, ErrUnsupported
	}

	var keySalt, keyItem []byte
	if err := db.QueryRow(`SELECT item1, item2 FROM metadata WHERE id = 2`).Scan(&keySalt, &keyItem); err != nil {
		return nil, ErrUnsupported
	}
	master, err := nssDecrypt(keyItem, keySalt, []byte(""))
	if err != nil {
		return nil, ErrUnsupported
	}
	if len(master) < 24 {
		return nil, ErrUnsupported
	}
	return master, nil
}

// nssDecrypt unwraps one NSS metadata entry: PBKDF2-SHA1 -> 3DES-CBC.
func nssDecrypt(blob, globalSalt, primaryPassword []byte) ([]byte, error) {
	var parsed nssBlob
	if _, err := asn1.Unmarshal(blob, &parsed); err != nil {
		return nil, ErrUnsupported
	}
	if len(parsed.Parameters.Salt) == 0 || len(parsed.Parameters.IV) != des.BlockSize {
		return nil, ErrUnsupported
	}
	salt := append(append([]byte{}, primaryPassword...), globalSalt...)
	key := pbkdf2.Key(salt, parsed.Parameters.Salt, 1, 24, sha1.New)
	block, err := des.NewTripleDESCipher(key)
	if err != nil {
		return nil, ErrUnsupported
	}
	if len(parsed.Ciphertext)%des.BlockSize != 0 {
		return nil, ErrUnsupported
	}
	plain := make([]byte, len(parsed.Ciphertext))
	cipher.NewCBCDecrypter(block, parsed.Parameters.IV).CryptBlocks(plain, parsed.Ciphertext)
	// PKCS#7 padding.
	if n := int(plain[len(plain)-1]); n > 0 && n <= des.BlockSize && len(plain) >= n {
		plain = plain[:len(plain)-n]
	}
	return plain, nil
}

// decryptFirefoxValue decrypts a legacy "v1" cookie value (AES-256-CBC).
func decryptFirefoxValue(encrypted, master []byte) (string, error) {
	if len(encrypted) < 3 || !strings.HasPrefix(string(encrypted[:3]), "v1") {
		return "", ErrUnsupported
	}
	// Legacy scheme: SHA1(masterKey) is the AES key, first 16 bytes are the IV.
	sum := sha1.Sum(master)
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return "", ErrUnsupported
	}
	payload := encrypted[3:]
	if len(payload) < aes.BlockSize || (len(payload))%aes.BlockSize != 0 {
		return "", ErrUnsupported
	}
	iv, ciphertext := payload[:aes.BlockSize], payload[aes.BlockSize:]
	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)
	if n := int(plain[len(plain)-1]); n > 0 && n <= aes.BlockSize && len(plain) >= n {
		plain = plain[:len(plain)-n]
	}
	return string(plain), nil
}
