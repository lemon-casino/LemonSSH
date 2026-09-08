// Package serialport owns the serial protocol owner (P3-08.2, TERM-03.2):
// port enumeration, configuration validation, open/close lifecycle and
// disconnect semantics. Real device I/O requires hardware; everything else is
// enforced here so misconfiguration fails closed before the device is opened.
package serialport

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrPortInvalid     = errors.New("serial port configuration invalid")
	ErrPortUnavailable = errors.New("serial port unavailable")
	ErrNotOpen         = errors.New("serial port not open")
)

// Config is the validated serial port configuration.
type Config struct {
	Port     string
	BaudRate int
	DataBits int
	Parity   string // "none", "odd", "even", "mark", "space"
	StopBits string // "1", "1.5", "2"
}

// Info describes one enumerated port.
type Info struct {
	Name string `json:"name"`
}

// Enumerator abstracts OS port discovery (injectable for tests).
type Enumerator interface {
	ListPorts() ([]Info, error)
}

// Opener abstracts opening a configured port (injectable for tests).
type Opener interface {
	Open(config Config) (Port, error)
}

// Port is one open serial connection.
type Port interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
}

var validParities = map[string]bool{"none": true, "odd": true, "even": true, "mark": true, "space": true}
var validStopBits = map[string]bool{"1": true, "1.5": true, "2": true}

// Validate enforces the configuration contract.
func Validate(config Config) error {
	if strings.TrimSpace(config.Port) == "" {
		return fmt.Errorf("%w: port name empty", ErrPortInvalid)
	}
	if config.BaudRate <= 0 {
		return fmt.Errorf("%w: baud rate must be positive", ErrPortInvalid)
	}
	if config.DataBits != 5 && config.DataBits != 6 && config.DataBits != 7 && config.DataBits != 8 {
		return fmt.Errorf("%w: data bits must be 5-8", ErrPortInvalid)
	}
	if !validParities[strings.ToLower(config.Parity)] {
		return fmt.Errorf("%w: parity %q", ErrPortInvalid, config.Parity)
	}
	if !validStopBits[config.StopBits] {
		return fmt.Errorf("%w: stop bits %q", ErrPortInvalid, config.StopBits)
	}
	return nil
}

// DefaultConfig returns the product default (115200 8N1).
func DefaultConfig(port string) Config {
	return Config{Port: port, BaudRate: 115200, DataBits: 8, Parity: "none", StopBits: "1"}
}

// Backend combines port discovery and opening.
type Backend interface {
	Enumerator
	Opener
}

// Session owns opened serial ports per terminal session.
type Session struct {
	mu      sync.Mutex
	ports   map[string]Port
	backend Backend
}

// NewSession constructs a session over a backend.
func NewSession(backend Backend) *Session {
	return &Session{ports: make(map[string]Port), backend: backend}
}

// List enumerates available ports.
func (s *Session) List() ([]Info, error) {
	return s.backend.ListPorts()
}

// Open validates, opens and registers a port. A duplicate open on the same
// name fails closed instead of leaking the previous handle.
func (s *Session) Open(config Config) error {
	if err := Validate(config); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.ports[config.Port]; exists {
		return fmt.Errorf("%w: %s already open", ErrPortUnavailable, config.Port)
	}
	port, err := s.backend.Open(config)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrPortUnavailable, config.Port, err)
	}
	s.ports[config.Port] = port
	return nil
}

// Write sends data to an open port.
func (s *Session) Write(portName string, data []byte) (int, error) {
	port, err := s.get(portName)
	if err != nil {
		return 0, err
	}
	return port.Write(data)
}

// Close releases one open port. Closing an unknown port is an error so
// double-close bugs surface instead of hiding.
func (s *Session) Close(portName string) error {
	s.mu.Lock()
	port, ok := s.ports[portName]
	delete(s.ports, portName)
	s.mu.Unlock()
	if !ok {
		return ErrNotOpen
	}
	return port.Close()
}

// CloseAll releases every open port (session teardown path).
func (s *Session) CloseAll() error {
	s.mu.Lock()
	ports := s.ports
	s.ports = make(map[string]Port)
	s.mu.Unlock()
	var firstErr error
	for _, port := range ports {
		if err := port.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Session) get(portName string) (Port, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	port, ok := s.ports[portName]
	if !ok {
		return nil, ErrNotOpen
	}
	return port, nil
}

// ConnectTimeout is the documented dial budget for device open attempts.
const ConnectTimeout = 5 * time.Second
