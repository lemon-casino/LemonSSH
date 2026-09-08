package sshpool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	netcattyssh "github.com/binaricat/netcatty/internal/terminal/ssh"
)

type recordingDial struct {
	mu    sync.Mutex
	calls int32
}

func (d *recordingDial) dial(ctx context.Context, config netcattyssh.DialConfig) (*netcattyssh.Transport, error) {
	atomic.AddInt32(&d.calls, 1)
	return &netcattyssh.Transport{}, nil
}
func (d *recordingDial) count() int { return int(atomic.LoadInt32(&d.calls)) }

func baseConfig() netcattyssh.DialConfig {
	return netcattyssh.DialConfig{
		Hostname: "host1",
		Port:     22,
		Username: "user",
		Auth:     netcattyssh.AuthMethod{Password: "secret"},
	}
}

func TestCompatibilityKeySensitivity(t *testing.T) {
	first, err := CompatibilityKey(baseConfig())
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompatibilityKey(baseConfig())
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("identical configs must share a key")
	}
	changedPassword := baseConfig()
	changedPassword.Auth.Password = "different"
	third, err := CompatibilityKey(changedPassword)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("different auth must not share a key")
	}
	changedHost := baseConfig()
	changedHost.Hostname = "host2"
	fourth, _ := CompatibilityKey(changedHost)
	if fourth == first {
		t.Fatal("different host must not share a key")
	}
	jumped := baseConfig()
	jumped.JumpHosts = []netcattyssh.DialConfig{changedHost}
	fifth, _ := CompatibilityKey(jumped)
	if fifth == first {
		t.Fatal("jump chain must change the key")
	}
}

func TestPoolSharesTransportAndSingleFlight(t *testing.T) {
	dial := &recordingDial{}
	pool := New(dial.dial)
	defer pool.Shutdown()
	ctx := context.Background()

	leaseShell, err := pool.Get(ctx, baseConfig(), KindShell)
	if err != nil {
		t.Fatal(err)
	}
	leaseSFTP, err := pool.Get(ctx, baseConfig(), KindSFTP)
	if err != nil {
		t.Fatal(err)
	}
	if leaseShell.entry != leaseSFTP.entry {
		t.Fatal("kinds must share one transport")
	}
	if dial.count() != 1 {
		t.Fatalf("expected single-flight dial, got %d", dial.count())
	}
	leaseShell.Return()
	leaseShellAgain, err := pool.Get(ctx, baseConfig(), KindShell)
	if err != nil {
		t.Fatal(err)
	}
	if dial.count() != 1 {
		t.Fatal("returned healthy transport must be reused")
	}
	leaseSFTP.Discard()
	leaseShellAgain.Return()
	if pool.Size() != 0 {
		t.Fatalf("discarded transports must be dropped, got %d", pool.Size())
	}
}

func TestPoolConcurrentSingleFlight(t *testing.T) {
	dial := &recordingDial{}
	pool := New(dial.dial)
	defer pool.Shutdown()
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lease, err := pool.Get(ctx, baseConfig(), KindShell)
			if err != nil {
				errs <- err
				return
			}
			lease.Return()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if dial.count() != 1 {
		t.Fatalf("concurrent Get must single-flight, got %d dials", dial.count())
	}
}

func TestPoolIdleTTLEvictionAndShutdown(t *testing.T) {
	dial := &recordingDial{}
	pool := New(dial.dial, WithIdleTTL(10*time.Millisecond), WithMaxIdle(1))
	lease, err := pool.Get(context.Background(), baseConfig(), KindForward)
	if err != nil {
		t.Fatal(err)
	}
	lease.Return()
	if pool.Size() != 1 {
		t.Fatalf("idle transport must stay pooled, got %d", pool.Size())
	}
	time.Sleep(30 * time.Millisecond)
	next, err := pool.Get(context.Background(), baseConfig(), KindShell)
	if err != nil {
		t.Fatal(err)
	}
	if pool.Size() != 1 || dial.count() != 2 {
		t.Fatalf("expired transport must be redialed: size=%d dials=%d", pool.Size(), dial.count())
	}
	next.Return()
	if err := pool.Shutdown(); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Get(context.Background(), baseConfig(), KindShell); !errors.Is(err, ErrPoolClosed) {
		t.Fatalf("closed pool must reject, got %v", err)
	}
}

func TestPoolAuthChangeDialsSeparately(t *testing.T) {
	dial := &recordingDial{}
	pool := New(dial.dial)
	defer pool.Shutdown()
	ctx := context.Background()
	first, err := pool.Get(ctx, baseConfig(), KindShell)
	if err != nil {
		t.Fatal(err)
	}
	changed := baseConfig()
	changed.Auth.Password = "other"
	second, err := pool.Get(ctx, changed, KindShell)
	if err != nil {
		t.Fatal(err)
	}
	if first.entry == second.entry {
		t.Fatal("different auth must not share a transport")
	}
	if dial.count() != 2 {
		t.Fatalf("expected two dials, got %d", dial.count())
	}
	first.Return()
	second.Return()
}

func TestPoolForwardingTransportsAreSingleUse(t *testing.T) {
	dial := &recordingDial{}
	pool := New(dial.dial)
	defer pool.Shutdown()
	ctx := context.Background()

	forwarding := baseConfig()
	forwarding.ForwardAgent = true
	lease, err := pool.Get(ctx, forwarding, KindShell)
	if err != nil {
		t.Fatal(err)
	}
	lease.Return()
	if pool.Size() != 0 {
		t.Fatalf("forwarding transport must not re-enter the pool, got %d", pool.Size())
	}

	again, err := pool.Get(ctx, forwarding, KindShell)
	if err != nil {
		t.Fatal(err)
	}
	again.Return()
	if dial.count() != 2 {
		t.Fatalf("forwarding transports must redial every time, got %d", dial.count())
	}

	plain, err := pool.Get(ctx, baseConfig(), KindShell)
	if err != nil {
		t.Fatal(err)
	}
	plain.Return()
	if pool.Size() != 1 {
		t.Fatal("non-forwarding transports must stay pooled")
	}
	if dial.count() != 3 {
		t.Fatalf("unexpected dial count %d", dial.count())
	}
}

func TestCompatibilityKeyIncludesForwarding(t *testing.T) {
	base := baseConfig()
	forwarding := baseConfig()
	forwarding.ForwardAgent = true
	first, err := CompatibilityKey(base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompatibilityKey(forwarding)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("forwarding must change the compatibility key")
	}
}
