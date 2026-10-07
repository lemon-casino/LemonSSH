package transfer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memSource struct {
	data []byte
}

func (m *memSource) ReadChunk(_ context.Context, _ TaskSpec, chunk Chunk) ([]byte, error) {
	end := chunk.Offset + chunk.Length
	if end > int64(len(m.data)) {
		end = int64(len(m.data))
	}
	if chunk.Offset >= int64(len(m.data)) {
		return []byte{}, nil
	}
	return m.data[chunk.Offset:end], nil
}

type memSink struct {
	mu   sync.Mutex
	data []byte
}

func (m *memSink) WriteChunk(_ context.Context, _ TaskSpec, chunk Chunk, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if int64(len(m.data)) < chunk.Offset+int64(len(data)) {
		grown := make([]byte, chunk.Offset+int64(len(data)))
		copy(grown, m.data)
		m.data = grown
	}
	copy(m.data[chunk.Offset:], data)
	return nil
}

func TestSchedulerCancelWaitsForWorkers(t *testing.T) {
	s := New()
	_, _ = s.Enqueue(TaskSpec{TaskID: "cancel-wait", HostKey: "local", TotalBytes: 1})
	entered, release := make(chan struct{}), make(chan struct{})
	source := fakeSourceFunc(func(context.Context, TaskSpec, Chunk) ([]byte, error) {
		close(entered)
		<-release
		return []byte("x"), nil
	})
	_ = s.Start(context.Background(), "cancel-wait", source, &memSink{})
	<-entered
	_ = s.Cancel("cancel-wait")
	done := make(chan struct{})
	go func() { _ = s.Wait(context.Background(), "cancel-wait"); close(done) }()
	select {
	case <-done:
		t.Fatal("wait returned while worker still owns files")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker never joined")
	}
}

func TestSchedulerEmptyFileCompletes(t *testing.T) {
	s := New()
	if _, err := s.Enqueue(TaskSpec{TaskID: "empty", HostKey: "local", TotalBytes: 0}); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), "empty", &memSource{}, &memSink{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	p, _ := s.Progress("empty")
	if p.State != StateCompleted {
		t.Fatalf("empty: %+v", p)
	}
}

func TestSchedulerRejectsShortSource(t *testing.T) {
	s := New()
	_, _ = s.Enqueue(TaskSpec{TaskID: "short", HostKey: "local", TotalBytes: 5})
	if err := s.Start(context.Background(), "short", &memSource{data: []byte("x")}, &memSink{}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		p, _ := s.Progress("short")
		if p.State == StateFailed {
			return
		}
		if p.State == StateCompleted {
			t.Fatal("short read reported success")
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("transfer did not settle")
}

func TestSchedulerCompletesAndAggregates(t *testing.T) {
	payload := make([]byte, 10000)
	for i := range payload {
		payload[i] = byte(i % 251)
	}
	source := &memSource{data: payload}
	sink := &memSink{}
	scheduler := New(WithHostConcurrency(2))

	spec := TaskSpec{TaskID: "t1", HostKey: "host1:22", TotalBytes: int64(len(payload)), ChunkSize: 1000}
	if _, err := scheduler.Enqueue(spec); err != nil {
		t.Fatal(err)
	}
	if _, err := scheduler.Enqueue(spec); err == nil {
		t.Fatal("duplicate task must be rejected")
	}
	if err := scheduler.Start(context.Background(), "t1", source, sink); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		progress, err := scheduler.Progress("t1")
		if err != nil {
			t.Fatal(err)
		}
		if progress.State == StateCompleted {
			break
		}
		if progress.State == StateFailed {
			t.Fatalf("transfer failed: %s", progress.Err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	progress, err := scheduler.Progress("t1")
	if err != nil {
		t.Fatal(err)
	}
	if progress.State != StateCompleted || progress.DoneBytes != int64(len(payload)) || progress.ChunksDone != 10 {
		t.Fatalf("unexpected final progress: %+v", progress)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if string(sink.data) != string(payload) {
		t.Fatal("sink content mismatch")
	}
}

func TestSchedulerCancelFailClosed(t *testing.T) {
	slowSource := func(_ context.Context, _ TaskSpec, _ Chunk) ([]byte, error) {
		time.Sleep(100 * time.Millisecond)
		return []byte("x"), nil
	}
	source := fakeSourceFunc(slowSource)
	sink := &memSink{}
	scheduler := New(WithHostConcurrency(1))
	spec := TaskSpec{TaskID: "t2", HostKey: "host1:22", TotalBytes: 100, ChunkSize: 10}
	if _, err := scheduler.Enqueue(spec); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Start(context.Background(), "t2", source, sink); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := scheduler.Cancel("t2"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		progress, _ := scheduler.Progress("t2")
		if progress.State == StateCancelled || progress.State == StateFailed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("cancel did not settle the task")
}

func TestSchedulerPauseResume(t *testing.T) {
	source := &memSource{data: make([]byte, 5000)}
	sink := &memSink{}
	scheduler := New(WithHostConcurrency(1))
	spec := TaskSpec{TaskID: "t3", HostKey: "host1:22", TotalBytes: 5000, ChunkSize: 100}
	if _, err := scheduler.Enqueue(spec); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Start(context.Background(), "t3", source, sink); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Pause("t3"); err != nil {
		t.Fatal(err)
	}
	paused, _ := scheduler.Progress("t3")
	if paused.LifecycleEpoch != 1 {
		t.Fatalf("pause epoch = %d", paused.LifecycleEpoch)
	}
	if err := scheduler.Resume("t3"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		progress, _ := scheduler.Progress("t3")
		if progress.LifecycleEpoch != 2 {
			t.Fatalf("resume epoch = %d", progress.LifecycleEpoch)
		}
		if progress.State == StateCompleted {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("resumed transfer did not complete")
}

func TestSchedulerUnknownTaskAndStates(t *testing.T) {
	scheduler := New()
	if _, err := scheduler.Progress("nope"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("unknown task progress: %v", err)
	}
	if err := scheduler.Pause("nope"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("unknown task pause: %v", err)
	}
	source := &memSource{data: []byte("data")}
	sink := &memSink{}
	spec := TaskSpec{TaskID: "t4", HostKey: "h", TotalBytes: 4, ChunkSize: 4}
	if _, err := scheduler.Enqueue(spec); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Pause("t4"); !errors.Is(err, ErrTaskNotRunning) {
		t.Fatalf("pause before start: %v", err)
	}
	if err := scheduler.Start(context.Background(), "t4", source, sink); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Start(context.Background(), "t4", source, sink); err == nil {
		t.Fatal("double start must fail")
	}
}

type fakeSourceFunc func(ctx context.Context, spec TaskSpec, chunk Chunk) ([]byte, error)

func (f fakeSourceFunc) ReadChunk(ctx context.Context, spec TaskSpec, chunk Chunk) ([]byte, error) {
	return f(ctx, spec, chunk)
}
