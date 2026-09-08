package applock

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeCredentials struct{}

func (fakeCredentials) Seal(plaintext []byte, purpose string) ([]byte, error) {
	out := append([]byte("sealed|"+purpose+"|"), plaintext...)
	return out, nil
}
func (fakeCredentials) Open(envelope []byte, purpose string) ([]byte, error) {
	prefix := []byte("sealed|" + purpose + "|")
	if len(envelope) <= len(prefix) {
		return nil, errors.New("bad")
	}
	return envelope[len(prefix):], nil
}

func TestEnableVerifyAndReject(t *testing.T) {
	service := New(fakeCredentials{})
	verifier, err := service.Enable(context.Background(), "hunter22")
	if err != nil {
		t.Fatal(err)
	}
	// The stored record must never carry the password.
	raw, _ := json.Marshal(verifier)
	if strings.Contains(string(raw), "hunter22") {
		t.Fatal("verifier record must not contain the password")
	}
	if err := service.Verify(verifier, "hunter22"); err != nil {
		t.Fatal(err)
	}
	if err := service.Verify(verifier, "wrongpass"); !errors.Is(err, ErrPasswordRejected) {
		t.Fatalf("wrong password must reject: %v", err)
	}
}

func TestWeakPasswordRejected(t *testing.T) {
	service := New(fakeCredentials{})
	if _, err := service.Enable(context.Background(), "abc"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("weak password must fail: %v", err)
	}
}

func TestChangePasswordRequiresOld(t *testing.T) {
	service := New(fakeCredentials{})
	verifier, err := service.Enable(context.Background(), "old-pass-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ChangePassword(context.Background(), verifier, "wrong", "new-pass-9"); !errors.Is(err, ErrPasswordRejected) {
		t.Fatalf("change with wrong old password must fail: %v", err)
	}
	changed, err := service.ChangePassword(context.Background(), verifier, "old-pass-1", "new-pass-9")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Verify(changed, "new-pass-9"); err != nil {
		t.Fatal(err)
	}
	if err := service.Verify(changed, "old-pass-1"); err == nil {
		t.Fatal("old password must stop working after change")
	}
}

func TestMissingVerifierFailsClosed(t *testing.T) {
	service := New(fakeCredentials{})
	if err := service.Verify(Verifier{}, "anything"); !errors.Is(err, ErrNoVerifier) {
		t.Fatalf("missing verifier must fail closed: %v", err)
	}
}
