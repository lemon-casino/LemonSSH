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

func (osBackend) ListPorts() ([]Info, error) {
	native, err := serial.GetPortsList()
	if err != nil {
		return nil, err
	}
	infos := make([]Info, 0, len(native))
	for _, name := range native {
		infos = append(infos, Info{Name: name})
	}
	return infos, nil
}

func (osBackend) Open(config Config) (Port, error) {
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
