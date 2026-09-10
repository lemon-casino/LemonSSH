package ssh

import (
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
	// Interactive answers keyboard-interactive challenges (MFA). Each
	// question is surfaced with its echo flag; empty answers are allowed.
	Interactive func(question string, echo bool) (string, error)
	// Challenge answers a full keyboard-interactive round (name, instruction,
	// all prompts) so the renderer can show one MFA modal per round.
	Challenge func(name, instruction string, questions []string, echoes []bool) ([]string, error)
}

var ErrNoAuthMethod = errors.New("no ssh auth method configured")

// BuildAuthMethods converts the neutral auth input into x/crypto methods in
// priority order: key, password, keyboard-interactive. x/crypto tries each in
// turn and accepts partial-success MFA flows.
func BuildAuthMethods(method AuthMethod) ([]ssh.AuthMethod, error) {
	var result []ssh.AuthMethod
	if strings.TrimSpace(method.Password) != "" || len(method.PrivateKeyPEM) > 0 || method.Interactive != nil || method.Challenge != nil {
		// at least one strategy present
	} else {
		return nil, ErrNoAuthMethod
	}
	if len(method.PrivateKeyPEM) > 0 {
		var signer ssh.Signer
		var err error
		if method.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(method.PrivateKeyPEM, []byte(method.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(method.PrivateKeyPEM)
		}
		if err != nil {
			return nil, fmt.Errorf("invalid private key: %w", err)
		}
		result = append(result, ssh.PublicKeys(signer))
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
