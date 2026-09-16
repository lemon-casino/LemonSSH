package terminaluse

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/binaricat/netcatty/internal/terminal/serialport"
	"github.com/binaricat/netcatty/internal/terminal/ymodem"
)

// SerialStartRequest is the shell-facing serial open payload. The client
// forwards the full line configuration; the serial owner validates it and
// fails closed on combinations the backend cannot honour.
type SerialStartRequest struct {
	Path        string `json:"path"`
	BaudRate    int    `json:"baudRate"`
	DataBits    int    `json:"dataBits"`
	StopBits    string `json:"stopBits"`
	Parity      string `json:"parity"`
	FlowControl string `json:"flowControl"`
}

// ListSerialPorts enumerates OS serial devices.
func (s *Service) ListSerialPorts() ([]serialport.Info, error) {
	return serialport.NewOSBackend().List()
}

// StartSerial opens a serial port and streams bytes onto the data plane.
func (s *Service) StartSerial(request SerialStartRequest) (string, error) {
	if request.Path == "" {
		return "", fmt.Errorf("serial path is required")
	}
	config := serialport.DefaultConfig(request.Path)
	if request.BaudRate > 0 {
		config.BaudRate = request.BaudRate
	}
	if request.DataBits != 0 {
		config.DataBits = request.DataBits
	}
	if request.StopBits != "" {
		config.StopBits = request.StopBits
	}
	if request.Parity != "" {
		config.Parity = request.Parity
	}
	if request.FlowControl != "" {
		config.FlowControl = request.FlowControl
	}
	backend := serialport.NewOSBackend()
	if err := backend.Open(config); err != nil {
		return "", err
	}
	s.mu.Lock()
	s.counter++
	sessionID := fmt.Sprintf("serial-%d", s.counter)
	s.mu.Unlock()
	bootstrap, err := s.controller.Open(sessionID)
	if err != nil {
		_ = backend.Close(request.Path)
		return "", err
	}
	s.mu.Lock()
	s.sessions[sessionID] = &terminalSession{serial: backend, serialID: request.Path, bootstrap: bootstrap}
	s.mu.Unlock()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := backend.Read(request.Path, buf)
			if n > 0 {
				if !s.publishOutput(sessionID, buf[:n]) {
					return
				}
			}
			if readErr != nil {
				s.Close(sessionID)
				return
			}
		}
	}()
	return sessionID, nil
}

// SendSerialYmodem uploads the file at filePath over the session's serial port
// using the YMODEM block protocol.
func (s *Service) SendSerialYmodem(sessionID, filePath string) (ymodem.SendResult, error) {
	term, ok := s.lookup(sessionID)
	if !ok {
		return ymodem.SendResult{}, fmt.Errorf("session %q not found", sessionID)
	}
	if term.serial == nil {
		return ymodem.SendResult{}, fmt.Errorf("session %q is not a serial session", sessionID)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ymodem.SendResult{}, fmt.Errorf("read transfer source: %w", err)
	}
	ctx, cancel := s.beginSerialTransfer(sessionID, term)
	defer cancel()
	stream := serialStream{session: term.serial, port: term.serialID}
	return ymodem.Send(ctx, stream, ymodem.SendOptions{
		FileName: filepath.Base(filePath),
		Data:     data,
	})
}

// ReceiveSerialYmodem downloads files from the serial peer into destinationDir.
func (s *Service) ReceiveSerialYmodem(sessionID, destinationDir string) ([]ymodem.ReceiveResult, error) {
	term, ok := s.lookup(sessionID)
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	if term.serial == nil {
		return nil, fmt.Errorf("session %q is not a serial session", sessionID)
	}
	ctx, cancel := s.beginSerialTransfer(sessionID, term)
	defer cancel()
	stream := serialStream{session: term.serial, port: term.serialID}
	return ymodem.Receive(ctx, stream, ymodem.ReceiveOptions{DestinationDir: destinationDir})
}

// beginSerialTransfer registers a cancellation handle for one serial transfer
// so CancelZmodem can abort it, and returns the transfer context.
func (s *Service) beginSerialTransfer(sessionID string, term *terminalSession) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	term.serialYmodemCancel = cancel
	s.sessions[sessionID] = term
	s.mu.Unlock()
	return ctx, func() {
		cancel()
		s.mu.Lock()
		if current, ok := s.sessions[sessionID]; ok && current == term {
			term.serialYmodemCancel = nil
		}
		s.mu.Unlock()
	}
}

// serialStream adapts an open serial session to the YMODEM ByteStream. Reads
// return the bounded-timeout behaviour the engine expects; a timeout surfaces
// as an error so the engine's retry loop can re-NACK or give up.
type serialStream struct {
	session *serialport.Session
	port    string
}

func (s serialStream) Read(p []byte) (int, error) {
	n, err := s.session.Read(s.port, p)
	if err != nil {
		return n, err
	}
	if n == 0 {
		// The serial backend's read timeout yields a zero-byte read. Report it
		// as a timeout so the engine does not spin on a busy loop.
		return 0, os.ErrDeadlineExceeded
	}
	return n, nil
}

func (s serialStream) Write(p []byte) (int, error) { return s.session.Write(s.port, p) }
