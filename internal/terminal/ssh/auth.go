package ssh

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/ssh"
)

// AuthMethod is the shell-neutral authentication input. Exactly one strategy
// is used per dial attempt; keyboard-interactive receives challenge callbacks
// so MFA flows can be surfaced to the renderer.
type AuthMethod struct {
	// Password authenticates with user:password.
	Password string
	// PrivateKeyPEM is an optional PEM private key; Passphrase decrypts it.
	PrivateKeyPEM []byte
	Passphrase    string
	// KeyPath labels the private key in passphrase prompts (file path or the
	// caller's key identifier). Display only.
	KeyPath string
	// RequestPassphrase prompts the renderer when the private key is encrypted
	// and no stored passphrase (or a wrong one) was supplied. An empty answer
	// or error fails the auth attempt.
	RequestPassphrase func(keyPath string, passphraseInvalid bool) (string, error)
	// Interactive answers keyboard-interactive challenges (MFA). Each
	// question is surfaced with its echo flag; empty answers are allowed.
	Interactive func(question string, echo bool) (string, error)
	// Challenge answers a full keyboard-interactive round (name, instruction,
	// all prompts) so the renderer can show one MFA modal per round.
	Challenge func(name, instruction string, questions []string, echoes []bool) ([]string, error)
	// UseAgent adds the local SSH agent as an auth method when reachable.
	UseAgent bool
	// Certificate is an OpenSSH user certificate; requires PrivateKeyPEM.
	Certificate []byte
}

// maxPassphraseAttempts bounds the prompt → parse → retry loop so a hung
// renderer cannot extend the dial indefinitely (each prompt also times out
// inside its broker).
const maxPassphraseAttempts = 3

var ErrNoAuthMethod = errors.New("no ssh auth method configured")

// BuildAuthMethods converts the neutral auth input into x/crypto methods in
// priority order: key, password, keyboard-interactive. x/crypto tries each in
// turn and accepts partial-success MFA flows.
func BuildAuthMethods(method AuthMethod) ([]ssh.AuthMethod, error) {
	return buildAuthMethods(context.Background(), method)
}

func buildAuthMethods(ctx context.Context, method AuthMethod) ([]ssh.AuthMethod, error) {
	var result []ssh.AuthMethod
	if strings.TrimSpace(method.Password) != "" || len(method.PrivateKeyPEM) > 0 || method.Interactive != nil || method.Challenge != nil || method.UseAgent {
		// at least one strategy present
	} else {
		return nil, ErrNoAuthMethod
	}
	if len(method.PrivateKeyPEM) > 0 {
		signer, err := parsePrivateKeyWithPrompt(method)
		if err != nil {
			return nil, err
		}
		result = append(result, ssh.PublicKeys(signer))
	}
	if method.UseAgent {
		var certificate []byte
		if len(method.PrivateKeyPEM) == 0 {
			certificate = method.Certificate
		}
		agentMethod, err := agentAuthWithCertificate(ctx, "", certificate)
		if err != nil {
			return nil, err
		}
		result = append(result, agentMethod)
	}
	if method.Password != "" {
		result = append(result, ssh.Password(method.Password))
	}
	if method.Challenge != nil {
		result = append(result, ssh.KeyboardInteractive(method.Challenge))
	} else if method.Interactive != nil {
		result = append(result, ssh.KeyboardInteractive(func(name, instruction string, questions []string, echoes []bool) ([]string, error) {
			answers := make([]string, 0, len(questions))
			for index, question := range questions {
				echo := false
				if index < len(echoes) {
					echo = echoes[index]
				}
				answer, err := method.Interactive(question, echo)
				if err != nil {
					return nil, err
				}
				answers = append(answers, answer)
			}
			return answers, nil
		}))
	}
	if len(result) == 0 {
		return nil, ErrNoAuthMethod
	}
	return result, nil
}

// parsePrivateKeyWithPrompt parses the configured private key, and when
// x/crypto reports a missing (or wrong) passphrase it asks RequestPassphrase
// and retries. Without a prompt callback the legacy fail-closed error is kept.
func parsePrivateKeyWithPrompt(method AuthMethod) (ssh.Signer, error) {
	parse := func(passphrase string) (ssh.Signer, error) {
		if len(method.Certificate) > 0 {
			return ParseCertificateSigner(method.PrivateKeyPEM, passphrase, method.Certificate)
		}
		if passphrase != "" {
			return ssh.ParsePrivateKeyWithPassphrase(method.PrivateKeyPEM, []byte(passphrase))
		}
		return ssh.ParsePrivateKey(method.PrivateKeyPEM)
	}
	signer, err := parse(method.Passphrase)
	if err == nil {
		return signer, nil
	}
	if method.RequestPassphrase == nil || !passphraseRequired(err) {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	for attempt := 0; attempt < maxPassphraseAttempts; attempt++ {
		answer, promptErr := method.RequestPassphrase(method.KeyPath, attempt > 0)
		if promptErr != nil {
			return nil, fmt.Errorf("passphrase prompt failed: %w", promptErr)
		}
		if answer == "" {
			return nil, fmt.Errorf("passphrase prompt cancelled: %w", ErrPassphraseCancelled)
		}
		signer, err = parse(answer)
		if err == nil {
			return signer, nil
		}
		if !passphraseRequired(err) {
			return nil, fmt.Errorf("invalid private key: %w", err)
		}
	}
	return nil, &PassphraseRejectedError{KeyPath: method.KeyPath, Err: err}
}

// passphraseRequired reports that the key is encrypted and the supplied (or
// empty) passphrase did not decrypt it: x/crypto returns PassphraseMissingError
// when no passphrase was given and x509.IncorrectPasswordError when a wrong
// one was.
func passphraseRequired(err error) bool {
	var missing *ssh.PassphraseMissingError
	return errors.As(err, &missing) || errors.Is(err, x509.IncorrectPasswordError)
}
