package forward

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// echoTunnel pretends to be the SSH relay: every byte written to the local
// connection is echoed back, proving the SOCKS5/local plumbing end to end.
func echoTunnel(ctx context.Context, host string, port uint16, local net.Conn) error {
	remote, err := net.Dial("tcp", net.JoinHostPort(host, itoa(int(port))))
	if err != nil {
		return err
	}
	defer remote.Close()
	done := make(chan error, 2)
	go func() { _, err := io.Copy(remote, local); done <- err }()
	go func() { _, err := io.Copy(local, remote); done <- err }()
	<-done
	return nil
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

func TestManagerLifecycleAndRevision(t *testing.T) {
	// Backing service the forwards point at.
	service, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	go func() {
		for {
			conn, err := service.Accept()
			if err != nil {
				return
			}
			go func() {
				_, _ = io.Copy(conn, conn)
				_ = conn.Close()
			}()
		}
	}()
	serviceAddr := service.Addr().String()
	serviceHost, servicePortRaw, _ := net.SplitHostPort(serviceAddr)
	servicePort := 0
	_, _ = fmt.Sscanf(servicePortRaw, "%d", &servicePort)

	manager, err := NewManager(echoTunnel)
	if err != nil {
		t.Fatal(err)
	}
	var revisionNotes []uint64
	manager.Subscribe(func(state State) { revisionNotes = append(revisionNotes, state.Revision) })

	state, err := manager.Start(context.Background(), Spec{
		ID: "f1", Kind: KindLocal, BindHost: "127.0.0.1", BindPort: 0,
		TargetHost: serviceHost, TargetPort: uint16(servicePort),
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision != 1 {
		t.Fatalf("first revision must be 1, got %d", state.Revision)
	}
	snapshot, err := manager.Snapshot("f1")
	if err != nil || snapshot.Spec.ID != "f1" {
		t.Fatalf("snapshot: %+v (%v)", snapshot, err)
	}
	// Duplicate ID rejected.
	if _, err := manager.Start(context.Background(), Spec{ID: "f1", Kind: KindLocal, BindHost: "127.0.0.1", BindPort: 0, TargetHost: serviceHost, TargetPort: uint16(servicePort)}); !errors.Is(err, ErrForwardExists) {
		t.Fatalf("duplicate id must fail, got %v", err)
	}
	if _, err := manager.Start(context.Background(), Spec{ID: "f2", Kind: "weird", BindHost: "127.0.0.1", BindPort: 0, TargetHost: "x", TargetPort: 1}); !errors.Is(err, ErrUnsupportedKind) {
		t.Fatalf("unsupported kind must fail, got %v", err)
	}

	// Round-trip bytes through the local forward to the echo service.
	port := snapshot.Spec.BindPort
	targetConn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", itoa(int(port))))
	if err != nil {
		t.Fatal(err)
	}
	probe := []byte("ping-through-forward")
	if _, err := targetConn.Write(probe); err != nil {
		t.Fatal(err)
	}
	_ = targetConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	got := make([]byte, len(probe))
	if _, err := io.ReadFull(targetConn, got); err != nil {
		t.Fatalf("forward echo failed: %v", err)
	}
	if !bytes.Equal(got, probe) {
		t.Fatalf("echo mismatch: %s", got)
	}
	_ = targetConn.Close()

	if _, err := manager.Stop("f1"); err != nil {
		t.Fatal(err)
	}
	if len(revisionNotes) == 0 {
		t.Fatal("subscribers must observe revisions")
	}
	if _, err := manager.Snapshot("f1"); !errors.Is(err, ErrForwardNotFound) {
		t.Fatalf("stopped forward must disappear, got %v", err)
	}
}

func TestDynamicSOCKS5RoundTrip(t *testing.T) {
	service, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	go func() {
		for {
			conn, err := service.Accept()
			if err != nil {
				return
			}
			go func() {
				_, _ = io.Copy(conn, conn)
				_ = conn.Close()
			}()
		}
	}()
	serviceHost, servicePortRaw, _ := net.SplitHostPort(service.Addr().String())
	servicePort := 0
	_, _ = fmt.Sscanf(servicePortRaw, "%d", &servicePort)

	manager, err := NewManager(echoTunnel)
	if err != nil {
		t.Fatal(err)
	}
	state, err := manager.Start(context.Background(), Spec{
		ID: "socks", Kind: KindDynamic, BindHost: "127.0.0.1", BindPort: 0,
		TargetHost: serviceHost, TargetPort: uint16(servicePort),
	})
	if err != nil {
		t.Fatal(err)
	}
	socksConn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", itoa(int(state.Spec.BindPort))))
	if err != nil {
		t.Fatal(err)
	}
	defer socksConn.Close()
	// Greeting: version 5, one method, no-auth.
	if _, err := socksConn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatal(err)
	}
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(socksConn, greeting); err != nil {
		t.Fatal(err)
	}
	if greeting[0] != 0x05 || greeting[1] != 0x00 {
		t.Fatalf("socks greeting mismatch: %v", greeting)
	}
	request := []byte{0x05, 0x01, 0x00, 0x03, byte(len(serviceHost))}
	request = append(request, serviceHost...)
	var portBytes [2]byte
	binary.BigEndian.PutUint16(portBytes[:], uint16(servicePort))
	request = append(request, portBytes[:]...)
	if _, err := socksConn.Write(request); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(socksConn, reply); err != nil {
		t.Fatal(err)
	}
	if reply[1] != 0x00 {
		t.Fatalf("socks connect rejected: %v", reply[1])
	}
	probe := []byte("socks-roundtrip")
	if _, err := socksConn.Write(probe); err != nil {
		t.Fatal(err)
	}
	_ = socksConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	got := make([]byte, len(probe))
	if _, err := io.ReadFull(socksConn, got); err != nil {
		t.Fatalf("socks tunnel echo failed: %v", err)
	}
	if !bytes.Equal(got, probe) {
		t.Fatal("socks echo mismatch")
	}
}

func TestStopAllAndConcurrency(t *testing.T) {
	manager, err := NewManager(func(context.Context, string, uint16, net.Conn) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = manager.Start(context.Background(), Spec{
				ID:         "f" + itoa(i),
				Kind:       KindLocal,
				BindHost:   "127.0.0.1",
				BindPort:   0,
				TargetHost: "127.0.0.1",
				TargetPort: 1,
			})
		}()
	}
	wg.Wait()
	if len(manager.List()) != 8 {
		t.Fatalf("expected 8 forwards, got %d", len(manager.List()))
	}
	manager.StopAll()
	if len(manager.List()) != 0 {
		t.Fatal("stop-all must clear the registry")
	}
}
