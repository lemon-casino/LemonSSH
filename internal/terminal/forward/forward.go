// Package forward owns port forwarding (P3-07): local, remote and dynamic
// SOCKS5 forwarding over an SSH transport. The tunnel function is injectable
// so lifecycle/registry semantics are testable without a live SSH server.
// State follows process epoch + monotonic revision with snapshot+subscribe.
package forward

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
)

var (
	ErrForwardExists   = errors.New("forward id already exists")
	ErrForwardNotFound = errors.New("forward not found")
	ErrUnsupportedKind = errors.New("unsupported forward kind")
	ErrPortInvalid     = errors.New("forward port invalid")
	ErrTunnelMissing   = errors.New("tunnel function is required")
)

type Kind string

const (
	KindLocal   Kind = "local"   // listen local, tunnel to target via SSH
	KindRemote  Kind = "remote"  // ask SSH server to listen, forward to local target
	KindDynamic Kind = "dynamic" // local SOCKS5 proxy through SSH
)

type Spec struct {
	ID         string `json:"id"`
	Kind       Kind   `json:"kind"`
	BindHost   string `json:"bindHost"`
	BindPort   uint16 `json:"bindPort"`
	TargetHost string `json:"targetHost"`
	TargetPort uint16 `json:"targetPort"`
	RuleID     string `json:"ruleId,omitempty"`
}

type State struct {
	Spec       Spec   `json:"spec"`
	Revision   uint64 `json:"revision"`
	ActiveConn int    `json:"activeConnections"`
	Err        string `json:"error,omitempty"`
}

// TunnelFunc connects one accepted local connection to its target through the
// SSH transport. In production this is ssh.Client.Dial over a pool lease.
type TunnelFunc func(ctx context.Context, targetHost string, targetPort uint16, local net.Conn) error

func writeSOCKS5Reply(client net.Conn, status byte) error {
	_, err := client.Write([]byte{0x05, status, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	return err
}

type forwardEntry struct {
	spec     Spec
	listener net.Listener
	revision uint64
	active   int
	errText  string
	cancel   context.CancelFunc
	ctx      context.Context
}

// Manager owns all forwarding for the process.
type Manager struct {
	mu           sync.Mutex
	entries      map[string]*forwardEntry
	revision     uint64
	tunnel       TunnelFunc
	listenRemote func(ctx context.Context, spec Spec) (net.Listener, error)
	subscribe    []func(State)
}

func NewManager(tunnel TunnelFunc) (*Manager, error) {
	if tunnel == nil {
		return nil, ErrTunnelMissing
	}
	return &Manager{entries: make(map[string]*forwardEntry), tunnel: tunnel}, nil
}

func proxyCopy(left, right net.Conn) {
	defer left.Close()
	defer right.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(left, right); done <- struct{}{} }()
	go func() { _, _ = io.Copy(right, left); done <- struct{}{} }()
	<-done
}

func NewManagerForRemote(listenRemote func(ctx context.Context, spec Spec) (net.Listener, error), tunnel TunnelFunc) (*Manager, error) {
	manager, err := NewManager(tunnel)
	if err != nil {
		return nil, err
	}
	manager.listenRemote = listenRemote
	return manager, nil
}

// Subscribe registers a state-change listener (invoked after each revision).
func (m *Manager) Subscribe(listener func(State)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribe = append(m.subscribe, listener)
}

// Start binds and begins forwarding.
func (m *Manager) Start(ctx context.Context, spec Spec) (State, error) {
	switch spec.Kind {
	case KindLocal, KindRemote, KindDynamic:
	default:
		return State{}, ErrUnsupportedKind
	}
		if spec.Kind != KindDynamic && spec.TargetPort == 0 {
			return State{}, ErrPortInvalid
		}
		var listener net.Listener
		var err error
		if spec.Kind == KindRemote {
			if m.listenRemote == nil {
				return State{}, fmt.Errorf("%w: remote listen", ErrUnsupportedKind)
			}
			listener, err = m.listenRemote(ctx, spec)
			if err != nil {
				return State{}, fmt.Errorf("remote listen %s:%d: %w", spec.BindHost, spec.BindPort, err)
			}
		} else {
			bindAddr := net.JoinHostPort(spec.BindHost, fmt.Sprintf("%d", spec.BindPort))
			listener, err = net.Listen("tcp", bindAddr)
			if err != nil {
				return State{}, fmt.Errorf("bind %s: %w", bindAddr, err)
			}
		}
	if actualAddr, ok := listener.Addr().(*net.TCPAddr); ok {
		spec.BindPort = uint16(actualAddr.Port)
	}
	m.mu.Lock()
	if _, exists := m.entries[spec.ID]; exists {
		m.mu.Unlock()
		_ = listener.Close()
		return State{}, ErrForwardExists
	}
	m.revision++
	runCtx, cancel := context.WithCancel(ctx)
	entry := &forwardEntry{spec: spec, listener: listener, revision: m.revision, ctx: runCtx, cancel: cancel}
	m.entries[spec.ID] = entry
	subscribers := append([]func(State){}, m.subscribe...)
	m.mu.Unlock()

	go m.acceptLoop(entry)
	state, _ := m.Snapshot(spec.ID)
	for _, listener := range subscribers {
		listener(state)
	}
	return state, nil
}

func (m *Manager) acceptLoop(entry *forwardEntry) {
	for {
		connection, err := entry.listener.Accept()
		if err != nil {
			m.mu.Lock()
			entry.errText = err.Error()
			m.mu.Unlock()
			return
		}
		m.mu.Lock()
		entry.active++
		m.mu.Unlock()
		go func() {
			defer func() {
				_ = connection.Close()
				m.mu.Lock()
				entry.active--
				m.mu.Unlock()
				}()
				switch entry.spec.Kind {
				case KindLocal:
					_ = m.tunnel(entry.ctx, entry.spec.TargetHost, entry.spec.TargetPort, connection)
				case KindDynamic:
					_ = serveSOCKS(entry.ctx, connection, m.tunnel)
				case KindRemote:
					target, dialErr := net.Dial("tcp", net.JoinHostPort(entry.spec.TargetHost, fmt.Sprintf("%d", entry.spec.TargetPort)))
					if dialErr != nil {
						return
					}
					proxyCopy(connection, target)
				}
		}()
	}
}

func serveSOCKS(ctx context.Context, client net.Conn, tunnel TunnelFunc) error {
	version := make([]byte, 1)
	if _, err := io.ReadFull(client, version); err != nil {
		return err
	}
	switch version[0] {
	case 0x05:
		return serveSOCKS5(ctx, client, tunnel)
	case 0x04:
		return serveSOCKS4(ctx, client, tunnel, version[0])
	default:
		return fmt.Errorf("socks: unsupported version %d", version[0])
	}
}

func serveSOCKS4(ctx context.Context, client net.Conn, tunnel TunnelFunc, version byte) error {
	header := make([]byte, 7)
	if _, err := io.ReadFull(client, header); err != nil {
		return err
	}
	command := header[0]
	port := binary.BigEndian.Uint16(header[1:3])
	ip := net.IP(header[3:7])
	for {
		next := make([]byte, 1)
		if _, err := io.ReadFull(client, next); err != nil {
			return err
		}
		if next[0] == 0 {
			break
		}
	}
	host := ip.String()
	if ip[0] == 0 && ip[1] == 0 && ip[2] == 0 && ip[3] != 0 {
		var name []byte
		for {
			next := make([]byte, 1)
			if _, err := io.ReadFull(client, next); err != nil {
				return err
			}
			if next[0] == 0 {
				break
			}
			name = append(name, next[0])
		}
		host = string(name)
	}
	if command != 0x01 {
		_, _ = client.Write([]byte{0x00, 0x5b, 0, 0, 0, 0, 0, 0})
		return errors.New("socks4: only CONNECT supported")
	}
	if _, err := client.Write([]byte{0x00, 0x5a, header[1], header[2], header[3], header[4], header[5], header[6]}); err != nil {
		return err
	}
	_ = version
	return tunnel(ctx, host, port, client)
}

// serveSOCKS5 implements the no-auth CONNECT variant of SOCKS5 and tunnels the
// connection through the SSH transport.
func serveSOCKS5(ctx context.Context, client net.Conn, tunnel TunnelFunc) error {
	nmethods := make([]byte, 1)
	if _, err := io.ReadFull(client, nmethods); err != nil {
		return err
	}
	methods := make([]byte, nmethods[0])
	if _, err := io.ReadFull(client, methods); err != nil {
		return err
	}
	if _, err := client.Write([]byte{0x05, 0x00}); err != nil {
		return err
	}
	request := make([]byte, 4)
	if _, err := io.ReadFull(client, request); err != nil {
		return err
	}
	if request[0] != 0x05 || request[1] != 0x01 {
		_ = writeSOCKS5Reply(client, 0x07)
		return errors.New("socks5: only CONNECT supported")
	}
	var host string
	switch request[3] {
	case 0x01:
		addr := make([]byte, 4)
		if _, err := io.ReadFull(client, addr); err != nil {
			return err
		}
		host = net.IP(addr).String()
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(client, length); err != nil {
			return err
		}
		name := make([]byte, length[0])
		if _, err := io.ReadFull(client, name); err != nil {
			return err
		}
		host = string(name)
	case 0x04:
		addr := make([]byte, 16)
		if _, err := io.ReadFull(client, addr); err != nil {
			return err
		}
		host = net.IP(addr).String()
	default:
		_ = writeSOCKS5Reply(client, 0x08)
		return errors.New("socks5: unsupported address type")
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(client, portBytes); err != nil {
		return err
	}
	port := binary.BigEndian.Uint16(portBytes)
		if err := writeSOCKS5Reply(client, 0x00); err != nil {
			return err
		}
	return tunnel(ctx, host, port, client)
}

// Stop terminates one forward.
func (m *Manager) Stop(id string) (State, error) {
	m.mu.Lock()
	entry, ok := m.entries[id]
	if !ok {
		m.mu.Unlock()
		return State{}, ErrForwardNotFound
	}
	m.revision++
	entry.revision = m.revision
	entry.cancel()
	_ = entry.listener.Close()
	state := State{Spec: entry.spec, Revision: entry.revision, ActiveConn: entry.active}
	listeners := append([]func(State){}, m.subscribe...)
	delete(m.entries, id)
	m.mu.Unlock()
	for _, listener := range listeners {
		listener(state)
	}
	return state, nil
}

// StopAll terminates every forward (shutdown path).
func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.entries))
	for id := range m.entries {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		_, _ = m.Stop(id)
	}
}

// Snapshot reports one forward's state.
func (m *Manager) Snapshot(id string) (State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.entries[id]
	if !ok {
		return State{}, ErrForwardNotFound
	}
	return State{Spec: entry.spec, Revision: entry.revision, ActiveConn: entry.active, Err: entry.errText}, nil
}

// List reports all forwards.
func (m *Manager) List() []State {
	m.mu.Lock()
	defer m.mu.Unlock()
	states := make([]State, 0, len(m.entries))
	for _, entry := range m.entries {
		states = append(states, State{Spec: entry.spec, Revision: entry.revision, ActiveConn: entry.active, Err: entry.errText})
	}
	return states
}
