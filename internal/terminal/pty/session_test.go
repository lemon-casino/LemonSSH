package pty

import (
	"context"
	"errors"
	"io"
	"testing"
)

type fakeProcess struct{ killed, waited bool }

func (p *fakeProcess) Read([]byte) (int, error)       { return 0, io.EOF }
func (p *fakeProcess) Write(data []byte) (int, error) { return len(data), nil }
func (p *fakeProcess) Close() error                   { return nil }
func (p *fakeProcess) Resize(uint16, uint16) error    { return nil }
func (p *fakeProcess) Interrupt() error               { return nil }
func (p *fakeProcess) Kill() error                    { p.killed = true; return nil }
func (p *fakeProcess) Wait() error                    { p.waited = true; return nil }
func (p *fakeProcess) PID() int                       { return 1 }

type fakeBackend struct{ process *fakeProcess }

func (b *fakeBackend) Start(context.Context, Config) (Process, error) { return b.process, nil }

func TestSessionLifecycleAndGeneration(t *testing.T) {
	process := &fakeProcess{}
	session := NewSession(BuildConfig("s1", "", "", nil, nil, 80, 24))
	if err := session.Start(context.Background(), &fakeBackend{process: process}); err != nil {
		t.Fatal(err)
	}
	generation := session.Generation()
	if _, err := session.Write(generation, []byte("ls\n")); err != nil {
		t.Fatal(err)
	}
	if err := session.Resize(generation, 100, 30); err != nil {
		t.Fatal(err)
	}
	if err := session.Interrupt(generation); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Write(generation+1, []byte("stale")); !errors.Is(err, ErrGenerationStale) {
		t.Fatalf("stale write accepted: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if !process.killed || !process.waited {
		t.Fatal("close must kill and reap process")
	}
}

func TestReconnectFencesOldGeneration(t *testing.T) {
	first := &fakeProcess{}
	session := NewSession(BuildConfig("s1", "/bin/sh", "", nil, nil, 80, 24))
	if err := session.Start(context.Background(), &fakeBackend{process: first}); err != nil {
		t.Fatal(err)
	}
	old := session.Generation()
	generation, err := session.Reconnect()
	if err != nil || generation != old+1 {
		t.Fatalf("reconnect: generation=%d err=%v", generation, err)
	}
	if !first.killed || !first.waited {
		t.Fatal("reconnect must reap old process")
	}
}

func TestDefaultsAndInvalidResize(t *testing.T) {
	config := BuildConfig("s1", "", "/path/does/not/exist", nil, nil, 0, 0)
	if config.Shell == "" || config.CWD == "" {
		t.Fatalf("defaults missing: %+v", config)
	}
	session := NewSession(config)
	process := &fakeProcess{}
	if err := session.Start(context.Background(), &fakeBackend{process: process}); err != nil {
		t.Fatal(err)
	}
	if err := session.Resize(session.Generation(), 0, 24); err == nil {
		t.Fatal("zero cols accepted")
	}
}
