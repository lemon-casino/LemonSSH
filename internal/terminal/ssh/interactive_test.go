package ssh

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestInteractiveBrokerAnswersOneChallenge(t *testing.T) {
	var seen KeyboardChallenge
	broker := NewInteractiveBroker(func(challenge KeyboardChallenge) {
		seen = challenge
	}, time.Second)
	handler := broker.Handler("bastion.example")

	var wg sync.WaitGroup
	wg.Add(1)
	var answers []string
	var handlerErr error
	go func() {
		defer wg.Done()
		answers, handlerErr = handler("login", "enter otp", []string{"Password: ", "OTP: "}, []bool{false, true})
	}()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if seen.RequestID != "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if seen.RequestID == "" {
		t.Fatal("expected keyboard-interactive challenge to be emitted")
	}
	if seen.Hostname != "bastion.example" || seen.Name != "login" || seen.Instructions != "enter otp" {
		t.Fatalf("challenge metadata: %+v", seen)
	}
	if len(seen.Prompts) != 2 || seen.Prompts[0].Echo || !seen.Prompts[1].Echo {
		t.Fatalf("prompts: %+v", seen.Prompts)
	}
	if err := broker.Respond(seen.RequestID, []string{"secret", "123456"}, false); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if handlerErr != nil {
		t.Fatal(handlerErr)
	}
	if len(answers) != 2 || answers[0] != "secret" || answers[1] != "123456" {
		t.Fatalf("answers: %#v", answers)
	}
}

func TestInteractiveBrokerCancelAndUnknown(t *testing.T) {
	broker := NewInteractiveBroker(func(KeyboardChallenge) {}, time.Second)
	if err := broker.Respond("missing", nil, false); !errors.Is(err, ErrInteractiveUnknown) {
		t.Fatalf("unknown request must fail closed: %v", err)
	}
	handler := broker.Handler("h")
	done := make(chan error, 1)
	go func() {
		_, err := handler("", "", []string{"PIN:"}, []bool{false})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	var requestID string
	for time.Now().Before(deadline) {
		requestID = broker.PendingRequestID()
		if requestID != "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if requestID == "" {
		t.Fatal("pending challenge missing")
	}
	if err := broker.Respond(requestID, nil, true); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrInteractiveCancelled) {
		t.Fatalf("cancel must fail closed: %v", err)
	}
}

func TestFormatProxyURL(t *testing.T) {
	url, err := FormatProxyURL("socks5", "127.0.0.1", 1080, "user", "p@ss")
	if err != nil || url != "socks5://user:p%40ss@127.0.0.1:1080" {
		t.Fatalf("got %q %v", url, err)
	}
	if _, err := FormatProxyURL("command", "x", 1, "", ""); err == nil {
		t.Fatal("command proxy must fail closed")
	}
}

func TestBuildDialConfigJumpAndProxy(t *testing.T) {
	policy := StrictPolicy(NewKnownHosts(t.TempDir() + "/known_hosts"))
	config := BuildDialConfig(ConnectInput{
		Hostname: "target",
		Port:     22,
		Username: "root",
		Password: "pw",
		ProxyURL: "socks5://127.0.0.1:1080",
		JumpHosts: []ConnectInput{{
			Hostname: "jump",
			Port:     2222,
			Username: "bastion",
			Password: "jpw",
		}},
	}, policy, nil)
	if config.ProxyURL != "socks5://127.0.0.1:1080" || config.Hostname != "target" {
		t.Fatalf("target hop: %+v", config)
	}
	if len(config.JumpHosts) != 1 || config.JumpHosts[0].Hostname != "jump" || config.JumpHosts[0].Port != 2222 {
		t.Fatalf("jump: %+v", config.JumpHosts)
	}
	if config.HostKeyPolicy == nil || config.JumpHosts[0].HostKeyPolicy == nil {
		t.Fatal("every hop must carry a host-key policy")
	}
}
