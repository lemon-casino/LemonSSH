package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/binaricat/lemonssh/internal/platform/filesystem"
)

// fakeUploader records sync-back uploads instead of dialing SFTP.
type fakeUploader struct {
	mu       sync.Mutex
	uploads  []uploadCall
	failNext int
}

type uploadCall struct {
	sessionID  string
	localPath  string
	remotePath string
	encoding   string
	bytes      int64
}

func (f *fakeUploader) Upload(sessionID, localPath, remotePath, encoding string) (int64, error) {
	f.mu.Lock()
	if f.failNext > 0 {
		f.failNext--
		f.mu.Unlock()
		return 0, fmt.Errorf("simulated upload failure")
	}
	info, _ := os.Stat(localPath)
	var size int64
	if info != nil {
		size = info.Size()
	}
	f.uploads = append(f.uploads, uploadCall{sessionID, localPath, remotePath, encoding, size})
	f.mu.Unlock()
	return size, nil
}

func (f *fakeUploader) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.uploads)
}

func (f *fakeUploader) last() uploadCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.uploads[len(f.uploads)-1]
}

// failNextUpload makes the next Upload return an error (error event path).
func (f *fakeUploader) failNextUpload() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext++
}

type eventRecorder struct {
	mu    sync.Mutex
	names []string
}

func (r *eventRecorder) emit(name string, _ any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = append(r.names, name)
}

func (r *eventRecorder) count(name string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := 0
	for _, n := range r.names {
		if n == name {
			total++
		}
	}
	return total
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func newWatchTestService(t *testing.T) (*FileWatchService, *fakeUploader, *eventRecorder, string) {
	t.Helper()
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatalf("temp service: %v", err)
	}
	uploader := &fakeUploader{}
	events := &eventRecorder{}
	service := newFileWatchService()
	service.setTempService(temp)
	service.setUploader(uploader)
	service.setEventEmitter(events.emit)
	// Tight timings so tests stay deterministic and fast.
	service.pollInterval = 5 * time.Millisecond
	service.debounce = 5 * time.Millisecond
	t.Cleanup(func() {
		if service.stopped != nil {
			close(service.stopped)
		}
	})
	return service, uploader, events, temp.Root()
}

func watchTestFile(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestFileWatchSyncsSavesBackToRemote(t *testing.T) {
	service, uploader, events, root := newWatchTestService(t)
	localPath := watchTestFile(t, root, "edit-note.txt", "v1")

	result, err := service.StartFileWatch(localPath, "/remote/edit-note.txt", "sftp-1", "utf-8")
	if err != nil {
		t.Fatalf("start watch: %v", err)
	}
	if result.WatchID == "" || result.Reused {
		t.Fatalf("unexpected start result: %+v", result)
	}

	if err := os.WriteFile(localPath, []byte("v1-saved-by-editor"), 0o600); err != nil {
		t.Fatalf("save edit: %v", err)
	}
	waitFor(t, 3*time.Second, func() bool { return uploader.count() > 0 }, "sync-back upload")

	call := uploader.last()
	if call.sessionID != "sftp-1" || call.remotePath != "/remote/edit-note.txt" || call.encoding != "utf-8" {
		t.Fatalf("upload call: %+v", call)
	}
	if call.bytes != int64(len("v1-saved-by-editor")) {
		t.Fatalf("bytes = %d, want %d", call.bytes, len("v1-saved-by-editor"))
	}
	waitFor(t, 3*time.Second, func() bool { return events.count(fileWatchSyncedEvent) > 0 }, "synced event")

	watches, err := service.ListFileWatches()
	if err != nil || len(watches) != 1 || watches[0].WatchID != result.WatchID || watches[0].SftpID != "sftp-1" {
		t.Fatalf("list watches: %+v, %v", watches, err)
	}
}

func TestFileWatchUploadFailureEmitsErrorEvent(t *testing.T) {
	service, uploader, events, root := newWatchTestService(t)
	localPath := watchTestFile(t, root, "edit-err.txt", "v1")
	if _, err := service.StartFileWatch(localPath, "/remote/edit-err.txt", "sftp-1", ""); err != nil {
		t.Fatalf("start watch: %v", err)
	}
	uploader.failNextUpload()
	// Different length so the save is detectable even when both writes land
	// inside one filesystem mtime tick (Windows coarsens timestamps).
	if err := os.WriteFile(localPath, []byte("v2-failed-save"), 0o600); err != nil {
		t.Fatalf("save edit: %v", err)
	}
	waitFor(t, 3*time.Second, func() bool { return events.count(fileWatchErrorEvent) > 0 }, "error event")
	if events.count(fileWatchSyncedEvent) != 0 {
		t.Fatalf("synced event must not fire for a failed upload")
	}
}

func TestFileWatchRegistersAndStops(t *testing.T) {
	service, _, _, root := newWatchTestService(t)
	localPath := watchTestFile(t, root, "edit-stop.txt", "v1")

	if result, err := service.RegisterTempFile("sftp-1", localPath); err != nil || !result.Success {
		t.Fatalf("register temp file: %+v, %v", result, err)
	}

	start, err := service.StartFileWatch(localPath, "/remote/edit-stop.txt", "sftp-1", "")
	if err != nil {
		t.Fatalf("start watch: %v", err)
	}
	// Duplicate start for the same key reuses the watch id.
	replay, err := service.StartFileWatch(localPath, "/remote/edit-stop.txt", "sftp-1", "")
	if err != nil || !replay.Reused || replay.WatchID != start.WatchID {
		t.Fatalf("duplicate start: %+v, %v", replay, err)
	}

	// Explicit stop keeps the registered download (retainer still active).
	if result, err := service.StopFileWatch(start.WatchID, false); err != nil || !result.Success {
		t.Fatalf("stop watch: %+v, %v", result, err)
	}
	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("registered download must survive explicit stop: %v", err)
	}
	// Idempotent second stop.
	if result, err := service.StopFileWatch(start.WatchID, false); err != nil || !result.Success {
		t.Fatalf("second stop: %+v, %v", result, err)
	}

	// Unregister releases the last reference and deletes the download.
	unregister, err := service.UnregisterTempFile("sftp-1", localPath)
	if err != nil || !unregister.Success || unregister.Retained || !unregister.WasTracked {
		t.Fatalf("unregister: %+v, %v", unregister, err)
	}
	waitFor(t, 3*time.Second, func() bool {
		_, statErr := os.Stat(localPath)
		return os.IsNotExist(statErr)
	}, "temp download cleanup")
}

func TestFileWatchUnregisterRetainsParallelReference(t *testing.T) {
	service, _, _, root := newWatchTestService(t)
	localPath := watchTestFile(t, root, "edit-shared.txt", "v1")
	if _, err := service.RegisterTempFile("sftp-1", localPath); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := service.RegisterTempFile("sftp-1", localPath); err != nil {
		t.Fatalf("register: %v", err)
	}
	first, err := service.UnregisterTempFile("sftp-1", localPath)
	if err != nil || !first.Success || !first.Retained {
		t.Fatalf("first unregister: %+v, %v", first, err)
	}
	if _, err := os.Stat(localPath); err != nil {
		t.Fatalf("still-referenced download must be retained: %v", err)
	}
	second, err := service.UnregisterTempFile("sftp-1", localPath)
	if err != nil || !second.Success || second.Retained {
		t.Fatalf("second unregister: %+v, %v", second, err)
	}
	waitFor(t, 3*time.Second, func() bool {
		_, statErr := os.Stat(localPath)
		return os.IsNotExist(statErr)
	}, "last-reference cleanup")
}

func TestFileWatchStopsOnSessionClose(t *testing.T) {
	service, uploader, events, root := newWatchTestService(t)
	localPath := watchTestFile(t, root, "edit-session.txt", "v1")
	if _, err := service.RegisterTempFile("sftp-1", localPath); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := service.StartFileWatch(localPath, "/remote/edit-session.txt", "sftp-1", ""); err != nil {
		t.Fatalf("start watch: %v", err)
	}
	watched := watchTestFile(t, root, "unwatched-session.txt", "v1")
	if _, err := service.RegisterTempFile("sftp-1", watched); err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := service.stopWatchersForSession("sftp-1", true); err != nil {
		t.Fatalf("stop for session: %v", err)
	}
	waitFor(t, 3*time.Second, func() bool { return events.count(fileWatchStoppedEvent) == 1 }, "stopped event")
	watches, err := service.ListFileWatches()
	if err != nil || len(watches) != 0 {
		t.Fatalf("watches after session close: %+v, %v", watches, err)
	}
	for _, path := range []string{localPath, watched} {
		waitFor(t, 3*time.Second, func() bool {
			_, statErr := os.Stat(path)
			return os.IsNotExist(statErr)
		}, "session temp cleanup for "+filepath.Base(path))
	}
	if uploader.count() != 0 {
		t.Fatalf("no upload may run after session close, got %d", uploader.count())
	}
}

func TestFileWatchForceStopsWhenWatchedFileVanishes(t *testing.T) {
	service, _, events, root := newWatchTestService(t)
	localPath := watchTestFile(t, root, "edit-vanish.txt", "v1")
	if _, err := service.StartFileWatch(localPath, "/remote/edit-vanish.txt", "sftp-1", ""); err != nil {
		t.Fatalf("start watch: %v", err)
	}
	if err := os.Remove(localPath); err != nil {
		t.Fatalf("remove watched file: %v", err)
	}
	waitFor(t, 3*time.Second, func() bool { return events.count(fileWatchStoppedEvent) == 1 }, "stopped event")
	watches, _ := service.ListFileWatches()
	if len(watches) != 0 {
		t.Fatalf("watch must be gone: %+v", watches)
	}
	// Restarting over the same key after a force-stop must not wedge.
	replay, err := service.StartFileWatch(localPath, "/remote/edit-vanish.txt", "sftp-1", "")
	if err == nil {
		t.Cleanup(func() { _, _ = service.StopFileWatch(replay.WatchID, false) })
		t.Fatalf("start on a missing file must fail, got %+v", replay)
	}
}

func TestFileWatchRejectsPathsOutsideManagedTemp(t *testing.T) {
	service, _, _, _ := newWatchTestService(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartFileWatch(outside, "/remote/outside.txt", "sftp-1", ""); err == nil {
		t.Fatal("watch outside the managed temp root must be rejected")
	}
	if result, err := service.RegisterTempFile("sftp-1", outside); err != nil || result.Success {
		t.Fatalf("register outside temp: %+v, %v", result, err)
	}
	if _, err := service.StartFileWatch("", "/remote/x", "sftp-1", ""); err == nil {
		t.Fatal("empty localPath must be rejected")
	}
	if _, err := service.StartFileWatch(filepath.Join("rel", "file.txt"), "/remote/x", "sftp-1", ""); err == nil {
		t.Fatal("missing file must be rejected")
	}
}

func TestSFTPCloseObserverStopsWatchers(t *testing.T) {
	service, _, events, root := newWatchTestService(t)
	localPath := watchTestFile(t, root, "edit-close.txt", "v1")
	if _, err := service.RegisterTempFile("sftp-close", localPath); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := service.StartFileWatch(localPath, "/remote/edit-close.txt", "sftp-close", ""); err != nil {
		t.Fatalf("start watch: %v", err)
	}

	sftpService := NewSFTPService(nil, nil)
	sftpService.setSessionCloseObserver(func(sessionID string) {
		_ = service.stopWatchersForSession(sessionID, true)
	})
	// A session the core does not know closes as a no-op without touching the
	// observer path is fine; exercise the observer directly through the same
	// helper the close paths use.
	sftpService.notifySessionClosed("sftp-close")

	waitFor(t, 3*time.Second, func() bool { return events.count(fileWatchStoppedEvent) == 1 }, "stopped event")
}
