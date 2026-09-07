package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type failOnceWriter struct {
	failed bool
	bytes.Buffer
}

func (w *failOnceWriter) Write(data []byte) (int, error) {
	if !w.failed {
		w.failed = true
		return 0, errors.New("injected marker write failure")
	}
	return w.Buffer.Write(data)
}

type shortOnceWriter struct {
	short bool
	bytes.Buffer
}

type alwaysZeroErrorWriter struct {
	calls atomic.Int32
}

func (w *alwaysZeroErrorWriter) Write([]byte) (int, error) {
	w.calls.Add(1)
	return 0, errors.New("permanent marker write failure")
}

func (w *shortOnceWriter) Write(data []byte) (int, error) {
	if !w.short {
		w.short = true
		written := len(data) - 1
		_, _ = w.Buffer.Write(data[:written])
		return written, nil
	}
	return w.Buffer.Write(data)
}

func autorunTestConfig() AutorunConfig {
	return AutorunConfig{
		Enabled: true, Workload: WorkloadSustained, SessionCount: 1, ChunkCount: 1600,
		HardTimeoutMillis:      defaultAutorunTimeout.Milliseconds(),
		ConditionTimeoutMillis: defaultConditionTimeout.Milliseconds(),
		PollIntervalMillis:     defaultConditionPollInterval.Milliseconds(),
	}
}

func lookupFrom(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, exists := values[key]
		return value, exists
	}
}

func TestAutorunEnvIsDisabledByDefaultAndRequiresExactGate(t *testing.T) {
	for _, values := range []map[string]string{
		{},
		{autorunEnv: ""},
		{autorunEnv: "true"},
		{autorunEnv: "01"},
	} {
		config, err := parseAutorunConfig(lookupFrom(values))
		if err != nil {
			t.Fatal(err)
		}
		if config.Enabled {
			t.Fatalf("non-exact autorun gate enabled config: %#v", values)
		}
		if config.Workload != WorkloadSustained || config.SessionCount != 1 || config.ChunkCount != 1600 ||
			config.HardTimeoutMillis != defaultAutorunTimeout.Milliseconds() {
			t.Fatalf("disabled defaults changed: %#v", config)
		}
	}

	config, err := parseAutorunConfig(lookupFrom(map[string]string{autorunEnv: "1"}))
	if err != nil || !config.Enabled {
		t.Fatalf("exact autorun gate was not enabled: %#v, %v", config, err)
	}
}

func TestAutorunEnvRejectsInvalidTimeoutBounds(t *testing.T) {
	for _, value := range []string{
		"", "not-a-number", "29", "301", "30.5", "+30", "-30", " 30", "30 ", "030",
		"36028797018963998", "18446744073709551616",
	} {
		_, err := parseAutorunConfig(lookupFrom(map[string]string{
			autorunEnv: "1", autorunTimeoutSecondsEnv: value,
		}))
		if err == nil {
			t.Fatalf("invalid timeout %q was accepted", value)
		}
	}
	for _, boundary := range []struct {
		value  string
		millis int64
	}{{value: "30", millis: 30_000}, {value: "300", millis: 300_000}} {
		config, err := parseAutorunConfig(lookupFrom(map[string]string{
			autorunEnv: "1", autorunTimeoutSecondsEnv: boundary.value,
		}))
		if err != nil || config.HardTimeoutMillis != boundary.millis || config.ConditionTimeoutMillis <= 0 ||
			config.ConditionTimeoutMillis > config.HardTimeoutMillis {
			t.Fatalf("bounded timeout %q was not parsed: %#v, %v", boundary.value, config, err)
		}
	}
}

func prepareSuccessfulAutorun(t *testing.T) (*ProbeService, AutorunResult, *bytes.Buffer) {
	t.Helper()
	config := ProbeConfig{AllowedOrigins: []string{testOrigin}, Autorun: autorunTestConfig()}
	service, err := NewProbeService(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := service.shutdown(); err != nil {
			t.Errorf("shutdown service: %v", err)
		}
	})
	output := &bytes.Buffer{}
	service.autorun.output = output
	info, err := service.CreateSession(WorkloadSustained, WorkloadOptions{ChunkCount: 1600})
	if err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	session := service.sessions[info.SessionID]
	expectedPayload := session.metrics.ExpectedPayloadBytes
	expectedCredit := session.metrics.ExpectedCreditBytes
	session.generation = 2
	session.sequence = 1600
	session.applied = 1600
	session.complete = true
	session.stopped = true
	session.outstanding = 0
	session.metrics = SessionMetrics{
		SessionID: info.SessionID, Workload: WorkloadSustained, Generation: 2,
		Sequence: 1600, AppliedSequence: 1600, PayloadBytes: expectedPayload, CreditBytes: expectedCredit,
		FramesSent: 1600, FramesAcked: 1600, BackendMaxOutstanding: receiveWindowBytes,
		PauseCount: 1, ResumeCount: 1, UrgentCount: 1, DrainCount: 1, RouteInterruptionCount: 1,
		ExpectedPayloadBytes: expectedPayload, ExpectedCreditBytes: expectedCredit, ExpectedFrames: 1600,
		ExpectedPayloadSHA256: canonicalPayloadSHA256, Complete: true, CloseStatus: "stopped",
	}
	service.mu.Unlock()
	result := AutorunResult{
		FormatVersion: 1, SessionID: info.SessionID, Workload: WorkloadSustained,
		SessionCount: 1, ChunkCount: 1600, PayloadBytes: expectedPayload, CreditBytes: expectedCredit,
		FrameCount: 1600, SequenceIntegrity: true, ByteIntegrity: true, CreditIntegrity: true, DigestIntegrity: true,
		UrgentRendererRTTMicros: 100, Generation: 2, RebindCount: 1,
		FrontendQueueHighWaterBytes: 128 * 1024, BackendOutstandingHighWaterBytes: receiveWindowBytes,
		MemoryStart: true, MemorySteady: true, MemoryComplete: true, MemorySettled: true,
		TotalDurationMillis: 1_000,
	}
	return service, result, output
}

func TestAutorunSuccessValidatesRetainedBackendEvidence(t *testing.T) {
	service, result, output := prepareSuccessfulAutorun(t)
	if err := service.ReportAutorunSuccess(result); err != nil {
		t.Fatal(err)
	}
	line := output.String()
	if strings.Count(line, "\n") != 1 || !strings.HasPrefix(line, "TERMINAL_DATA_PLANE_PROBE_RESULT {") ||
		strings.Contains(line, result.SessionID) || !strings.Contains(line, `"digestIntegrity":true`) {
		t.Fatalf("unexpected compact success marker: %q", line)
	}
	if status := autorunExitStatus(true, service.autorun.outcomeSnapshot()); status != autorunExitSuccess {
		t.Fatalf("success exit status = %d", status)
	}
}

func TestAutorunSuccessUsesBackendElapsedAndRejectsLateBackendResult(t *testing.T) {
	t.Run("renderer duration is untrusted", func(t *testing.T) {
		service, result, output := prepareSuccessfulAutorun(t)
		service.autorun.elapsed = func(time.Time) time.Duration { return 1234 * time.Millisecond }
		result.TotalDurationMillis = 9_223_372_036_854_775_807
		if err := service.ReportAutorunSuccess(result); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), `"totalDurationMillis":1234`) ||
			strings.Contains(output.String(), `9223372036854775807`) {
			t.Fatalf("marker did not use backend elapsed time: %q", output.String())
		}
	})

	t.Run("exact backend deadline wins", func(t *testing.T) {
		service, result, output := prepareSuccessfulAutorun(t)
		service.autorun.elapsed = func(time.Time) time.Duration { return defaultAutorunTimeout }
		result.TotalDurationMillis = 1
		if err := service.ReportAutorunSuccess(result); !errors.Is(err, errAutorunHardDeadline) {
			t.Fatalf("result at backend deadline error = %v", err)
		}
		if service.autorun.outcomeSnapshot() != autorunOutcomeTimeout || strings.Count(output.String(), "\n") != 1 ||
			!strings.HasPrefix(output.String(), "TERMINAL_DATA_PLANE_PROBE_FAILURE ") {
			t.Fatalf("deadline did not atomically commit timeout: %q", output.String())
		}
	})

	t.Run("one nanosecond before deadline succeeds", func(t *testing.T) {
		service, result, output := prepareSuccessfulAutorun(t)
		service.autorun.elapsed = func(time.Time) time.Duration { return defaultAutorunTimeout - time.Nanosecond }
		if err := service.ReportAutorunSuccess(result); err != nil {
			t.Fatal(err)
		}
		if service.autorun.outcomeSnapshot() != autorunOutcomeSuccess ||
			!strings.HasPrefix(output.String(), "TERMINAL_DATA_PLANE_PROBE_RESULT ") {
			t.Fatalf("pre-deadline result did not succeed: %q", output.String())
		}
	})
}

func TestAutorunSuccessRejectsFrontendOrBackendMismatch(t *testing.T) {
	service, result, output := prepareSuccessfulAutorun(t)
	result.DigestIntegrity = false
	if err := service.ReportAutorunSuccess(result); err == nil {
		t.Fatal("false digest integrity was accepted")
	}
	if output.Len() != 0 || service.autorun.outcomeSnapshot() != autorunOutcomePending {
		t.Fatal("rejected result produced a terminal marker")
	}
	result.DigestIntegrity = true
	result.PayloadBytes++
	if err := service.ReportAutorunSuccess(result); err == nil {
		t.Fatal("backend payload mismatch was accepted")
	}
}

func TestAutorunExactlyOneOutcomeWins(t *testing.T) {
	output := &bytes.Buffer{}
	coordinator := newAutorunCoordinator(autorunTestConfig(), output)
	result := AutorunResult{FormatVersion: 1, Workload: WorkloadSustained}
	var waitGroup sync.WaitGroup
	for index := 0; index < 32; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			if index%2 == 0 {
				_ = coordinator.completeSuccess(result)
			} else {
				_ = coordinator.completeFailure("frontend", "failed", autorunOutcomeFailure)
			}
		}(index)
	}
	waitGroup.Wait()
	if strings.Count(output.String(), "\n") != 1 || coordinator.outcomeSnapshot() == autorunOutcomePending {
		t.Fatalf("race produced multiple or no outcomes: %q", output.String())
	}
}

func TestAutorunDeadlineAndFailureRaceYieldsOneOutcome(t *testing.T) {
	output := &bytes.Buffer{}
	coordinator := newAutorunCoordinator(autorunTestConfig(), output)
	coordinator.elapsed = func(time.Time) time.Duration { return defaultAutorunTimeout }
	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		<-start
		_ = coordinator.completeSuccess(AutorunResult{FormatVersion: 1, Workload: WorkloadSustained})
	}()
	go func() {
		defer waitGroup.Done()
		<-start
		_ = coordinator.completeFailure("frontend", "failed", autorunOutcomeFailure)
	}()
	close(start)
	waitGroup.Wait()
	if coordinator.outcomeSnapshot() == autorunOutcomePending || coordinator.outcomeSnapshot() == autorunOutcomeSuccess ||
		strings.Count(output.String(), "\n") != 1 {
		t.Fatalf("deadline/failure race outcome=%d marker=%q", coordinator.outcomeSnapshot(), output.String())
	}
}

func TestAutorunMarkerWriteMustCompleteBeforeOutcomeCommit(t *testing.T) {
	t.Run("zero-byte failure permits later failure marker", func(t *testing.T) {
		writer := &failOnceWriter{}
		coordinator := newAutorunCoordinator(autorunTestConfig(), writer)
		if err := coordinator.completeSuccess(AutorunResult{FormatVersion: 1, Workload: WorkloadSustained}); err == nil {
			t.Fatal("injected marker write failure was hidden")
		}
		if coordinator.outcomeSnapshot() != autorunOutcomePending || writer.Len() != 0 {
			t.Fatal("zero-byte marker failure committed outcome or bytes")
		}
		select {
		case <-coordinator.outcomeReady:
			t.Fatal("zero-byte marker failure closed outcomeReady")
		default:
		}
		if err := coordinator.completeFailure("writer", "failed", autorunOutcomeFailure); err != nil {
			t.Fatal(err)
		}
		if strings.Count(writer.String(), "\n") != 1 || strings.Count(writer.String(), "TERMINAL_DATA_PLANE_PROBE_FAILURE ") != 1 {
			t.Fatalf("retry did not produce one failure marker: %q", writer.String())
		}
	})

	t.Run("nonzero partial write is terminal", func(t *testing.T) {
		writer := &shortOnceWriter{}
		coordinator := newAutorunCoordinator(autorunTestConfig(), writer)
		err := coordinator.completeSuccess(AutorunResult{FormatVersion: 1, Workload: WorkloadSustained})
		if !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("short marker write error = %v", err)
		}
		captured := writer.String()
		if coordinator.outcomeSnapshot() != autorunOutcomeFailure || writer.Len() == 0 ||
			!strings.HasPrefix(captured, "TERMINAL_DATA_PLANE_PROBE_RESULT ") || strings.Count(captured, "\n") != 0 ||
			applicationExitStatus(true, coordinator.outcomeSnapshot(), false) == 0 {
			t.Fatalf("partial marker policy mismatch: outcome=%d bytes=%q", coordinator.outcomeSnapshot(), captured)
		}
		select {
		case <-coordinator.outcomeReady:
		default:
			t.Fatal("partial marker did not unblock outcomeReady")
		}
		if err := coordinator.completeFailure("writer", "failed", autorunOutcomeFailure); !errors.Is(err, errAutorunOutcomeDecided) {
			t.Fatalf("partial marker allowed second outcome: %v", err)
		}
		if writer.String() != captured {
			t.Fatalf("second marker bytes appended after partial write: before=%q after=%q", captured, writer.String())
		}
	})
}

func TestAutorunTimeoutAndCancellationAreLifecycleOwned(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		config := autorunTestConfig()
		config.HardTimeoutMillis = 10
		output := &bytes.Buffer{}
		coordinator := newAutorunCoordinator(config, output)
		coordinator.quitDelay = 0
		coordinator.startupGrace = time.Second
		coordinator.stopGrace = 100 * time.Millisecond
		var forceCount atomic.Int32
		coordinator.forceExit = func(int) { forceCount.Add(1) }
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var waitGroup sync.WaitGroup
		quit := make(chan struct{}, 1)
		var quitCount atomic.Int32
		coordinator.start(ctx, &waitGroup, func() {
			quitCount.Add(1)
			quit <- struct{}{}
			coordinator.signalStopped()
		})
		select {
		case <-coordinator.outcomeReady:
		case <-time.After(time.Second):
			t.Fatal("timeout outcome was not recorded")
		}
		select {
		case <-quit:
			t.Fatal("quit ran before Wails readiness")
		case <-time.After(20 * time.Millisecond):
		}
		coordinator.signalReady()
		select {
		case <-quit:
		case <-time.After(time.Second):
			t.Fatal("timeout did not schedule quit")
		}
		waitGroup.Wait()
		select {
		case <-coordinator.quitLoopDone:
		case <-time.After(time.Second):
			t.Fatal("readiness quit loop did not finish")
		}
		select {
		case <-coordinator.loopDone:
		default:
			t.Fatal("timeout lifecycle was not finished before quit")
		}
		if quitCount.Load() != 1 || forceCount.Load() != 0 || coordinator.outcomeSnapshot() != autorunOutcomeTimeout ||
			autorunExitStatus(true, coordinator.outcomeSnapshot()) != autorunExitTimeout ||
			!strings.HasPrefix(output.String(), "TERMINAL_DATA_PLANE_PROBE_FAILURE ") {
			t.Fatalf("timeout outcome mismatch: %q", output.String())
		}
	})

	t.Run("never ready no-op quit forces timeout exit", func(t *testing.T) {
		config := autorunTestConfig()
		config.HardTimeoutMillis = 5
		coordinator := newAutorunCoordinator(config, &bytes.Buffer{})
		coordinator.quitDelay = 0
		coordinator.startupGrace = 5 * time.Millisecond
		coordinator.stopGrace = 30 * time.Millisecond
		coordinator.retryDelay = 2 * time.Millisecond
		coordinator.maxAttempts = 2
		var forceCount atomic.Int32
		var forceCode atomic.Int32
		coordinator.forceExit = func(code int) {
			forceCode.Store(int32(code))
			forceCount.Add(1)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var waitGroup sync.WaitGroup
		var quitCount atomic.Int32
		coordinator.start(ctx, &waitGroup, func() { quitCount.Add(1) })
		select {
		case <-coordinator.quitLoopDone:
		case <-time.After(time.Second):
			t.Fatal("never-ready watchdog did not terminate")
		}
		waitGroup.Wait()
		if quitCount.Load() != 2 || forceCount.Load() != 1 || forceCode.Load() != autorunExitTimeout ||
			coordinator.outcomeSnapshot() != autorunOutcomeTimeout {
			t.Fatalf("never-ready quits=%d forces=%d code=%d outcome=%d", quitCount.Load(), forceCount.Load(), forceCode.Load(), coordinator.outcomeSnapshot())
		}
	})

	t.Run("stopped wins ready race", func(t *testing.T) {
		config := autorunTestConfig()
		config.HardTimeoutMillis = 5
		coordinator := newAutorunCoordinator(config, &bytes.Buffer{})
		coordinator.quitDelay = 0
		coordinator.startupGrace = time.Second
		coordinator.stopGrace = 50 * time.Millisecond
		var forceCount atomic.Int32
		coordinator.forceExit = func(int) { forceCount.Add(1) }
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var waitGroup sync.WaitGroup
		var quitCount atomic.Int32
		coordinator.start(ctx, &waitGroup, func() { quitCount.Add(1) })
		select {
		case <-coordinator.outcomeReady:
		case <-time.After(time.Second):
			t.Fatal("timeout outcome was not recorded")
		}
		startSignals := make(chan struct{})
		var signalGroup sync.WaitGroup
		signalGroup.Add(2)
		go func() {
			defer signalGroup.Done()
			<-startSignals
			coordinator.signalStopped()
		}()
		go func() {
			defer signalGroup.Done()
			<-startSignals
			coordinator.signalReady()
		}()
		close(startSignals)
		signalGroup.Wait()
		select {
		case <-coordinator.quitLoopDone:
		case <-time.After(time.Second):
			t.Fatal("stopped/ready arbitration did not finish")
		}
		waitGroup.Wait()
		if quitCount.Load() > 1 || forceCount.Load() != 0 {
			t.Fatalf("stopped/ready race quits=%d forces=%d", quitCount.Load(), forceCount.Load())
		}
	})

	t.Run("blocking quit is bounded", func(t *testing.T) {
		config := autorunTestConfig()
		config.HardTimeoutMillis = 5
		coordinator := newAutorunCoordinator(config, &bytes.Buffer{})
		coordinator.quitDelay = 0
		coordinator.startupGrace = time.Second
		coordinator.stopGrace = 25 * time.Millisecond
		coordinator.maxAttempts = 3
		var forceCount atomic.Int32
		var forceCode atomic.Int32
		coordinator.forceExit = func(code int) {
			forceCode.Store(int32(code))
			forceCount.Add(1)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var waitGroup sync.WaitGroup
		var quitCount atomic.Int32
		releaseQuit := make(chan struct{})
		coordinator.start(ctx, &waitGroup, func() {
			quitCount.Add(1)
			<-releaseQuit
		})
		select {
		case <-coordinator.outcomeReady:
		case <-time.After(time.Second):
			t.Fatal("timeout outcome was not recorded")
		}
		coordinator.signalReady()
		select {
		case <-coordinator.quitLoopDone:
		case <-time.After(time.Second):
			t.Fatal("blocking quit held coordinator open")
		}
		waitGroup.Wait()
		if quitCount.Load() != 1 || forceCount.Load() != 1 || forceCode.Load() != autorunExitTimeout {
			t.Fatalf("blocking quit calls=%d forces=%d code=%d", quitCount.Load(), forceCount.Load(), forceCode.Load())
		}
		close(releaseQuit)
	})

	t.Run("timeout failure race force exits exactly once", func(t *testing.T) {
		config := autorunTestConfig()
		config.HardTimeoutMillis = 100
		output := &bytes.Buffer{}
		coordinator := newAutorunCoordinator(config, output)
		coordinator.quitDelay = 0
		coordinator.startupGrace = time.Millisecond
		coordinator.stopGrace = 20 * time.Millisecond
		coordinator.maxAttempts = 1
		var forceCount atomic.Int32
		var forceCode atomic.Int32
		coordinator.forceExit = func(code int) {
			forceCode.Store(int32(code))
			forceCount.Add(1)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var waitGroup sync.WaitGroup
		coordinator.start(ctx, &waitGroup, func() {})
		start := make(chan struct{})
		var outcomeGroup sync.WaitGroup
		outcomeGroup.Add(2)
		go func() {
			defer outcomeGroup.Done()
			<-start
			_ = coordinator.completeFailure("backend-timeout", "timeout", autorunOutcomeTimeout)
		}()
		go func() {
			defer outcomeGroup.Done()
			<-start
			_ = coordinator.completeFailure("frontend", "failed", autorunOutcomeFailure)
		}()
		close(start)
		outcomeGroup.Wait()
		select {
		case <-coordinator.quitLoopDone:
		case <-time.After(time.Second):
			t.Fatal("race force-exit coordinator did not finish")
		}
		waitGroup.Wait()
		outcome := coordinator.outcomeSnapshot()
		if forceCount.Load() != 1 || int(forceCode.Load()) != autorunExitStatus(true, outcome) ||
			strings.Count(output.String(), "\n") != 1 {
			t.Fatalf("race outcome=%d forces=%d code=%d marker=%q", outcome, forceCount.Load(), forceCode.Load(), output.String())
		}
	})

	t.Run("permanent zero-byte marker failure force exits incomplete", func(t *testing.T) {
		config := autorunTestConfig()
		config.HardTimeoutMillis = 5
		writer := &alwaysZeroErrorWriter{}
		coordinator := newAutorunCoordinator(config, writer)
		coordinator.quitDelay = 0
		coordinator.startupGrace = 2 * time.Millisecond
		coordinator.stopGrace = 20 * time.Millisecond
		coordinator.maxAttempts = 1
		var quitCount atomic.Int32
		var forceCount atomic.Int32
		var forceCode atomic.Int32
		coordinator.forceExit = func(code int) {
			forceCode.Store(int32(code))
			forceCount.Add(1)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var waitGroup sync.WaitGroup
		coordinator.start(ctx, &waitGroup, func() { quitCount.Add(1) })
		select {
		case <-coordinator.quitLoopDone:
		case <-time.After(time.Second):
			t.Fatal("permanent marker failure coordinator did not finish")
		}
		waitGroup.Wait()
		select {
		case <-coordinator.loopDone:
		default:
			t.Fatal("permanent marker failure lifecycle did not finish")
		}
		select {
		case <-coordinator.outcomeReady:
			t.Fatal("zero-byte marker failure fabricated a terminal marker outcome")
		default:
		}
		if coordinator.outcomeSnapshot() != autorunOutcomePending || writer.calls.Load() != 1 ||
			quitCount.Load() != 1 || forceCount.Load() != 1 || forceCode.Load() != autorunExitIncomplete {
			t.Fatalf("pending outcome=%d writes=%d quits=%d forces=%d code=%d", coordinator.outcomeSnapshot(), writer.calls.Load(), quitCount.Load(), forceCount.Load(), forceCode.Load())
		}
	})

	t.Run("main quit-loop wait is bounded", func(t *testing.T) {
		coordinator := newAutorunCoordinator(autorunTestConfig(), &bytes.Buffer{})
		coordinator.waitTimeout = 5 * time.Millisecond
		if coordinator.waitForQuitLoop() {
			t.Fatal("unstarted quit loop reported completion")
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		config := autorunTestConfig()
		output := &bytes.Buffer{}
		coordinator := newAutorunCoordinator(config, output)
		ctx, cancel := context.WithCancel(context.Background())
		var waitGroup sync.WaitGroup
		quitCalled := false
		coordinator.start(ctx, &waitGroup, func() { quitCalled = true })
		cancel()
		waitGroup.Wait()
		select {
		case <-coordinator.quitLoopDone:
		default:
			t.Fatal("cancelled quit loop did not terminate")
		}
		select {
		case <-coordinator.loopDone:
		default:
			t.Fatal("cancelled lifecycle did not terminate")
		}
		if quitCalled || output.Len() != 0 || coordinator.outcomeSnapshot() != autorunOutcomePending {
			t.Fatal("cancellation fabricated an autorun outcome")
		}
	})
}

func TestAutorunExitStatusDecision(t *testing.T) {
	tests := []struct {
		enabled bool
		outcome autorunOutcome
		want    int
	}{
		{enabled: false, outcome: autorunOutcomePending, want: autorunExitSuccess},
		{enabled: true, outcome: autorunOutcomeSuccess, want: autorunExitSuccess},
		{enabled: true, outcome: autorunOutcomeFailure, want: autorunExitFailure},
		{enabled: true, outcome: autorunOutcomeTimeout, want: autorunExitTimeout},
		{enabled: true, outcome: autorunOutcomePending, want: autorunExitIncomplete},
	}
	for _, test := range tests {
		if got := autorunExitStatus(test.enabled, test.outcome); got != test.want {
			t.Fatalf("exit status (%t, %d) = %d, want %d", test.enabled, test.outcome, got, test.want)
		}
	}
	applicationTests := []struct {
		enabled   bool
		outcome   autorunOutcome
		runFailed bool
		want      int
	}{
		{enabled: false, outcome: autorunOutcomePending, runFailed: true, want: 1},
		{enabled: true, outcome: autorunOutcomePending, runFailed: true, want: autorunExitFailure},
		{enabled: true, outcome: autorunOutcomeSuccess, runFailed: true, want: autorunExitFailure},
		{enabled: true, outcome: autorunOutcomeTimeout, runFailed: true, want: autorunExitTimeout},
		{enabled: true, outcome: autorunOutcomeSuccess, want: autorunExitSuccess},
	}
	for _, test := range applicationTests {
		if got := applicationExitStatus(test.enabled, test.outcome, test.runFailed); got != test.want {
			t.Fatalf("application exit status (%t, %d, %t) = %d, want %d", test.enabled, test.outcome, test.runFailed, got, test.want)
		}
	}

	output := &bytes.Buffer{}
	coordinator := newAutorunCoordinator(autorunTestConfig(), output)
	recordAutorunRunFailure(coordinator, errors.New("run failed"))
	recordAutorunRunFailure(coordinator, errors.New("run failed again"))
	if coordinator.outcomeSnapshot() != autorunOutcomeFailure || strings.Count(output.String(), "\n") != 1 ||
		!strings.Contains(output.String(), `"step":"application-run"`) {
		t.Fatalf("run failure did not produce one bounded outcome: %q", output.String())
	}
}
