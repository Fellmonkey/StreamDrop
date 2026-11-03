//go:build windows

package security

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dpapiStore uses the Win32 CryptoAPI (CryptProtectData / CryptUnprotectData).
// The key is derived by Windows from the logged-in user profile, so the file is
// unreadable by another account and does not travel with the database.
type dpapiStore struct{}

func newStore() SecretStore { return dpapiStore{} }

var (
	crypt32              = windows.NewLazySystemDLL("crypt32.dll")
	procCryptProtectData = crypt32.NewProc("CryptProtectData")
	procCryptUnprotect   = crypt32.NewProc("CryptUnprotectData")
)

const cryptProtectUIForbidden = 0x1

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func newBlob(b []byte) dataBlob {
	if len(b) == 0 {
		return dataBlob{}
	}
	return dataBlob{cbData: uint32(len(b)), pbData: &b[0]}
}

func (b dataBlob) bytes() []byte {
	if b.cbData == 0 || b.pbData == nil {
		return nil
	}
	out := make([]byte, b.cbData)
	copy(out, unsafe.Slice(b.pbData, b.cbData))
	return out
}

// description is embedded in the protected blob so a developer inspecting the
// database can tell what the ciphertext belongs to.
var description = []byte("StreamDrop")

func (dpapiStore) Encrypt(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	in := newBlob(data)
	desc := newBlob(description)
	var out dataBlob
	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		uintptr(unsafe.Pointer(&desc.pbData)),
		0, // optional entropy
		0, // reserved
		0, // prompt struct
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.pbData)))
	return out.bytes(), nil
}

func (dpapiStore) Decrypt(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	in := newBlob(data)
	var out dataBlob
	r, _, err := procCryptUnprotect.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		0,
		0,
		0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&out)),
	)
	if r == 0 {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.pbData)))
	res := out.bytes()
	if res == nil {
		return nil, errors.New("DPAPI returned an empty payload")
	}
	return res, nil
}
