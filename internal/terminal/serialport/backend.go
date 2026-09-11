//go:build !js

package serialport

import (
	"fmt"
	"time"

	"go.bug.st/serial"
)

// OSBackend implements Enumerator and Opener with go.bug.st/serial.
type OSBackend struct{}

// NewOSBackend constructs the real platform backend.
func NewOSBackend() *Session { return NewSession(osBackend{}) }

type osBackend struct{}

// ListPorts returns enriched port metadata where the platform can supply it.
// listPortInfos is platform-split so Linux/Windows use the cgo-free USB
// enumerator while macOS falls back to name-only under CGO_ENABLED=0.
func (osBackend) ListPorts() ([]Info, error) {
	return listPortInfos()
}

func (osBackend) Open(config Config) (Port, error) {
	if NormalizeFlowControl(config.FlowControl) != "none" {
		// go.bug.st/serial v1.6.4 hardcodes RTS/CTS off and clears IXON/IXOFF,
		// so it cannot program either hardware or software flow control.
		return nil, fmt.Errorf("%w: %s", ErrFlowControlUnsupported, config.FlowControl)
	}
	mode := &serial.Mode{
		BaudRate: config.BaudRate,
		DataBits: config.DataBits,
	}
	switch config.Parity {
	case "odd":
		mode.Parity = serial.OddParity
	case "even":
		mode.Parity = serial.EvenParity
	case "mark":
		mode.Parity = serial.MarkParity
	case "space":
		mode.Parity = serial.SpaceParity
	default:
		mode.Parity = serial.NoParity
	}
	switch config.StopBits {
	case "1.5":
		mode.StopBits = serial.OnePointFiveStopBits
	case "2":
		mode.StopBits = serial.TwoStopBits
	default:
		mode.StopBits = serial.OneStopBit
	}
	port, err := serial.Open(config.Port, mode)
	if err != nil {
		return nil, err
	}
	if readTimeoutErr := port.SetReadTimeout(100 * time.Millisecond); readTimeoutErr != nil {
		_ = port.Close()
		return nil, fmt.Errorf("set read timeout: %w", readTimeoutErr)
	}
	return port, nil
}
