package main

// FileWatchService restores the external-editor auto-sync chain on the Wails
// runtime. A remote file downloaded to the managed temp dir and opened in an
// external editor is polled for saves; each save is streamed back to its
// remote SFTP source and reported through renderer events:
//
//	lemonssh:filewatch:synced   { watchId, localPath, remotePath, bytesWritten }
//	lemonssh:filewatch:error    { watchId, localPath, remotePath, error }
//	lemonssh:filewatch:stopped  { watchId, localPath, remotePath, sftpId }
//
// The state machine mirrors the retired Electron fileWatcherBridge
// (electron/bridges/fileWatcherBridge.cjs, removed in bcd94770) that the
// renderer hooks (useSftpFileWatch / useSftpExternalOperations) were written
// against. Watching uses mtime/size polling rather than fsnotify on purpose:
// editors save via temp-file + rename, which inotify-style watchers miss, and
// the old bridge documented the same rationale for choosing fs.watchFile.
// Polling also keeps the trigger path deterministic for unit tests without a
// platform event dependency.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/binaricat/lemonssh/internal/platform/applog"
	"github.com/binaricat/lemonssh/internal/platform/filesystem"
)

// fileWatchIDSequence issues process-unique watch ids.
var fileWatchIDSequence atomic.Uint64

func newFileWatchID() string {
	return fmt.Sprintf("%d", fileWatchIDSequence.Add(1))
}

const (
	// maxActiveFileWatches bounds renderer-requested watches; the renderer
	// opens one watch per external-edit window.
	maxActiveFileWatches = 256
	// fileWatchEventPrefix namespaces the renderer events.
	fileWatchSyncedEvent  = "lemonssh:filewatch:synced"
	fileWatchErrorEvent   = "lemonssh:filewatch:error"
	fileWatchStoppedEvent = "lemonssh:filewatch:stopped"
)

// cleanupRetryDelays mirrors the retired bridge's best-effort temp cleanup:
// Windows keeps a deleting file busy while the editor still holds it open.
var cleanupRetryDelays = []time.Duration{100 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond, 1000 * time.Millisecond, 2000 * time.Millisecond}

// FileWatchStartResult reports the watch id and whether an existing watch for
// the same (sftpId, localPath, remotePath) was reused.
type FileWatchStartResult struct {
	WatchID string `json:"watchId"`
	Reused  bool   `json:"reused"`
}

// FileWatchResult is the shared boolean outcome shape used by stop/register.
type FileWatchResult struct {
	Success bool `json:"success"`
}

// FileWatchUnregisterResult reports whether the temp file is still retained by
// another registered reference and whether it was tracked at all.
type FileWatchUnregisterResult struct {
	Success    bool `json:"success"`
	Retained   bool `json:"retained,omitempty"`
	WasTracked bool `json:"wasTracked,omitempty"`
}

// FileWatchSummary is one active watch in ListFileWatches output.
type FileWatchSummary struct {
	WatchID    string `json:"watchId"`
	LocalPath  string `json:"localPath"`
	RemotePath string `json:"remotePath"`
	SftpID     string `json:"sftpId"`
}

// FileWatchSyncedEvent is emitted after a save streamed back successfully.
type FileWatchSyncedEvent struct {
	WatchID      string `json:"watchId"`
	LocalPath    string `json:"localPath"`
	RemotePath   string `json:"remotePath"`
	BytesWritten int64  `json:"bytesWritten"`
}

// FileWatchErrorEvent is emitted when a sync-back upload fails.
type FileWatchErrorEvent struct {
	WatchID    string `json:"watchId"`
	LocalPath  string `json:"localPath"`
	RemotePath string `json:"remotePath"`
	Error      string `json:"error"`
}

// FileWatchStoppedEvent is emitted when a watch ends without renderer request
// (temp file deleted, SFTP session closed).
type FileWatchStoppedEvent struct {
	WatchID    string `json:"watchId"`
	LocalPath  string `json:"localPath"`
	RemotePath string `json:"remotePath"`
	SftpID     string `json:"sftpId"`
}

// fileWatchUploader is the sync-back seam; *SFTPService implements it.
type fileWatchUploader interface {
	Upload(sessionID, localPath, remotePath, encoding string) (int64, error)
}

// fileWatchEntry is one watched temp file. The poller goroutine owns the
// lifecycle; the mutex only serializes the mutable progress fields shared with
// in-flight sync goroutines.
type fileWatchEntry struct {
	watchID    string
	watchKey   string
	localPath  string
	remotePath string
	sftpID     string
	encoding   string

	mu              sync.Mutex
	lastModTime     time.Time
	lastSize        int64
	observedModTime time.Time
	observedSize    int64
	pendingSince    time.Time
	syncing         bool
	stopped         bool
}

func (e *fileWatchEntry) markStopped() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopped {
		return false
	}
	e.stopped = true
	return true
}

func (e *fileWatchEntry) isStopped() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stopped
}

// FileWatchService is the Wails-facing external-edit sync-back service.
type FileWatchService struct {
	mu        sync.Mutex
	watches   map[string]*fileWatchEntry
	watchKeys map[string]string
	// tempFiles tracks renderer-registered external-edit downloads per SFTP
	// session so they are cleaned up when the session closes without a watch.
	tempFiles map[string]map[string]int

	uploader fileWatchUploader
	temp     *filesystem.TempService
	emit     func(name string, payload any)

	pollInterval time.Duration
	debounce     time.Duration
	now          func() time.Time

	started bool
	stopped chan struct{}
	wg      sync.WaitGroup
}

func newFileWatchService() *FileWatchService {
	return &FileWatchService{
		watches:      make(map[string]*fileWatchEntry),
		watchKeys:    make(map[string]string),
		tempFiles:    make(map[string]map[string]int),
		pollInterval: time.Second,
		debounce:     500 * time.Millisecond,
		now:          time.Now,
	}
}

func (s *FileWatchService) setTempService(temp *filesystem.TempService) { s.temp = temp }

func (s *FileWatchService) setEventEmitter(emit func(name string, payload any)) { s.emit = emit }

// setSFTPService wires the sync-back uploader. Called once during startup.
func (s *FileWatchService) setSFTPService(service *SFTPService) { s.uploader = service }

func (s *FileWatchService) setUploader(uploader fileWatchUploader) { s.uploader = uploader }

func (s *FileWatchService) emitEvent(name string, payload any) {
	if s.emit == nil {
		return
	}
	s.emit(name, payload)
}

func buildFileWatchKey(sftpID, localPath, remotePath string) string {
	return sftpID + "\x00" + filepath.Clean(localPath) + "\x00" + remotePath
}

// managedTempRel resolves a local path against the managed temp root and
// rejects anything outside it: only LemonSSH's dedicated temp service may hold
// externally edited downloads.
func (s *FileWatchService) managedTempRel(localPath string) (string, error) {
	if s.temp == nil {
		return "", errors.New("managed temp service unavailable")
	}
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(s.temp.Root(), abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("external-edit watch requires a managed temp file")
	}
	return rel, nil
}

// StartFileWatch watches a managed-temp download and streams every save back
// to its remote source. Duplicate starts for the same key reuse the watch.
func (s *FileWatchService) StartFileWatch(localPath, remotePath, sftpID string, encoding string) (FileWatchStartResult, error) {
	if localPath == "" || remotePath == "" || sftpID == "" {
		return FileWatchStartResult{}, errors.New("localPath, remotePath and sftpId are required")
	}
	rel, err := s.managedTempRel(localPath)
	if err != nil {
		return FileWatchStartResult{}, err
	}
	target, err := s.temp.FilePath(rel)
	if err != nil {
		return FileWatchStartResult{}, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return FileWatchStartResult{}, fmt.Errorf("cannot watch file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return FileWatchStartResult{}, errors.New("cannot watch file: path is not a regular file")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	watchKey := buildFileWatchKey(sftpID, target, remotePath)
	if existingID, ok := s.watchKeys[watchKey]; ok {
		if entry := s.watches[existingID]; entry != nil && !entry.isStopped() {
			return FileWatchStartResult{WatchID: existingID, Reused: true}, nil
		}
		// Stale key from a stopped entry; fall through and re-create.
		delete(s.watchKeys, watchKey)
	}
	if len(s.watches) >= maxActiveFileWatches {
		return FileWatchStartResult{}, fmt.Errorf("too many active external file watches (max %d)", maxActiveFileWatches)
	}
	s.ensureLoopLocked()
	entry := &fileWatchEntry{
		watchID:    fmt.Sprintf("watch-%s", newFileWatchID()),
		watchKey:   watchKey,
		localPath:  target,
		remotePath: remotePath,
		sftpID:     sftpID,
		encoding:   encoding,
	}
	entry.lastModTime = info.ModTime()
	entry.lastSize = info.Size()
	entry.observedModTime = info.ModTime()
	entry.observedSize = info.Size()
	s.watches[entry.watchID] = entry
	s.watchKeys[watchKey] = entry.watchID
	return FileWatchStartResult{WatchID: entry.watchID}, nil
}

// StopFileWatch stops one watch and optionally deletes its temp download.
func (s *FileWatchService) StopFileWatch(watchID string, cleanupTempFile bool) (FileWatchResult, error) {
	s.mu.Lock()
	entry := s.watches[watchID]
	if entry == nil {
		// The owning session cleanup or the renderer may already have
		// released it; stopping twice stays a successful no-op.
		s.mu.Unlock()
		return FileWatchResult{Success: true}, nil
	}
	delete(s.watches, watchID)
	if s.watchKeys[entry.watchKey] == watchID {
		delete(s.watchKeys, entry.watchKey)
	}
	s.mu.Unlock()

	entry.markStopped()
	if cleanupTempFile {
		s.cleanupTempFile(entry.localPath)
	}
	return FileWatchResult{Success: true}, nil
}

// ListFileWatches returns the active watches for diagnostics.
func (s *FileWatchService) ListFileWatches() ([]FileWatchSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	summaries := make([]FileWatchSummary, 0, len(s.watches))
	for _, entry := range s.watches {
		summaries = append(summaries, FileWatchSummary{
			WatchID:    entry.watchID,
			LocalPath:  entry.localPath,
			RemotePath: entry.remotePath,
			SftpID:     entry.sftpID,
		})
	}
	return summaries, nil
}

// RegisterTempFile records a renderer-owned external-edit download so session
// cleanup can delete it even when auto-sync was disabled. Refcounted: parallel
// editors of the same download each register once.
func (s *FileWatchService) RegisterTempFile(sftpID, localPath string) (FileWatchResult, error) {
	if sftpID == "" || localPath == "" {
		return FileWatchResult{Success: false}, nil
	}
	rel, err := s.managedTempRel(localPath)
	if err != nil {
		return FileWatchResult{Success: false}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	files := s.tempFiles[sftpID]
	if files == nil {
		files = make(map[string]int)
		s.tempFiles[sftpID] = files
	}
	files[rel]++
	return FileWatchResult{Success: true}, nil
}

// UnregisterTempFile releases one renderer reference. The last reference
// deletes the managed temp download; a still-referenced file is retained.
func (s *FileWatchService) UnregisterTempFile(sftpID, localPath string) (FileWatchUnregisterResult, error) {
	if sftpID == "" || localPath == "" {
		return FileWatchUnregisterResult{Success: false}, nil
	}
	rel, err := s.managedTempRel(localPath)
	if err != nil {
		return FileWatchUnregisterResult{Success: false}, nil
	}
	s.mu.Lock()
	files := s.tempFiles[sftpID]
	refs := files[rel]
	tracked := refs > 0
	if tracked {
		refs--
		if refs <= 0 {
			delete(files, rel)
		} else {
			files[rel] = refs
		}
	}
	if files != nil && len(files) == 0 {
		delete(s.tempFiles, sftpID)
	}
	s.mu.Unlock()

	retained := s.tempFileRetained(sftpID, rel)
	if !retained {
		// Mirrors the retired bridge: the last (or an untracked) release
		// deletes the download; the renderer relies on unregister to clean up
		// failed external opens.
		s.cleanupTempFile(localPath)
	}
	return FileWatchUnregisterResult{Success: true, Retained: retained, WasTracked: tracked}, nil
}

func (s *FileWatchService) tempFileRetained(sftpID, rel string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tempFiles[sftpID][rel] > 0
}

// stopWatchersForSession force-stops every watch and registered temp download
// bound to an SFTP session. Wired to SFTPService's close path.
func (s *FileWatchService) stopWatchersForSession(sftpID string, cleanupTempFiles bool) error {
	s.mu.Lock()
	watchIDs := make([]string, 0, len(s.watches))
	for watchID, entry := range s.watches {
		if entry.sftpID == sftpID {
			watchIDs = append(watchIDs, watchID)
		}
	}
	var cleanupRels []string
	if cleanupTempFiles {
		for rel := range s.tempFiles[sftpID] {
			cleanupRels = append(cleanupRels, rel)
		}
	}
	delete(s.tempFiles, sftpID)
	entries := make([]*fileWatchEntry, 0, len(watchIDs))
	for _, watchID := range watchIDs {
		entry := s.watches[watchID]
		delete(s.watches, watchID)
		if s.watchKeys[entry.watchKey] == watchID {
			delete(s.watchKeys, entry.watchKey)
		}
		entries = append(entries, entry)
	}
	s.mu.Unlock()

	for _, entry := range entries {
		if entry.markStopped() {
			s.emitEvent(fileWatchStoppedEvent, FileWatchStoppedEvent{
				WatchID:    entry.watchID,
				LocalPath:  entry.localPath,
				RemotePath: entry.remotePath,
				SftpID:     entry.sftpID,
			})
		}
	}
	if cleanupTempFiles {
		for _, rel := range cleanupRels {
			s.cleanupTempRel(rel)
		}
	}
	return nil
}

// cleanupTempFile deletes one managed temp download, retrying briefly while an
// external editor still holds the file open.
func (s *FileWatchService) cleanupTempFile(localPath string) {
	rel, err := s.managedTempRel(localPath)
	if err != nil {
		return
	}
	s.cleanupTempRel(rel)
}

func (s *FileWatchService) cleanupTempRel(rel string) {
	if s.temp == nil {
		return
	}
	if err := s.temp.Remove(rel); err == nil || errors.Is(err, fs.ErrNotExist) {
		return
	}
	// The entry is already marked for removal; retry the raw path in case the
	// editor still holds a lock (Windows sharing violation).
	target, err := s.temp.FilePath(rel)
	if err != nil {
		return
	}
	for _, delay := range cleanupRetryDelays {
		time.Sleep(delay)
		if err := os.Remove(target); err == nil || errors.Is(err, fs.ErrNotExist) {
			return
		}
	}
	applog.Errorf("file watch temp cleanup gave up path=%q", target)
}

// ensureLoopLocked starts the single poller goroutine on first watch.
func (s *FileWatchService) ensureLoopLocked() {
	if s.started {
		return
	}
	s.started = true
	s.stopped = make(chan struct{})
	s.wg.Add(1)
	go s.loop()
}

func (s *FileWatchService) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopped:
			return
		case <-ticker.C:
			s.pollTick()
		}
	}
}

// pollTick scans every watch once: detect mtime/size changes, debounce, then
// hand the sync to a goroutine so one slow upload never blocks detection.
func (s *FileWatchService) pollTick() {
	now := s.now()
	s.mu.Lock()
	entries := make([]*fileWatchEntry, 0, len(s.watches))
	for _, entry := range s.watches {
		entries = append(entries, entry)
	}
	s.mu.Unlock()

	for _, entry := range entries {
		if entry.isStopped() {
			continue
		}
		info, err := os.Stat(entry.localPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// The download vanished (cleanup elsewhere): force-stop and
				// tell the renderer so it drops its retainer.
				s.forceStop(entry, true)
			}
			continue
		}
		entry.mu.Lock()
		// Edge detection against the previous tick's observation: a fresh
		// save (re)arms the debounce window, mirroring the retired bridge's
		// fs.watchFile change edges + clearTimeout/setTimeout behavior. The
		// already-synced version (lastModTime/lastSize) must not be compared
		// here — it stays stale until the debounced sync runs.
		statChanged := !info.ModTime().Equal(entry.observedModTime) || info.Size() != entry.observedSize
		switch {
		case statChanged:
			entry.observedModTime = info.ModTime()
			entry.observedSize = info.Size()
			entry.pendingSince = now
		case !entry.pendingSince.IsZero() && now.Sub(entry.pendingSince) >= s.debounce && !entry.syncing:
			entry.pendingSince = time.Time{}
			entry.syncing = true
			entry.lastModTime = info.ModTime()
			entry.lastSize = info.Size()
			entry.mu.Unlock()
			go s.syncEntry(entry)
			continue
		}
		entry.mu.Unlock()
	}
}

// syncEntry streams one debounced save back to the remote source.
func (s *FileWatchService) syncEntry(entry *fileWatchEntry) {
	defer func() {
		entry.mu.Lock()
		entry.syncing = false
		entry.mu.Unlock()
	}()
	if entry.isStopped() || s.uploader == nil {
		return
	}
	// A sync running while the entry was stopped must not resurrect it.
	s.mu.Lock()
	active := s.watches[entry.watchID] == entry
	s.mu.Unlock()
	if !active {
		return
	}
	if _, err := os.Stat(entry.localPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			s.forceStop(entry, true)
		} else {
			s.emitSyncError(entry, err.Error())
		}
		return
	}
	bytesWritten, err := s.uploader.Upload(entry.sftpID, entry.localPath, entry.remotePath, entry.encoding)
	if entry.isStopped() {
		return
	}
	if err != nil {
		s.emitSyncError(entry, err.Error())
		return
	}
	s.emitEvent(fileWatchSyncedEvent, FileWatchSyncedEvent{
		WatchID:      entry.watchID,
		LocalPath:    entry.localPath,
		RemotePath:   entry.remotePath,
		BytesWritten: bytesWritten,
	})
}

func (s *FileWatchService) emitSyncError(entry *fileWatchEntry, message string) {
	s.emitEvent(fileWatchErrorEvent, FileWatchErrorEvent{
		WatchID:    entry.watchID,
		LocalPath:  entry.localPath,
		RemotePath: entry.remotePath,
		Error:      message,
	})
}

// forceStop removes one watch and emits the stopped event so the renderer can
// forget its retainer.
func (s *FileWatchService) forceStop(entry *fileWatchEntry, cleanupTempFile bool) {
	s.mu.Lock()
	if s.watches[entry.watchID] != entry {
		s.mu.Unlock()
		return
	}
	delete(s.watches, entry.watchID)
	if s.watchKeys[entry.watchKey] == entry.watchID {
		delete(s.watchKeys, entry.watchKey)
	}
	s.mu.Unlock()

	if entry.markStopped() {
		s.emitEvent(fileWatchStoppedEvent, FileWatchStoppedEvent{
			WatchID:    entry.watchID,
			LocalPath:  entry.localPath,
			RemotePath: entry.remotePath,
			SftpID:     entry.sftpID,
		})
	}
	if cleanupTempFile {
		s.cleanupTempFile(entry.localPath)
	}
}
