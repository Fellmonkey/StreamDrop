// Package security provides at-rest encryption for account credentials, OAuth
// tokens and notification secrets.
package security

// SecretStore encrypts and decrypts small secrets before they are written to
// SQLite.
type SecretStore interface {
	Encrypt(data []byte) ([]byte, error)
	Decrypt(data []byte) ([]byte, error)
}

// New returns the platform implementation: DPAPI on Windows, AES-256-GCM with
// a machine-derived key elsewhere.
func New() SecretStore {
	return newStore()
}
