package mosh

import (
	"strings"
	"testing"
)

func TestETBootstrapContractAndSecretRedaction(t *testing.T) {
	input, err := ETBootstrapInput()
	if err != nil {
		t.Fatal(err)
	}
	if len(input) != 65 || !strings.HasPrefix(input, "XXX") || !strings.HasSuffix(input, "_xterm-256color\n") {
		t.Fatalf("invalid bootstrap shape length %d", len(input))
	}
	pair := strings.Repeat("I", 16) + "/" + strings.Repeat("K", 32)
	got, err := ReadETConnect(strings.NewReader("banner\r\nIDPASSKEY:" + pair + "\r\n"))
	if err != nil || got != pair {
		t.Fatal("valid upstream response rejected", err)
	}
	for _, response := range []string{"IDPASSKEY:private-secret\n", "IDPASSKEY:" + strings.Repeat("I", 16) + "/" + strings.Repeat("!", 32) + "\n", "noise"} {
		_, err = ReadETConnect(strings.NewReader(response))
		if err == nil || strings.Contains(err.Error(), "private-secret") {
			t.Fatal("invalid response must fail without credentials")
		}
	}
	command := ETServerCommand("/opt/et path/et'terminal", "/run/custom'fifo")
	if command != `'/opt/et path/et'\''terminal' --serverfifo='/run/custom'\''fifo'` {
		t.Fatal(command)
	}
	if strings.Contains(command, input[:16]) {
		t.Fatal("bootstrap input leaked into command")
	}
}
