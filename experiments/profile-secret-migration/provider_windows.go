//go:build windows

package migrationprobe

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const cryptprotectUIForbidden = 0x1

type platformProvider struct{}

func NewPlatformProvider() Provider { return &platformProvider{} }

func (*platformProvider) Name() string    { return ProviderName }
func (*platformProvider) Available() bool { return true }

func (*platformProvider) Seal(plaintext []byte, purpose string) ([]byte, error) {
	if len(plaintext) == 0 || len(plaintext) > MaxPlaintextBytes {
		return nil, errors.New("invalid plaintext size")
	}
	entropy, err := purposeEntropy(purpose)
	if err != nil {
		return nil, err
	}
	in := bytesToBlob(plaintext)
	entropyBlob := bytesToBlob(entropy)
	var out windows.DataBlob
	err = windows.CryptProtectData(&in, nil, &entropyBlob, 0, nil, cryptprotectUIForbidden, &out)
	runtime.KeepAlive(plaintext)
	runtime.KeepAlive(entropy)
	zero(entropy)
	if err != nil {
		return nil, errors.New("seal failed")
	}
	native := unsafe.Slice(out.Data, out.Size)
	defer func() {
		zero(native)
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	}()
	ciphertext := append([]byte(nil), native...)
	defer zero(ciphertext)
	return encodeEnvelope(purpose, ciphertext)
}

func (*platformProvider) Open(data []byte, purpose string) ([]byte, error) {
	ciphertext, err := decodeEnvelope(data, purpose)
	if err != nil {
		return nil, err
	}
	defer zero(ciphertext)
	entropy, err := purposeEntropy(purpose)
	if err != nil {
		return nil, err
	}
	in := bytesToBlob(ciphertext)
	entropyBlob := bytesToBlob(entropy)
	var out windows.DataBlob
	err = windows.CryptUnprotectData(&in, nil, &entropyBlob, 0, nil, cryptprotectUIForbidden, &out)
	runtime.KeepAlive(ciphertext)
	runtime.KeepAlive(entropy)
	zero(entropy)
	if err != nil {
		return nil, errors.New("open failed")
	}
	native := unsafe.Slice(out.Data, out.Size)
	defer func() {
		zero(native)
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	}()
	return append([]byte(nil), native...), nil
}

func bytesToBlob(value []byte) windows.DataBlob {
	if len(value) == 0 {
		return windows.DataBlob{}
	}
	return windows.DataBlob{Size: uint32(len(value)), Data: &value[0]}
}

func purposeEntropy(purpose string) ([]byte, error) {
	if err := validatePurpose(purpose); err != nil {
		return nil, err
	}
	canonical := make([]byte, 0, 64+len(purpose))
	canonical = appendField(canonical, "profile-secret-migration-envelope")
	var version [4]byte
	binary.BigEndian.PutUint32(version[:], uint32(EnvelopeVersion))
	canonical = append(canonical, version[:]...)
	canonical = appendField(canonical, ProviderName)
	canonical = appendField(canonical, purpose)
	sum := sha256.Sum256(canonical)
	zero(canonical)
	entropy := append([]byte(nil), sum[:]...)
	zero(sum[:])
	return entropy, nil
}
