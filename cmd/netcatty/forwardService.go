package main

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/binaricat/netcatty/internal/terminal/forward"
	netcattyssh "github.com/binaricat/netcatty/internal/terminal/ssh"
	"github.com/binaricat/netcatty/internal/terminal/sshpool"
)

type ForwardService struct {
	mu         sync.Mutex
	pool       *sshpool.Pool
	knownHosts *netcattyssh.KnownHosts
	manager    *forward.Manager
	lease      *sshpool.Lease
}

func NewForwardService(pool *sshpool.Pool, knownHosts *netcattyssh.KnownHosts) *ForwardService {
	return &ForwardService{pool: pool, knownHosts: knownHosts}
}

func (s *ForwardService) ensureManager(host string, port uint16, username, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.manager != nil {
		return nil
	}
	if port == 0 {
		port = 22
	}
	config := netcattyssh.DialConfig{
		Hostname:      host,
		Port:          port,
		Username:      username,
		Auth:          netcattyssh.AuthMethod{Password: password},
		HostKeyPolicy: netcattyssh.StrictPolicy(s.knownHosts),
	}
	lease, err := s.pool.Get(context.Background(), config, sshpool.KindForward)
	if err != nil {
		return err
	}
	client := lease.Client()
	manager, err := forward.NewManager(func(ctx context.Context, targetHost string, targetPort uint16, local net.Conn) error {
		remote, dialErr := client.Dial("tcp", net.JoinHostPort(targetHost, fmt.Sprintf("%d", targetPort)))
		if dialErr != nil {
			return dialErr
		}
		go proxyConn(local, remote)
		return nil
	})
	if err != nil {
		lease.Discard()
		return err
	}
	s.lease = lease
	s.manager = manager
	return nil
}

func proxyConn(left, right net.Conn) {
	defer left.Close()
	defer right.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = ioCopy(left, right); done <- struct{}{} }()
	go func() { _, _ = ioCopy(right, left); done <- struct{}{} }()
	<-done
}

func ioCopy(dst net.Conn, src net.Conn) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := src.Read(buf)
		if n > 0 {
			written, writeErr := dst.Write(buf[:n])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
		}
		if err != nil {
			return total, err
		}
	}
}

func (s *ForwardService) Start(id, kind, bindHost string, bindPort uint16, targetHost string, targetPort uint16, sshHost string, sshPort uint16, username, password string) (forward.State, error) {
	if err := s.ensureManager(sshHost, sshPort, username, password); err != nil {
		return forward.State{}, err
	}
	return s.manager.Start(context.Background(), forward.Spec{
		ID:         id,
		Kind:       forward.Kind(kind),
		BindHost:   bindHost,
		BindPort:   bindPort,
		TargetHost: targetHost,
		TargetPort: targetPort,
	})
}

func (s *ForwardService) Stop(id string) (forward.State, error) {
	s.mu.Lock()
	manager := s.manager
	s.mu.Unlock()
	if manager == nil {
		return forward.State{}, forward.ErrForwardNotFound
	}
	return manager.Stop(id)
}

func (s *ForwardService) List() []forward.State {
	s.mu.Lock()
	manager := s.manager
	s.mu.Unlock()
	if manager == nil {
		return nil
	}
	return manager.List()
}

func (s *ForwardService) Snapshot(id string) (forward.State, error) {
	s.mu.Lock()
	manager := s.manager
	s.mu.Unlock()
	if manager == nil {
		return forward.State{}, forward.ErrForwardNotFound
	}
	return manager.Snapshot(id)
}
