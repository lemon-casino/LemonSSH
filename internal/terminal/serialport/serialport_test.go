package serialport

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

type fakePort struct {
	mu       sync.Mutex
	closed   bool
	written  []byte
	closeErr error
}

func (p *fakePort) Read(b []byte) (int, error) { return 0, nil }
func (p *fakePort) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.written = append(p.written, b...)
	return len(b), nil
}
func (p *fakePort) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return p.closeErr
}

type fakeBackend struct {
	mu        sync.Mutex
	ports     map[string]*fakePort
	listError error
}

func newFakeBackend() *fakeBackend { return &fakeBackend{ports: make(map[string]*fakePort)} }
func (b *fakeBackend) ListPorts() ([]Info, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.listError != nil {
		return nil, b.listError
	}
	infos := make([]Info, 0, len(b.ports))
	for name := range b.ports {
		infos = append(infos, Info{Name: name})
	}
	return infos, nil
}
func (b *fakeBackend) Open(config Config) (Port, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	port, ok := b.ports[config.Port]
	if !ok {
		return nil, errors.New("device missing")
	}
	return port, nil
}

func TestValidateEnforcesContract(t *testing.T) {
	valid := DefaultConfig("COM3")
	if err := Validate(valid); err != nil {
		t.Fatalf("default config must validate: %v", err)
	}
	bad := []Config{
		{Port: "", BaudRate: 115200, DataBits: 8, Parity: "none", StopBits: "1"},
		{Port: "COM3", BaudRate: 0, DataBits: 8, Parity: "none", StopBits: "1"},
		{Port: "COM3", BaudRate: 115200, DataBits: 9, Parity: "none", StopBits: "1"},
		{Port: "COM3", BaudRate: 115200, DataBits: 8, Parity: "flipped", StopBits: "1"},
		{Port: "COM3", BaudRate: 115200, DataBits: 8, Parity: "none", StopBits: "3"},
	}
	for index, config := range bad {
		if err := Validate(config); err == nil {
			t.Fatalf("invalid config %d accepted", index)
		}
	}
}

func TestSessionEnumerateOpenWriteClose(t *testing.T) {
	backend := newFakeBackend()
	backend.ports["COM3"] = &fakePort{}
	backend.ports["COM4"] = &fakePort{}
	session := NewSession(backend)

	ports, err := session.List()
	if err != nil || len(ports) != 2 {
		t.Fatalf("enumerate: %v (%d)", err, len(ports))
	}
	if err := session.Open(DefaultConfig("COM3")); err != nil {
		t.Fatalf("open: %v", err)
	}
	// Duplicate open fails closed without leaking the first handle.
	if err := session.Open(DefaultConfig("COM3")); err == nil {
		t.Fatal("duplicate open must fail")
	}
	n, err := session.Write("COM3", []byte("AT\r\n"))
	if err != nil || n != 4 {
		t.Fatalf("write: %d %v", n, err)
	}
	// Missing device fails closed with the wrapped sentinel.
	if err := session.Open(DefaultConfig("COM9")); err == nil || !strings.Contains(err.Error(), ErrPortUnavailable.Error()) {
		t.Fatalf("missing device must fail closed, got %v", err)
	}
	if err := session.Close("COM3"); err != nil {
		t.Fatal(err)
	}
	if err := session.Close("COM3"); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("double close must surface, got %v", err)
	}
}

func TestSessionCloseAllReleasesEveryPort(t *testing.T) {
	backend := newFakeBackend()
	com3 := &fakePort{}
	com4 := &fakePort{}
	backend.ports["COM3"] = com3
	backend.ports["COM4"] = com4
	session := NewSession(backend)
	if err := session.Open(DefaultConfig("COM3")); err != nil {
		t.Fatal(err)
	}
	if err := session.Open(DefaultConfig("COM4")); err != nil {
		t.Fatal(err)
	}
	if err := session.CloseAll(); err != nil {
		t.Fatal(err)
	}
	if !com3.closed || !com4.closed {
		t.Fatal("close-all must close every port")
	}
	if _, writeErr := session.Write("COM3", []byte("x")); !errors.Is(writeErr, ErrNotOpen) {
		t.Fatalf("write after close-all must fail: %v", writeErr)
	}
}

func TestListErrorPropagates(t *testing.T) {
	backend := newFakeBackend()
	backend.listError = errors.New("driver failure")
	session := NewSession(backend)
	if _, err := session.List(); err == nil {
		t.Fatal("list error must propagate")
	}
}

func TestValidateEnforcesFlowControlVocabulary(t *testing.T) {
	base := DefaultConfig("COM3")
	if base.FlowControl != "none" {
		t.Fatalf("default flow control must be none, got %q", base.FlowControl)
	}
	for _, valid := range []string{"", "none", "xon/xoff", "rts/cts", "RTS/CTS"} {
		config := base
		config.FlowControl = valid
		if err := Validate(config); err != nil {
			t.Fatalf("flow control %q must validate: %v", valid, err)
		}
	}
	for _, invalid := range []string{"hardware", "software", "rts", "1"} {
		config := base
		config.FlowControl = invalid
		if err := Validate(config); err == nil {
			t.Fatalf("flow control %q must be rejected", invalid)
		}
	}
}

func TestNormalizeFlowControl(t *testing.T) {
	cases := map[string]string{
		"":         "none",
		"  ":       "none",
		"none":     "none",
		"NONE":     "none",
		"xon/xoff": "xon/xoff",
		"RTS/CTS":  "rts/cts",
	}
	for input, expected := range cases {
		if got := NormalizeFlowControl(input); got != expected {
			t.Fatalf("NormalizeFlowControl(%q) = %q, want %q", input, got, expected)
		}
	}
}

// rejectingBackend records the config it was asked to open so tests can assert
// the full line configuration survives to the backend unchanged.
type rejectingBackend struct {
	opened Config
}

func (b *rejectingBackend) ListPorts() ([]Info, error) { return nil, nil }
func (b *rejectingBackend) Open(config Config) (Port, error) {
	b.opened = config
	return nil, errors.New("no device in test")
}

func TestOpenPassesFullConfigToBackend(t *testing.T) {
	backend := &rejectingBackend{}
	session := NewSession(backend)
	config := Config{Port: "COM7", BaudRate: 9600, DataBits: 7, Parity: "even", StopBits: "2", FlowControl: "none"}
	if err := session.Open(config); err == nil {
		t.Fatal("open must surface the backend error")
	}
	if backend.opened != config {
		t.Fatalf("backend saw %+v, want %+v", backend.opened, config)
	}
}
