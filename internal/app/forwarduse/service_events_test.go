package forwarduse

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/binaricat/lemonssh/internal/app/terminaluse"
	lemonsshssh "github.com/binaricat/lemonssh/internal/terminal/ssh"
	"github.com/binaricat/lemonssh/internal/terminal/sshpool"
)

// runtimeEventCollector is the event collector used by the ordering tests:
// it records every RuntimeEvent the service delivers through the sink so the
// assertions can verify the exact delivery order.
type runtimeEventCollector struct {
	mu     sync.Mutex
	events []RuntimeEvent
}

func (c *runtimeEventCollector) record(event RuntimeEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *runtimeEventCollector) collected() []RuntimeEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]RuntimeEvent(nil), c.events...)
}

// assertContiguousRevisions asserts the collector received strictly ordered,
// contiguous revisions (1..N) for one epoch — the contract the renderer's
// gap detection relies on.
func assertContiguousRevisions(t *testing.T, events []RuntimeEvent, epoch string) {
	t.Helper()
	for i, event := range events {
		want := uint64(i + 1)
		if event.Epoch != epoch {
			t.Fatalf("event %d: epoch %q, want %q (events: %+v)", i, event.Epoch, epoch, events)
		}
		if event.Revision != want {
			t.Fatalf("event %d: revision %d, want contiguous %d (events: %+v)", i, event.Revision, want, events)
		}
	}
}

// startLoopbackSSHClient spins up a minimal in-memory SSH peer on a real
// loopback listener and returns the connected client. The fake peer grants
// "tcpip-forward" global requests so remote forward listeners open without
// touching any outside network.
func startLoopbackSSHClient(t *testing.T) *gossh.Client {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := &gossh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		serverSide, err := listener.Accept()
		if err != nil {
			return
		}
		defer serverSide.Close()
		_, channels, requests, err := gossh.NewServerConn(serverSide, serverConfig)
		if err != nil {
			return
		}
		go func() {
			for request := range requests {
				switch request.Type {
				case "tcpip-forward", "cancel-tcpip-forward":
					var payload struct {
						Addr string
						Port uint32
					}
					_ = gossh.Unmarshal(request.Payload, &payload)
					if request.WantReply {
						_ = request.Reply(true, gossh.Marshal(struct{ Port uint32 }{payload.Port}))
					}
				default:
					if request.WantReply {
						_ = request.Reply(false, nil)
					}
				}
			}
		}()
		for incoming := range channels {
			_ = incoming.Reject(gossh.Prohibited, "forward tests open no channels")
		}
	}()

	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{
		User:            "tester",
		Auth:            []gossh.AuthMethod{gossh.Password("unused")},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// newEventTestService builds a service over an injectable pool whose dial
// returns the in-memory loopback client, with the epoch stamped and the
// collector installed.
func newEventTestService(t *testing.T) (*Service, *runtimeEventCollector) {
	t.Helper()
	client := startLoopbackSSHClient(t)
	pool := sshpool.New(func(context.Context, lemonsshssh.DialConfig) (*lemonsshssh.Transport, error) {
		return &lemonsshssh.Transport{Client: client}, nil
	})
	t.Cleanup(func() { _ = pool.Shutdown() })
	service := New(pool, nil)
	service.SetRuntimeEpoch("wails")
	collector := &runtimeEventCollector{}
	service.OnRuntimeEvent(collector.record)
	return service, collector
}

func eventTestConnectRequest() terminaluse.SSHConnectRequest {
	skipVerify := false
	return terminaluse.SSHConnectRequest{
		Hostname:       "example.invalid",
		Port:           22,
		Username:       "tester",
		VerifyHostKeys: &skipVerify,
	}
}

// TestPublishDeliversEventsInRevisionOrder is the ordering regression test:
// Start/Stop share publish, so concurrent table mutations must still reach
// the sink strictly in revision order.
func TestPublishDeliversEventsInRevisionOrder(t *testing.T) {
	service := New(nil, nil)
	service.SetRuntimeEpoch("wails")
	collector := &runtimeEventCollector{}
	service.OnRuntimeEvent(collector.record)

	const writers = 8
	const perWriter = 25
	var wg sync.WaitGroup
	for writer := 0; writer < writers; writer++ {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				service.publish(func() (RuntimeEvent, bool) {
					return RuntimeEvent{
						Kind:     RuntimeEventKindUpsert,
						TunnelID: fmt.Sprintf("pf-rule-%d-%d", writer, i),
						RuleID:   fmt.Sprintf("rule-%d", writer),
					}, true
				})
			}
		}(writer)
	}
	wg.Wait()

	events := collector.collected()
	if len(events) != writers*perWriter {
		t.Fatalf("collector received %d events, want %d", len(events), writers*perWriter)
	}
	assertContiguousRevisions(t, events, "wails")
}

// TestPublishSkipsUnchangedTable: read-only operations claim no revision and
// emit no event.
func TestPublishSkipsUnchangedTable(t *testing.T) {
	service := New(nil, nil)
	service.SetRuntimeEpoch("wails")
	collector := &runtimeEventCollector{}
	service.OnRuntimeEvent(collector.record)

	service.publish(func() (RuntimeEvent, bool) { return RuntimeEvent{}, false })

	if events := collector.collected(); len(events) != 0 {
		t.Fatalf("unchanged table must not emit, got %+v", events)
	}
	if snapshot := service.RuntimeSnapshot("wails"); snapshot.Revision != 0 {
		t.Fatalf("unchanged table must not advance the revision, got %d", snapshot.Revision)
	}
}

// TestStartStopEmitsOrderedUpsertAndRemove drives the public Start/Stop API
// end to end (in-memory SSH peer) and asserts the event sequence the
// renderer's runtime subscription consumes.
func TestStartStopEmitsOrderedUpsertAndRemove(t *testing.T) {
	service, collector := newEventTestService(t)
	request := eventTestConnectRequest()

	const tunnelID = "pf-rule-1-1710000000"
	started := service.Start(tunnelID, "remote", "127.0.0.1", 18443, "target.internal", 22, request)
	if !started.Success {
		t.Fatalf("start failed: %s", started.Error)
	}

	events := collector.collected()
	if len(events) != 1 {
		t.Fatalf("want exactly one event after Start, got %d: %+v", len(events), events)
	}
	upsert := events[0]
	if upsert.Kind != RuntimeEventKindUpsert {
		t.Fatalf("want upsert event, got %q", upsert.Kind)
	}
	if upsert.Record == nil || upsert.Record.TunnelID != tunnelID || upsert.Record.RuleID != "rule-1" || upsert.Record.Phase != "active" {
		t.Fatalf("bad upsert payload: %+v", upsert)
	}
	assertContiguousRevisions(t, events, "wails")

	// The reuse path changes nothing and must stay silent.
	reused := service.Start(tunnelID, "remote", "127.0.0.1", 18443, "target.internal", 22, request)
	if !reused.Success || !reused.Reused {
		t.Fatalf("second start for one rule must reuse, got %+v", reused)
	}
	if events = collector.collected(); len(events) != 1 {
		t.Fatalf("reuse must not emit, got %+v", events)
	}

	// The snapshot must report the same revision the last event carried.
	snapshot := service.RuntimeSnapshot("wails")
	if snapshot.Revision != 1 || len(snapshot.Records) != 1 || snapshot.Records[0].TunnelID != tunnelID {
		t.Fatalf("bad snapshot after Start: %+v", snapshot)
	}

	stopped := service.Stop(tunnelID)
	if !stopped.Success || stopped.Status != "inactive" {
		t.Fatalf("stop failed: %+v", stopped)
	}
	events = collector.collected()
	if len(events) != 2 {
		t.Fatalf("want exactly two events after Stop, got %d: %+v", len(events), events)
	}
	remove := events[1]
	if remove.Kind != RuntimeEventKindRemove || remove.TunnelID != tunnelID || remove.RuleID != "rule-1" {
		t.Fatalf("bad remove payload: %+v", remove)
	}
	assertContiguousRevisions(t, events, "wails")

	// Stopping an already-stopped tunnel changes nothing.
	if stopped := service.Stop(tunnelID); !stopped.Success {
		t.Fatalf("stopping a missing tunnel must still succeed: %+v", stopped)
	}
	if events = collector.collected(); len(events) != 2 {
		t.Fatalf("stopping a missing tunnel must not emit, got %+v", events)
	}

	if final := service.RuntimeSnapshot("wails"); final.Revision != 2 || len(final.Records) != 0 {
		t.Fatalf("bad snapshot after Stop: %+v", final)
	}
}

// TestStopByRuleIdEmitsOrderedRemoves covers the rule teardown path the
// renderer's delete/import flows use: one ordered remove per matching tunnel,
// nothing for unknown rules.
func TestStopByRuleIdEmitsOrderedRemoves(t *testing.T) {
	service, collector := newEventTestService(t)
	request := eventTestConnectRequest()

	first := service.Start("pf-rule-1-1", "remote", "127.0.0.1", 18443, "target.internal", 22, request)
	second := service.Start("pf-rule-2-2", "remote", "127.0.0.1", 18444, "target.internal", 22, request)
	if !first.Success || !second.Success {
		t.Fatalf("starts failed: %+v %+v", first, second)
	}

	result := service.StopByRuleId("rule-1")
	if stopped, _ := result["stopped"].(int); stopped != 1 {
		t.Fatalf("StopByRuleId stopped %v, want 1", result["stopped"])
	}
	events := collector.collected()
	if len(events) != 3 {
		t.Fatalf("want three events after StopByRuleId, got %d: %+v", len(events), events)
	}
	remove := events[2]
	if remove.Kind != RuntimeEventKindRemove || remove.RuleID != "rule-1" || remove.TunnelID != first.TunnelID {
		t.Fatalf("bad StopByRuleId remove payload: %+v", remove)
	}
	assertContiguousRevisions(t, events, "wails")

	// Unknown rule: no stops, no events.
	result = service.StopByRuleId("rule-unknown")
	if stopped, _ := result["stopped"].(int); stopped != 0 {
		t.Fatalf("unknown rule stopped %v, want 0", result["stopped"])
	}
	if events = collector.collected(); len(events) != 3 {
		t.Fatalf("unknown rule must not emit, got %+v", events)
	}

	// Tearing the remaining tunnel down keeps the sequence contiguous.
	if stopped := service.Stop(second.TunnelID); !stopped.Success {
		t.Fatalf("stop failed: %+v", stopped)
	}
	assertContiguousRevisions(t, collector.collected(), "wails")
}

// TestOnRuntimeEventNilSinkDisablesDelivery: unwiring the sink must stop
// delivery without breaking Start/Stop.
func TestOnRuntimeEventNilSinkDisablesDelivery(t *testing.T) {
	service, collector := newEventTestService(t)
	request := eventTestConnectRequest()

	if started := service.Start("pf-rule-1-1", "remote", "127.0.0.1", 18443, "target.internal", 22, request); !started.Success {
		t.Fatalf("start failed: %s", started.Error)
	}
	service.OnRuntimeEvent(nil)
	if stopped := service.Stop("pf-rule-1-1"); !stopped.Success {
		t.Fatalf("stop failed: %+v", stopped)
	}
	if events := collector.collected(); len(events) != 1 {
		t.Fatalf("events after unwiring must stay at the pre-unwire count, got %+v", events)
	}
	if snapshot := service.RuntimeSnapshot("wails"); snapshot.Revision != 2 {
		t.Fatalf("stop after unwiring must still advance the revision, got %d", snapshot.Revision)
	}
}
