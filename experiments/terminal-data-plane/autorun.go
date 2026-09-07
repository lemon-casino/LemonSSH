package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

const (
	autorunEnv                   = "NETCATTY_TERMINAL_DATA_PLANE_AUTORUN"
	autorunTimeoutSecondsEnv     = "NETCATTY_TERMINAL_DATA_PLANE_AUTORUN_TIMEOUT_SECONDS"
	defaultAutorunTimeout        = 90 * time.Second
	minAutorunTimeout            = 30 * time.Second
	maxAutorunTimeout            = 300 * time.Second
	defaultConditionTimeout      = 45 * time.Second
	defaultConditionPollInterval = 25 * time.Millisecond
	autorunQuitDelay             = 25 * time.Millisecond
	autorunStartupReadyGrace     = 500 * time.Millisecond
	autorunProcessStopGrace      = 5 * time.Second
	autorunQuitRetryDelay        = 100 * time.Millisecond
	autorunMaxQuitAttempts       = 3
	autorunQuitLoopWaitTimeout   = 2 * time.Second

	autorunExitSuccess    = 0
	autorunExitFailure    = 20
	autorunExitTimeout    = 21
	autorunExitIncomplete = 22
)

type AutorunConfig struct {
	Enabled                bool   `json:"enabled"`
	Workload               string `json:"workload"`
	SessionCount           int    `json:"sessionCount"`
	ChunkCount             int    `json:"chunkCount"`
	HardTimeoutMillis      int64  `json:"hardTimeoutMillis"`
	ConditionTimeoutMillis int64  `json:"conditionTimeoutMillis"`
	PollIntervalMillis     int64  `json:"pollIntervalMillis"`
}

type AutorunResult struct {
	FormatVersion                    int    `json:"formatVersion"`
	SessionID                        string `json:"sessionId"`
	Workload                         string `json:"workload"`
	SessionCount                     int    `json:"sessionCount"`
	ChunkCount                       int    `json:"chunkCount"`
	PayloadBytes                     uint64 `json:"payloadBytes"`
	CreditBytes                      uint64 `json:"creditBytes"`
	FrameCount                       uint64 `json:"frameCount"`
	SequenceIntegrity                bool   `json:"sequenceIntegrity"`
	ByteIntegrity                    bool   `json:"byteIntegrity"`
	CreditIntegrity                  bool   `json:"creditIntegrity"`
	DigestIntegrity                  bool   `json:"digestIntegrity"`
	UrgentRendererRTTMicros          uint64 `json:"urgentRendererRttMicros"`
	Generation                       uint32 `json:"generation"`
	RebindCount                      uint64 `json:"rebindCount"`
	FrontendQueueHighWaterBytes      uint64 `json:"frontendQueueHighWaterBytes"`
	BackendOutstandingHighWaterBytes uint64 `json:"backendOutstandingHighWaterBytes"`
	MemoryStart                      bool   `json:"memoryStart"`
	MemorySteady                     bool   `json:"memorySteady"`
	MemoryComplete                   bool   `json:"memoryComplete"`
	MemorySettled                    bool   `json:"memorySettled"`
	TotalDurationMillis              int64  `json:"totalDurationMillis"`
}

type AutorunFailure struct {
	FormatVersion int    `json:"formatVersion"`
	Step          string `json:"step"`
	Code          string `json:"code"`
}

type autorunSuccessMarker struct {
	FormatVersion                    int    `json:"formatVersion"`
	Platform                         string `json:"platform"`
	Arch                             string `json:"arch"`
	Workload                         string `json:"workload"`
	SessionCount                     int    `json:"sessionCount"`
	ChunkCount                       int    `json:"chunkCount"`
	PayloadBytes                     uint64 `json:"payloadBytes"`
	CreditBytes                      uint64 `json:"creditBytes"`
	FrameCount                       uint64 `json:"frameCount"`
	SequenceIntegrity                bool   `json:"sequenceIntegrity"`
	ByteIntegrity                    bool   `json:"byteIntegrity"`
	CreditIntegrity                  bool   `json:"creditIntegrity"`
	DigestIntegrity                  bool   `json:"digestIntegrity"`
	UrgentRendererRTTMicros          uint64 `json:"urgentRendererRttMicros"`
	Generation                       uint32 `json:"generation"`
	RebindCount                      uint64 `json:"rebindCount"`
	FrontendQueueHighWaterBytes      uint64 `json:"frontendQueueHighWaterBytes"`
	BackendOutstandingHighWaterBytes uint64 `json:"backendOutstandingHighWaterBytes"`
	MemoryStart                      bool   `json:"memoryStart"`
	MemorySteady                     bool   `json:"memorySteady"`
	MemoryComplete                   bool   `json:"memoryComplete"`
	MemorySettled                    bool   `json:"memorySettled"`
	TotalDurationMillis              int64  `json:"totalDurationMillis"`
}

type autorunFailureMarker struct {
	FormatVersion       int    `json:"formatVersion"`
	Platform            string `json:"platform"`
	Arch                string `json:"arch"`
	Step                string `json:"step"`
	Code                string `json:"code"`
	TotalDurationMillis int64  `json:"totalDurationMillis"`
}

type autorunOutcome uint8

const (
	autorunOutcomePending autorunOutcome = iota
	autorunOutcomeSuccess
	autorunOutcomeFailure
	autorunOutcomeTimeout
)

var errAutorunOutcomeDecided = errors.New("autorun outcome is already decided")
var errAutorunHardDeadline = errors.New("autorun result reached the backend hard deadline")

type autorunCoordinator struct {
	mu           sync.Mutex
	config       AutorunConfig
	output       io.Writer
	outcome      autorunOutcome
	startedAt    time.Time
	outcomeReady chan struct{}
	loopDone     chan struct{}
	quitLoopDone chan struct{}
	ready        chan struct{}
	stopped      chan struct{}
	startOnce    sync.Once
	readyOnce    sync.Once
	forceOnce    sync.Once
	quitDelay    time.Duration
	startupGrace time.Duration
	stopGrace    time.Duration
	retryDelay   time.Duration
	maxAttempts  int
	waitTimeout  time.Duration
	elapsed      func(time.Time) time.Duration
	forceExit    func(int)
	lifecycleMu  sync.Mutex
	appStopped   bool
	detachMu     sync.Mutex
	detachReady  []func()
}

func parseAutorunConfig(lookup func(string) (string, bool)) (AutorunConfig, error) {
	config := defaultAutorunConfig()
	gate, _ := lookup(autorunEnv)
	if gate != "1" {
		return config, nil
	}
	config.Enabled = true
	if raw, present := lookup(autorunTimeoutSecondsEnv); present {
		if !validStrictDecimal(raw) {
			return AutorunConfig{}, fmt.Errorf("%s must be an integer", autorunTimeoutSecondsEnv)
		}
		seconds, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return AutorunConfig{}, fmt.Errorf("%s must be an integer", autorunTimeoutSecondsEnv)
		}
		if seconds < uint64(minAutorunTimeout/time.Second) || seconds > uint64(maxAutorunTimeout/time.Second) {
			return AutorunConfig{}, fmt.Errorf("%s must be between %d and %d", autorunTimeoutSecondsEnv, int(minAutorunTimeout.Seconds()), int(maxAutorunTimeout.Seconds()))
		}
		timeout := time.Duration(seconds) * time.Second
		config.HardTimeoutMillis = timeout.Milliseconds()
		config.ConditionTimeoutMillis = min(defaultConditionTimeout, timeout/2).Milliseconds()
	}
	return config, nil
}

func validStrictDecimal(value string) bool {
	if value == "" || value[0] == '0' {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func defaultAutorunConfig() AutorunConfig {
	return AutorunConfig{
		Workload:               WorkloadSustained,
		SessionCount:           1,
		ChunkCount:             1600,
		HardTimeoutMillis:      defaultAutorunTimeout.Milliseconds(),
		ConditionTimeoutMillis: defaultConditionTimeout.Milliseconds(),
		PollIntervalMillis:     defaultConditionPollInterval.Milliseconds(),
	}
}

func newAutorunCoordinator(config AutorunConfig, output io.Writer) *autorunCoordinator {
	return &autorunCoordinator{
		config:       config,
		output:       output,
		startedAt:    time.Now(),
		outcomeReady: make(chan struct{}),
		loopDone:     make(chan struct{}),
		quitLoopDone: make(chan struct{}),
		ready:        make(chan struct{}),
		stopped:      make(chan struct{}),
		quitDelay:    autorunQuitDelay,
		startupGrace: autorunStartupReadyGrace,
		stopGrace:    autorunProcessStopGrace,
		retryDelay:   autorunQuitRetryDelay,
		maxAttempts:  autorunMaxQuitAttempts,
		waitTimeout:  autorunQuitLoopWaitTimeout,
		elapsed:      time.Since,
		forceExit:    os.Exit,
	}
}

func (a *autorunCoordinator) start(ctx context.Context, waitGroup *sync.WaitGroup, quit func()) {
	if !a.config.Enabled {
		return
	}
	a.startOnce.Do(func() {
		a.mu.Lock()
		a.startedAt = time.Now()
		a.mu.Unlock()
		waitGroup.Add(1)
		go func() {
			shouldQuit := a.runLifecycle(ctx)
			waitGroup.Done()
			if shouldQuit && quit != nil {
				a.coordinateProcessStop(ctx, quit)
			}
			close(a.quitLoopDone)
		}()
	})
}

func (a *autorunCoordinator) runLifecycle(ctx context.Context) bool {
	defer close(a.loopDone)
	timeout := time.NewTimer(time.Duration(a.config.HardTimeoutMillis) * time.Millisecond)
	defer timeout.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-a.outcomeReady:
	case <-timeout.C:
		_ = a.completeFailure("backend-timeout", "timeout", autorunOutcomeTimeout)
	}

	delay := time.NewTimer(a.quitDelay)
	defer delay.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-delay.C:
		return true
	}
}

func (a *autorunCoordinator) signalReady() {
	a.readyOnce.Do(func() { close(a.ready) })
}

func (a *autorunCoordinator) signalStopped() {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	if a.appStopped {
		return
	}
	a.appStopped = true
	close(a.stopped)
}

func (a *autorunCoordinator) addReadinessDetachers(detachers ...func()) {
	a.detachMu.Lock()
	defer a.detachMu.Unlock()
	for _, detach := range detachers {
		if detach != nil {
			a.detachReady = append(a.detachReady, detach)
		}
	}
}

func (a *autorunCoordinator) detachReadinessCallbacks() {
	a.detachMu.Lock()
	detachers := a.detachReady
	a.detachReady = nil
	a.detachMu.Unlock()
	for _, detach := range detachers {
		detach()
	}
}

func (a *autorunCoordinator) coordinateProcessStop(ctx context.Context, quit func()) {
	deadline := time.NewTimer(a.stopGrace)
	defer deadline.Stop()
	startup := time.NewTimer(min(a.startupGrace, a.stopGrace))
	defer startup.Stop()
	startupReady := startup.C
	ready := a.ready
	contextDone := ctx.Done()
	var attemptDone <-chan struct{}
	var retryTimer *time.Timer
	var retry <-chan time.Time
	attempts := 0
	shutdownObserved := false

	stopRetry := func() {
		if retryTimer != nil {
			if !retryTimer.Stop() {
				select {
				case <-retryTimer.C:
				default:
				}
			}
		}
		retry = nil
	}
	defer stopRetry()
	startAttempt := func() {
		if attemptDone != nil || attempts >= a.maxAttempts || shutdownObserved || a.applicationStopped() {
			return
		}
		select {
		case <-ctx.Done():
			shutdownObserved = true
			contextDone = nil
			return
		default:
		}
		attempts++
		done := make(chan struct{})
		attemptDone = done
		go func() {
			quit()
			close(done)
		}()
	}
	scheduleRetry := func() {
		if attempts >= a.maxAttempts || shutdownObserved {
			return
		}
		if retryTimer == nil {
			retryTimer = time.NewTimer(a.retryDelay)
		} else {
			retryTimer.Reset(a.retryDelay)
		}
		retry = retryTimer.C
	}

	for {
		if a.applicationStopped() {
			return
		}
		select {
		case <-a.stopped:
			return
		case <-deadline.C:
			a.forceExitIfNeeded()
			return
		case <-contextDone:
			shutdownObserved = true
			contextDone = nil
			stopRetry()
		case <-ready:
			ready = nil
			startupReady = nil
			stopRetry()
			startAttempt()
		case <-startupReady:
			startupReady = nil
			startAttempt()
		case <-attemptDone:
			attemptDone = nil
			if a.applicationStopped() {
				return
			}
			scheduleRetry()
		case <-retry:
			retry = nil
			startAttempt()
		}
	}
}

func (a *autorunCoordinator) applicationStopped() bool {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	return a.appStopped
}

func (a *autorunCoordinator) forceExitIfNeeded() {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	if a.appStopped {
		return
	}
	outcome := a.outcomeSnapshot()
	// Emergency boundary for this disposable autorun harness only. Full probe
	// shutdown may wait on this coordinator, so process teardown owns cleanup.
	a.forceOnce.Do(func() {
		a.forceExit(autorunExitStatus(true, outcome))
	})
}

func (a *autorunCoordinator) waitForQuitLoop() bool {
	if !a.config.Enabled {
		return true
	}
	timeout := time.NewTimer(a.waitTimeout)
	defer timeout.Stop()
	select {
	case <-a.quitLoopDone:
		return true
	case <-timeout.C:
		return false
	}
}

func (a *autorunCoordinator) completeSuccess(result AutorunResult) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.outcome != autorunOutcomePending {
		return errAutorunOutcomeDecided
	}
	elapsed := a.elapsedLocked()
	if elapsed >= time.Duration(a.config.HardTimeoutMillis)*time.Millisecond {
		marker := autorunFailureMarker{
			FormatVersion: 1, Platform: runtime.GOOS, Arch: runtime.GOARCH,
			Step: "backend-timeout", Code: "timeout", TotalDurationMillis: elapsedMillis(elapsed),
		}
		if err := a.writeOutcomeLocked(autorunOutcomeTimeout, "TERMINAL_DATA_PLANE_PROBE_FAILURE", marker); err != nil {
			return errors.Join(errAutorunHardDeadline, err)
		}
		return errAutorunHardDeadline
	}
	marker := autorunSuccessMarker{
		FormatVersion: result.FormatVersion, Platform: runtime.GOOS, Arch: runtime.GOARCH,
		Workload: result.Workload, SessionCount: result.SessionCount, ChunkCount: result.ChunkCount,
		PayloadBytes: result.PayloadBytes, CreditBytes: result.CreditBytes, FrameCount: result.FrameCount,
		SequenceIntegrity: result.SequenceIntegrity, ByteIntegrity: result.ByteIntegrity,
		CreditIntegrity: result.CreditIntegrity, DigestIntegrity: result.DigestIntegrity,
		UrgentRendererRTTMicros: result.UrgentRendererRTTMicros, Generation: result.Generation,
		RebindCount: result.RebindCount, FrontendQueueHighWaterBytes: result.FrontendQueueHighWaterBytes,
		BackendOutstandingHighWaterBytes: result.BackendOutstandingHighWaterBytes,
		MemoryStart:                      result.MemoryStart, MemorySteady: result.MemorySteady,
		MemoryComplete: result.MemoryComplete, MemorySettled: result.MemorySettled,
		TotalDurationMillis: elapsedMillis(elapsed),
	}
	return a.writeOutcomeLocked(autorunOutcomeSuccess, "TERMINAL_DATA_PLANE_PROBE_RESULT", marker)
}

func (a *autorunCoordinator) completeFailure(step, code string, outcome autorunOutcome) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.outcome != autorunOutcomePending {
		return errAutorunOutcomeDecided
	}
	elapsed := a.elapsedLocked()
	marker := autorunFailureMarker{
		FormatVersion: 1, Platform: runtime.GOOS, Arch: runtime.GOARCH,
		Step: step, Code: code, TotalDurationMillis: elapsedMillis(elapsed),
	}
	return a.writeOutcomeLocked(outcome, "TERMINAL_DATA_PLANE_PROBE_FAILURE", marker)
}

func (a *autorunCoordinator) writeOutcomeLocked(outcome autorunOutcome, prefix string, marker any) error {
	encoded, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	line := append([]byte(prefix+" "), encoded...)
	line = append(line, '\n')
	written, writeErr := a.output.Write(line)
	if writeErr == nil && written == len(line) {
		a.outcome = outcome
		close(a.outcomeReady)
		return nil
	}
	if written > 0 {
		a.outcome = autorunOutcomeFailure
		close(a.outcomeReady)
	}
	if writeErr != nil {
		return writeErr
	}
	return io.ErrShortWrite
}

func (a *autorunCoordinator) elapsedLocked() time.Duration {
	return max(0, a.elapsed(a.startedAt))
}

func elapsedMillis(elapsed time.Duration) int64 {
	return max(1, elapsed.Milliseconds())
}

func (a *autorunCoordinator) outcomeSnapshot() autorunOutcome {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.outcome
}

func autorunExitStatus(enabled bool, outcome autorunOutcome) int {
	if !enabled {
		return autorunExitSuccess
	}
	switch outcome {
	case autorunOutcomeSuccess:
		return autorunExitSuccess
	case autorunOutcomeTimeout:
		return autorunExitTimeout
	case autorunOutcomeFailure:
		return autorunExitFailure
	default:
		return autorunExitIncomplete
	}
}

func applicationExitStatus(autorunEnabled bool, outcome autorunOutcome, runFailed bool) int {
	if runFailed {
		if autorunEnabled {
			if outcome == autorunOutcomeTimeout {
				return autorunExitTimeout
			}
			return autorunExitFailure
		}
		return 1
	}
	return autorunExitStatus(autorunEnabled, outcome)
}

func recordAutorunRunFailure(autorun *autorunCoordinator, runErr error) {
	if runErr != nil && autorun.config.Enabled && autorun.outcomeSnapshot() == autorunOutcomePending {
		_ = autorun.completeFailure("application-run", "run-failed", autorunOutcomeFailure)
	}
}

func writeAutorunStartupFailure(output io.Writer, step, code string) {
	marker := autorunFailureMarker{
		FormatVersion: 1, Platform: runtime.GOOS, Arch: runtime.GOARCH,
		Step: step, Code: code, TotalDurationMillis: 0,
	}
	encoded, _ := json.Marshal(marker)
	_, _ = fmt.Fprintf(output, "TERMINAL_DATA_PLANE_PROBE_FAILURE %s\n", encoded)
}

func (p *ProbeService) GetAutorunConfig() AutorunConfig {
	return p.autorun.config
}

func (p *ProbeService) ReportAutorunSuccess(result AutorunResult) error {
	if !p.autorun.config.Enabled {
		return errors.New("autorun is disabled")
	}
	if err := p.validateAutorunResult(result); err != nil {
		return err
	}
	return p.autorun.completeSuccess(result)
}

func (p *ProbeService) ReportAutorunFailure(failure AutorunFailure) error {
	if !p.autorun.config.Enabled {
		return errors.New("autorun is disabled")
	}
	if failure.FormatVersion != 1 || !validAutorunLabel(failure.Step) || !validAutorunLabel(failure.Code) {
		return errors.New("invalid bounded autorun failure")
	}
	return p.autorun.completeFailure(failure.Step, failure.Code, autorunOutcomeFailure)
}

func (p *ProbeService) validateAutorunResult(result AutorunResult) error {
	config := p.autorun.config
	if result.FormatVersion != 1 || result.Workload != config.Workload || result.SessionCount != config.SessionCount ||
		result.ChunkCount != config.ChunkCount {
		return errors.New("autorun result identity is invalid")
	}
	if !result.SequenceIntegrity || !result.ByteIntegrity || !result.CreditIntegrity || !result.DigestIntegrity ||
		!result.MemoryStart || !result.MemorySteady || !result.MemoryComplete || !result.MemorySettled {
		return errors.New("autorun integrity or memory phase evidence is incomplete")
	}
	if result.UrgentRendererRTTMicros == 0 || result.UrgentRendererRTTMicros > 10_000_000 ||
		result.Generation != 2 || result.RebindCount != 1 || result.FrontendQueueHighWaterBytes == 0 ||
		result.FrontendQueueHighWaterBytes > receiveWindowBytes || result.BackendOutstandingHighWaterBytes == 0 ||
		result.BackendOutstandingHighWaterBytes > receiveWindowBytes {
		return errors.New("autorun stall, urgent, or rebind evidence is invalid")
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	session := p.sessions[result.SessionID]
	if session == nil || session.workloadID != config.Workload || !session.stopped || !session.complete {
		return errors.New("autorun session is missing or not finalized")
	}
	metrics := session.metrics
	if metrics.Error != "" || metrics.CloseStatus != "stopped" || metrics.Running || !metrics.Complete ||
		session.outstanding != 0 || metrics.PreStopOutstandingBytes != 0 {
		return errors.New("autorun session did not stop cleanly")
	}
	if metrics.Sequence != uint64(config.ChunkCount) || metrics.AppliedSequence != metrics.Sequence ||
		metrics.ExpectedFrames != metrics.Sequence || metrics.FramesSent != metrics.Sequence ||
		metrics.FramesAcked != metrics.Sequence || result.FrameCount != metrics.Sequence {
		return errors.New("autorun frame or sequence evidence does not match backend state")
	}
	if metrics.ExpectedPayloadSHA256 != canonicalPayloadSHA256 || metrics.PayloadBytes != metrics.ExpectedPayloadBytes ||
		metrics.CreditBytes != metrics.ExpectedCreditBytes || result.PayloadBytes != metrics.PayloadBytes ||
		result.CreditBytes != metrics.CreditBytes {
		return errors.New("autorun byte, credit, or digest expectation does not match backend state")
	}
	if metrics.Generation != result.Generation || metrics.RouteInterruptionCount != result.RebindCount ||
		metrics.DrainCount != 1 || metrics.PauseCount == 0 || metrics.ResumeCount == 0 || metrics.UrgentCount != 1 ||
		metrics.BackendMaxOutstanding != result.BackendOutstandingHighWaterBytes {
		return errors.New("autorun backend stall, urgent, or rebind evidence does not match")
	}
	return nil
}

func validAutorunLabel(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}
