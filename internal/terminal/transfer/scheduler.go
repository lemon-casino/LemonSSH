// Package transfer owns the Go transfer scheduler (P3-06): task identity,
// chunk checkpoints, pause/resume/cancel, per-host concurrency limits and
// aggregated progress. Transfers run against an abstract chunked source and
// sink so SFTP/local/relay transports share one scheduler.
package transfer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrTaskNotFound   = errors.New("transfer task not found")
	ErrTaskNotRunning = errors.New("transfer task is not running")
	ErrSinkShortWrite = errors.New("transfer sink short write")
)

// Chunk is one resumable unit of work.
type Chunk struct {
	Index  int64
	Offset int64
	Length int64
	Done   bool
}

// TaskSpec describes one transfer.
type TaskSpec struct {
	TaskID     string
	HostKey    string // per-host concurrency scope (endpoint identity)
	SourcePath string
	SinkPath   string
	TotalBytes int64
	ChunkSize  int64
}

// State is the task lifecycle state.
type State string

const (
	StatePending   State = "pending"
	StateRunning   State = "running"
	StatePaused    State = "paused"
	StateCompleted State = "completed"
	StateCancelled State = "cancelled"
	StateFailed    State = "failed"
)

// Progress is the aggregated progress snapshot. UI subscribes to snapshots at
// a bounded rate — never per chunk — so Wails events are not flooded.
type Progress struct {
	LifecycleEpoch uint64    `json:"lifecycleEpoch"`
	TaskID         string    `json:"taskId"`
	State          State     `json:"state"`
	TotalBytes     int64     `json:"totalBytes"`
	DoneBytes      int64     `json:"doneBytes"`
	ChunksDone     int       `json:"chunksDone"`
	ChunksTotal    int       `json:"chunksTotal"`
	StartedAt      time.Time `json:"startedAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Err            string    `json:"error,omitempty"`
}

// Source reads one chunk of the transfer.
type Source interface {
	ReadChunk(ctx context.Context, spec TaskSpec, chunk Chunk) ([]byte, error)
}

// Sink writes one chunk at a resumable offset.
type Sink interface {
	WriteChunk(ctx context.Context, spec TaskSpec, chunk Chunk, data []byte) error
}

type task struct {
	lifecycleEpoch uint64
	done           chan struct{}
	startedAt      time.Time
	spec           TaskSpec
	chunks         []Chunk
	state          State
	errText        string
	pauseGate      int32
	cancel         context.CancelFunc
}

// Scheduler owns all transfers with per-host concurrency limits.
type Scheduler struct {
	mu              sync.Mutex
	tasks           map[string]*task
	hostSlots       map[string]chan struct{}
	hostConcurrency int
	chunkTimeout    time.Duration
	nowFunc         func() time.Time
}

// Option configures the scheduler.
type Option func(*Scheduler)

// WithHostConcurrency bounds simultaneous chunk workers per host.
func WithHostConcurrency(n int) Option {
	return func(s *Scheduler) { s.hostConcurrency = n }
}

// New constructs a scheduler.
func New(options ...Option) *Scheduler {
	scheduler := &Scheduler{
		tasks:           make(map[string]*task),
		hostSlots:       make(map[string]chan struct{}),
		hostConcurrency: 3,
		chunkTimeout:    60 * time.Second,
		nowFunc:         time.Now,
	}
	for _, option := range options {
		option(scheduler)
	}
	return scheduler
}

func newChunks(total, chunkSize int64) []Chunk {
	if chunkSize <= 0 {
		chunkSize = 1024 * 512
	}
	count := (total + chunkSize - 1) / chunkSize
	chunks := make([]Chunk, 0, count)
	for index := int64(0); index < count; index++ {
		offset := index * chunkSize
		length := chunkSize
		if offset+length > total {
			length = total - offset
		}
		chunks = append(chunks, Chunk{Index: index, Offset: offset, Length: length})
	}
	return chunks
}

// Enqueue registers a transfer in the pending state.
func (s *Scheduler) Enqueue(spec TaskSpec) (Progress, error) {
	if spec.TaskID == "" || spec.HostKey == "" || spec.TotalBytes < 0 {
		return Progress{}, fmt.Errorf("invalid transfer spec")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tasks[spec.TaskID]; exists {
		return Progress{}, fmt.Errorf("duplicate transfer task %s", spec.TaskID)
	}
	created := &task{done: make(chan struct{}), startedAt: s.nowFunc(), spec: spec, chunks: newChunks(spec.TotalBytes, spec.ChunkSize), state: StatePending}
	s.tasks[spec.TaskID] = created
	return s.snapshotLocked(created), nil
}

// Start launches the transfer workers.
func (s *Scheduler) Start(ctx context.Context, taskID string, source Source, sink Sink) error {
	s.mu.Lock()
	created, ok := s.tasks[taskID]
	if !ok {
		s.mu.Unlock()
		return ErrTaskNotFound
	}
	if created.state != StatePending && created.state != StatePaused {
		s.mu.Unlock()
		return ErrTaskNotRunning
	}
	runCtx, cancel := context.WithCancel(ctx)
	created.cancel = cancel
	created.state = StateRunning
	slots := s.hostSlots[created.spec.HostKey]
	if slots == nil {
		slots = make(chan struct{}, s.hostConcurrency)
		s.hostSlots[created.spec.HostKey] = slots
	}
	s.mu.Unlock()

	go s.run(runCtx, created, slots, source, sink)
	return nil
}

func (s *Scheduler) run(ctx context.Context, created *task, slots chan struct{}, source Source, sink Sink) {
	var workerWG sync.WaitGroup
	defer close(created.done)
	defer workerWG.Wait()
	chunkIndex := 0
	for chunkIndex < len(created.chunks) {
		select {
		case <-ctx.Done():
			s.finish(created, StateFailed, ctx.Err())
			return
		default:
		}
		// Pause gate: spin-wait on the gate while paused, checking cancel.
		for atomic.LoadInt32(&created.pauseGate) == 1 {
			select {
			case <-ctx.Done():
				s.finish(created, StateFailed, ctx.Err())
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
		chunk := created.chunks[chunkIndex]
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		workerWG.Add(1)
		go func(chunk Chunk) {
			defer workerWG.Done()
			defer func() { <-slots }()
			chunkCtx, cancel := context.WithTimeout(ctx, s.chunkTimeout)
			defer cancel()
			data, err := source.ReadChunk(chunkCtx, created.spec, chunk)
			if err != nil {
				s.finish(created, StateFailed, err)
				return
			}
			if int64(len(data)) != chunk.Length {
				s.finish(created, StateFailed, fmt.Errorf("source short read: got %d want %d", len(data), chunk.Length))
				return
			}
			if err := sink.WriteChunk(chunkCtx, created.spec, chunk, data); err != nil {
				s.finish(created, StateFailed, err)
				return
			}
			s.mu.Lock()
			created.chunks[chunk.Index].Done = true
			s.mu.Unlock()
		}(chunk)
		chunkIndex++
	}
	workerWG.Wait()
	s.finish(created, StateCompleted, nil)
}

func (s *Scheduler) finish(created *task, state State, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if created.state == StateCompleted || created.state == StateCancelled || created.state == StateFailed {
		return
	}
	// A user cancel wins over worker failures; otherwise record the failure.
	if !(state == StateFailed && created.state == StateCancelled) {
		created.state = state
	}
	created.errText = ""
	if err != nil && created.state == StateFailed {
		created.errText = err.Error()
	}
}

func (s *Scheduler) Wait(ctx context.Context, taskID string) error {
	s.mu.Lock()
	created, ok := s.tasks[taskID]
	s.mu.Unlock()
	if !ok {
		return ErrTaskNotFound
	}
	select {
	case <-created.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Progress reports the current snapshot.
func (s *Scheduler) Progress(taskID string) (Progress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	created, ok := s.tasks[taskID]
	if !ok {
		return Progress{}, ErrTaskNotFound
	}
	return s.snapshotLocked(created), nil
}

func (s *Scheduler) snapshotLocked(created *task) Progress {
	progress := Progress{
		LifecycleEpoch: created.lifecycleEpoch,
		TaskID:         created.spec.TaskID,
		State:          created.state,
		TotalBytes:     created.spec.TotalBytes,
		ChunksTotal:    len(created.chunks),
		Err:            created.errText,
		StartedAt:      created.startedAt,
		UpdatedAt:      s.nowFunc(),
	}
	for _, chunk := range created.chunks {
		if chunk.Done {
			progress.DoneBytes += chunk.Length
			progress.ChunksDone++
		}
	}
	return progress
}

// Pause suspends a running transfer.
func (s *Scheduler) Pause(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	created, ok := s.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}
	if created.state != StateRunning {
		return ErrTaskNotRunning
	}
	created.lifecycleEpoch++
	created.state = StatePaused
	atomic.StoreInt32(&created.pauseGate, 1)
	return nil
}

// Resume continues a paused transfer.
func (s *Scheduler) Resume(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	created, ok := s.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}
	if created.state != StatePaused {
		return ErrTaskNotRunning
	}
	created.lifecycleEpoch++
	atomic.StoreInt32(&created.pauseGate, 0)
	created.state = StateRunning
	return nil
}

// Cancel terminates a transfer.
func (s *Scheduler) Cancel(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	created, ok := s.tasks[taskID]
	if !ok {
		return ErrTaskNotFound
	}
	created.state = StateCancelled
	if created.cancel != nil {
		created.cancel()
	}
	return nil
}
