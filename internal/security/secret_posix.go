//go:build !windows

package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/crypto/pbkdf2"
)

// aesStore encrypts with AES-256-GCM under a key derived from the machine id.
//
// Documented limitation (back_spec.md §3.3): the key is bound to this machine,
// so the encrypted database does not move to another computer, and cloned VMs
// that share a machine-id share the key.
type aesStore struct{}

var (
	storeOnce sync.Once
	storeKey  []byte
	storeErr  error
)

func newStore() SecretStore { return aesStore{} }

func machineID() (string, error) {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
		if err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "IOPlatformUUID") {
					parts := strings.Split(line, "=")
					if len(parts) == 2 {
						return strings.Trim(strings.TrimSpace(parts[1]), "\""), nil
					}
				}
			}
		}
	}
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(path); err == nil {
			if id := strings.TrimSpace(string(b)); id != "" {
				return id, nil
			}
		}
	}
	// Last resort: hostname + user, still never empty.
	host, _ := os.Hostname()
	return fmt.Sprintf("%s/%s", host, os.Getenv("USER")), nil
}

func key() ([]byte, error) {
	storeOnce.Do(func() {
		id, err := machineID()
		if err != nil {
			storeErr = err
			return
		}
		// Deterministic KDF over the machine id. PBKDF2 is used rather than
		// Argon2 because the input is a random 128-bit identifier, not a
		// human password: there is nothing to brute-force offline without the
		// id itself.
		storeKey = pbkdf2.Key([]byte(id), []byte("streamdrop-secret-v1"), 120000, 32, sha256.New)
	})
	return storeKey, storeErr
}

func (aesStore) Encrypt(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	k, err := key()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, data, nil), nil
}

func (aesStore) Decrypt(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	k, err := key()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("secret: ciphertext too short")
	}
	nonce, payload := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, payload, nil)
}
