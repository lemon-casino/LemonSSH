package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

func encryptedKeyPEM(t *testing.T, passphrase string) ([]byte, gossh.PublicKey) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := gossh.MarshalPrivateKeyWithPassphrase(private, "", []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}
	public, err := gossh.NewPublicKey(private.Public().(ed25519.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block), public
}

func TestPassphraseBrokerRequestRespond(t *testing.T) {
	// emit delivers through a buffered channel: receiving is a reliable
	// completion signal for the callback (and its happens-before edge), where
	// polling PendingRequestID only proves registration, which request()
	// completes BEFORE emitting.
	emitted := make(chan PassphraseRequest, 1)
	broker := NewPassphraseBroker(func(request PassphraseRequest) {
		emitted <- request
	}, nil, time.Second)
	prompt := broker.Requester("term-1", 4)

	done := make(chan struct{})
	var answer string
	var promptErr error
	go func() {
		defer close(done)
		answer, promptErr = prompt("host-a", "/home/u/key_ed25519", false)
	}()

	var request PassphraseRequest
	select {
	case request = <-emitted:
	case <-time.After(time.Second):
		t.Fatal("prompt never emitted")
	}
	if broker.PendingRequestID() == "" {
		t.Fatal("emitted request never registered")
	}
	if err := broker.Respond(request.RequestID, "phrase", false); err != nil {
		t.Fatal(err)
	}
	<-done
	if promptErr != nil || answer != "phrase" {
		t.Fatalf("prompt returned %q, %v", answer, promptErr)
	}
	if request.SessionID != "term-1" || request.BootEpoch != 4 || request.Hostname != "host-a" {
		t.Fatalf("session correlation missing: %+v", request)
	}
	if request.KeyPath != "/home/u/key_ed25519" || request.KeyName != "key_ed25519" {
		t.Fatalf("key labels missing: %+v", request)
	}
	if err := broker.Respond(request.RequestID, "again", false); !errors.Is(err, ErrPassphraseUnknown) {
		t.Fatalf("stale respond: %v", err)
	}
}

func TestPassphraseBrokerCancelAndTimeout(t *testing.T) {
	var events []string
	notify := func(name string, payload any) { events = append(events, name) }
	broker := NewPassphraseBroker(func(PassphraseRequest) {}, notify, 20*time.Millisecond)
	prompt := broker.Requester("", 0)

	done := make(chan struct{})
	var promptErr error
	go func() {
		defer close(done)
		_, promptErr = prompt("host", "key", false)
	}()
	// Wait for the registration instead of a fixed sleep: a loaded runner
	// must not miss the cancel window and turn this into a timeout round.
	deadline := time.Now().Add(time.Second)
	for broker.PendingRequestID() == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if id := broker.PendingRequestID(); id != "" {
		if err := broker.Respond(id, "", true); err != nil {
			t.Fatal(err)
		}
	}
	<-done
	if !errors.Is(promptErr, ErrPassphraseCancelled) {
		t.Fatalf("cancel: %v", promptErr)
	}

	_, timeoutErr := prompt("host", "key", false)
	if !errors.Is(timeoutErr, ErrPassphraseTimeout) {
		t.Fatalf("timeout: %v", timeoutErr)
	}
	// The cancel round notifies the renderer, the timeout round does too.
	if len(events) != 2 || events[0] != EventPassphraseCancelled || events[1] != EventPassphraseTimeout {
		t.Fatalf("notify events: %v", events)
	}
}

func TestParsePrivateKeyWithPromptRecovers(t *testing.T) {
	pemBytes, public := encryptedKeyPEM(t, "phrase")
	prompts := 0
	signer, err := parsePrivateKeyWithPrompt(AuthMethod{
		PrivateKeyPEM: pemBytes,
		KeyPath:       "C:/keys/id_ed25519",
		RequestPassphrase: func(keyPath string, passphraseInvalid bool) (string, error) {
			prompts++
			if keyPath != "C:/keys/id_ed25519" {
				t.Fatalf("keyPath %q", keyPath)
			}
			if passphraseInvalid {
				t.Fatal("first prompt must not be flagged invalid")
			}
			return "phrase", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prompts != 1 {
		t.Fatalf("prompts %d", prompts)
	}
	if string(signer.PublicKey().Marshal()) != string(public.Marshal()) {
		t.Fatal("wrong signer recovered")
	}
}

func TestParsePrivateKeyWithPromptRecoversAfterWrongAttempt(t *testing.T) {
	pemBytes, public := encryptedKeyPEM(t, "phrase")
	var invalidFlags []bool
	signer, err := parsePrivateKeyWithPrompt(AuthMethod{
		PrivateKeyPEM: pemBytes,
		KeyPath:       "id",
		RequestPassphrase: func(_ string, passphraseInvalid bool) (string, error) {
			invalidFlags = append(invalidFlags, passphraseInvalid)
			if len(invalidFlags) == 1 {
				return "wrong", nil
			}
			return "phrase", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(signer.PublicKey().Marshal()) != string(public.Marshal()) {
		t.Fatal("wrong signer recovered")
	}
	if len(invalidFlags) != 2 || !invalidFlags[1] {
		t.Fatalf("retry must flag the passphrase invalid: %v", invalidFlags)
	}
}

func TestParsePrivateKeyWithPromptCancelAndRejection(t *testing.T) {
	pemBytes, _ := encryptedKeyPEM(t, "phrase")
	_, err := parsePrivateKeyWithPrompt(AuthMethod{
		PrivateKeyPEM: pemBytes,
		KeyPath:       "id",
		RequestPassphrase: func(string, bool) (string, error) {
			return "", nil
		},
	})
	if !errors.Is(err, ErrPassphraseCancelled) {
		t.Fatalf("empty answer: %v", err)
	}

	promptErr := errors.New("renderer gone")
	_, err = parsePrivateKeyWithPrompt(AuthMethod{
		PrivateKeyPEM: pemBytes,
		RequestPassphrase: func(string, bool) (string, error) {
			return "", promptErr
		},
	})
	if !errors.Is(err, promptErr) {
		t.Fatalf("prompt error: %v", err)
	}

	_, err = parsePrivateKeyWithPrompt(AuthMethod{
		PrivateKeyPEM: pemBytes,
		KeyPath:       "stored-key",
		RequestPassphrase: func(string, bool) (string, error) {
			return "wrong", nil
		},
	})
	var rejected *PassphraseRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("rejection: %v", err)
	}
	if rejected.KeyPath != "stored-key" {
		t.Fatalf("rejected key path %q", rejected.KeyPath)
	}
}

func TestEncryptedKeyWithoutPromptFailsClosed(t *testing.T) {
	pemBytes, _ := encryptedKeyPEM(t, "phrase")
	_, err := parsePrivateKeyWithPrompt(AuthMethod{PrivateKeyPEM: pemBytes})
	if err == nil || !strings.Contains(err.Error(), "invalid private key") {
		t.Fatalf("expected legacy fail-closed error, got %v", err)
	}
}

func TestKeyNameFor(t *testing.T) {
	cases := map[string]string{
		"":                          "SSH key",
		"/home/u/.ssh/id_rsa":       "id_rsa",
		`C:\Users\Lemon\.ssh\k.pem`: "k.pem",
		"key_ed25519":               "key_ed25519",
	}
	for input, want := range cases {
		if got := keyNameFor(input); got != want {
			t.Fatalf("keyNameFor(%q) = %q, want %q", input, got, want)
		}
	}
}
