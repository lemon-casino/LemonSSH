package main

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestAutomaticZmodemReceiveDetection(t *testing.T) {
	s := &TerminalService{sessions: map[string]*terminalSession{"term": {}}}
	detected := false
	s.setEventEmitter(func(name string, payload any) {
		if name == "terminal:zmodem" {
			detected = true
		}
	})
	// lrzsz ZRQINIT hex header with zero position and zero CRC.
	header := []byte("**\x18B00000000000000\r\n\x11")
	if !s.detectZmodem("term", header[:5]) {
		t.Fatal("partial header leaked")
	}
	if !s.detectZmodem("term", header[5:]) || !detected {
		t.Fatal("valid ZRQINIT not detected")
	}
	term, _ := s.lookup("term")
	if term.zmodem == nil {
		t.Fatal("no capture installed")
	}
	s.endZmodem("term", term.zmodem)
}

func TestMoshHandshakeReadDeadline(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	done := make(chan error, 1)
	go func() { _, err := scanConnectDeadline(r, 10*time.Millisecond, func() { r.Close() }); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("deadline ignored")
		}
	case <-time.After(time.Second):
		t.Fatal("handshake read hung")
	}
}

func TestZmodemReadHonorsStepDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	stream := &terminalZmodemStream{PipeReader: r, writer: w, ctx: ctx, cancel: cancel}
	step, stop := context.WithTimeout(ctx, 10*time.Millisecond)
	defer stop()
	done := make(chan error, 1)
	go func() { _, err := stream.readContext(step, make([]byte, 1)); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("deadline ignored")
		}
	case <-time.After(time.Second):
		t.Fatal("read hung past deadline")
	}
}

func TestZmodemCaptureDivertsTerminalBytes(t *testing.T) {
	s := &TerminalService{sessions: map[string]*terminalSession{"term": {}}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stream, err := s.beginZmodem("term", ctx, cancel)
	if err != nil {
		t.Fatal(err)
	}
	defer s.endZmodem("term", stream)
	done := make(chan bool, 1)
	go func() { done <- s.publishOutput("term", []byte("wire")) }()
	data := make([]byte, 4)
	if _, err := io.ReadFull(stream, data); err != nil || string(data) != "wire" {
		t.Fatalf("captured %q %v", data, err)
	}
	if !<-done {
		t.Fatal("wire rejected")
	}
	if _, err := s.beginZmodem("term", ctx, cancel); err == nil {
		t.Fatal("concurrent protocol session accepted")
	}
}
