package mosh

import (
	"errors"
	"testing"
)

func TestParseConnectBasic(t *testing.T) {
	connect, end, ok, err := ParseConnect([]byte("MOSH CONNECT 60001 abc123KEY==\n"))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if connect.Port != 60001 || connect.Key != "abc123KEY==" {
		t.Fatalf("parsed %+v", connect)
	}
	if end != len("MOSH CONNECT 60001 abc123KEY==\n") {
		t.Fatalf("matchEnd %d", end)
	}
}

func TestParseConnectStripsControlSequences(t *testing.T) {
	// mosh-server can emit the line amid cursor-control output; the key must
	// still parse cleanly and the escape must not become part of it.
	line := "MOSH CONNECT 60001 KEY==\x1b[?25h\n"
	connect, _, ok, err := ParseConnect([]byte(line))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if connect.Key != "KEY==" {
		t.Fatalf("key %q", connect.Key)
	}
}

func TestParseConnectIncompleteLineWaits(t *testing.T) {
	// No newline yet: the marker is present but the line is not complete.
	if _, _, ok, err := ParseConnect([]byte("MOSH CONNECT 60001 KEY==")); ok || err != nil {
		t.Fatalf("ok=%v err=%v; must wait for the newline", ok, err)
	}
}

func TestParseConnectAbsentMarker(t *testing.T) {
	if _, _, ok, err := ParseConnect([]byte("Welcome to the server\n")); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestParseConnectRejectsBadPort(t *testing.T) {
	for _, line := range []string{"MOSH CONNECT 99999 KEY==\n", "MOSH CONNECT abc KEY==\n", "MOSH CONNECT 0 KEY==\n"} {
		if _, _, _, err := ParseConnect([]byte(line)); !errors.Is(err, ErrBadPort) {
			t.Fatalf("line %q must reject the port, got %v", line, err)
		}
	}
}

func TestParseConnectRejectsBadKey(t *testing.T) {
	if _, _, _, err := ParseConnect([]byte("MOSH CONNECT 60001\n")); !errors.Is(err, ErrNoConnectLine) {
		t.Fatalf("missing key must reject, got %v", err)
	}
}

func TestParseConnectExtractsIP(t *testing.T) {
	data := []byte("MOSH IP 10.0.0.5\nMOSH CONNECT 60002 secretKEY==\n")
	connect, _, ok, err := ParseConnect(data)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if connect.IP != "10.0.0.5" {
		t.Fatalf("ip %q", connect.IP)
	}
	if connect.Port != 60002 {
		t.Fatalf("port %d", connect.Port)
	}
}

func TestTrailingMarkerPartial(t *testing.T) {
	// A tail that could still grow into a marker must be retained.
	for _, tail := range []string{"MOSH", "MOSH CON", "MOSH CONNECT", "MOSH I"} {
		if !TrailingMarkerPartial("some output " + tail) {
			t.Fatalf("tail %q must be treated as partial", tail)
		}
	}
	if TrailingMarkerPartial("some output done") {
		t.Fatal("completed output must not be treated as partial")
	}
}

func TestServerCommand(t *testing.T) {
	if got := ServerCommand(""); got != "mosh-server new -s -c 256" {
		t.Fatalf("default command %q", got)
	}
	// A path with spaces must stay a single quoted word.
	if got := ServerCommand("/opt/my tools/mosh-server"); got != "'/opt/my tools/mosh-server' new -s -c 256" {
		t.Fatalf("quoted command %q", got)
	}
}

func TestClientArgsNeverCarriesKey(t *testing.T) {
	args := ClientArgs("example.com", Connect{Port: 60003, Key: "topsecret"})
	if len(args) != 2 || args[0] != "example.com" || args[1] != "60003" {
		t.Fatalf("args %v", args)
	}
	for _, arg := range args {
		if arg == "topsecret" {
			t.Fatal("key must not appear in argv")
		}
	}
	// When the server announced an address, the client targets it directly.
	args = ClientArgs("example.com", Connect{Port: 60003, Key: "k", IP: "10.1.2.3"})
	if args[0] != "10.1.2.3" {
		t.Fatalf("args %v", args)
	}
}

func TestShellQuoteEscapesEmbeddedQuote(t *testing.T) {
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Fatalf("quoted %q", got)
	}
}
