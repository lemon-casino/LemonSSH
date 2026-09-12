package main

import (
	"archive/zip"
	"context"
	"fmt"
	"github.com/binaricat/netcatty/internal/platform/filesystem"
	"github.com/binaricat/netcatty/internal/terminal/transfer"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type TransferStartRequest struct {
	SourceHostID           string `json:"sourceHostId,omitempty"`
	TargetHostID           string `json:"targetHostId,omitempty"`
	ParentTaskID           string `json:"parentTaskId,omitempty"`
	DirectoryEntryIndex    *int   `json:"directoryEntryIndex,omitempty"`
	DirectoryEntryIdentity string `json:"directoryEntryIdentity,omitempty"`
	TaskID                 string `json:"taskId"`
	SourceSessionID        string `json:"sourceSessionId"`
	TargetSessionID        string `json:"targetSessionId"`
	SourcePath             string `json:"sourcePath"`
	TargetPath             string `json:"targetPath"`
}

type transferFile interface {
	io.ReaderAt
	io.WriterAt
	io.Closer
	Stat() (os.FileInfo, error)
	Truncate(int64) error
}
type transferFileIO struct{ source, target transferFile }

func (f transferFileIO) ReadChunk(ctx context.Context, _ transfer.TaskSpec, chunk transfer.Chunk) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data := make([]byte, chunk.Length)
	n, err := f.source.ReadAt(data, chunk.Offset)
	if n == len(data) {
		err = nil
	}
	return data[:n], err
}
func (f transferFileIO) WriteChunk(ctx context.Context, _ transfer.TaskSpec, chunk transfer.Chunk, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	n, err := f.target.WriteAt(data, chunk.Offset)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}

type TransferSnapshot struct {
	Phase       string `json:"phase,omitempty"`
	ControlKind string `json:"controlKind,omitempty"`
	transfer.Progress
	SourceSessionID        string `json:"sourceSessionId"`
	TargetSessionID        string `json:"targetSessionId"`
	SourcePath             string `json:"sourcePath"`
	TargetPath             string `json:"targetPath"`
	SourceHostID           string `json:"sourceHostId,omitempty"`
	TargetHostID           string `json:"targetHostId,omitempty"`
	ParentTaskID           string `json:"parentTaskId,omitempty"`
	DirectoryEntryIndex    *int   `json:"directoryEntryIndex,omitempty"`
	DirectoryEntryIdentity string `json:"directoryEntryIdentity,omitempty"`
}

type TransferService struct {
	temp       *filesystem.TempService
	compressed map[string]*compressedTransfer
	requests   map[string]TransferStartRequest
	mu         sync.Mutex
	sftp       *SFTPService
	scheduler  *transfer.Scheduler
}

func (s *TransferService) setTempService(temp *filesystem.TempService) { s.temp = temp }

func (s *TransferService) setSFTPService(service *SFTPService) { s.sftp = service }

func (s *TransferService) openFile(sessionID, path string, flags int) (transferFile, func(), error) {
	if sessionID == "" {
		f, err := os.OpenFile(path, flags, 0600)
		return f, func() {}, err
	}
	if s.sftp == nil {
		return nil, nil, fmt.Errorf("SFTP service unavailable")
	}
	client, release, err := s.sftp.acquire(sessionID)
	if err != nil {
		return nil, nil, err
	}
	f, err := client.raw.OpenFile(path, flags)
	if err != nil {
		release()
		return nil, nil, err
	}
	return f, release, nil
}

func (s *TransferService) Start(request TransferStartRequest) (transfer.Progress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.compressed[request.TaskID] != nil {
		return transfer.Progress{}, fmt.Errorf("duplicate transfer task")
	}
	return s.startLocked(request)
}

func (s *TransferService) startLocked(request TransferStartRequest) (transfer.Progress, error) {
	if request.TaskID == "" || request.SourcePath == "" || request.TargetPath == "" {
		return transfer.Progress{}, fmt.Errorf("task and paths required")
	}
	if _, err := s.scheduler.Progress(request.TaskID); err == nil {
		return transfer.Progress{}, fmt.Errorf("duplicate transfer task")
	}
	if request.SourceSessionID == request.TargetSessionID && filepath.Clean(request.SourcePath) == filepath.Clean(request.TargetPath) {
		return transfer.Progress{}, fmt.Errorf("source and destination are identical")
	}
	source, releaseSource, err := s.openFile(request.SourceSessionID, request.SourcePath, os.O_RDONLY)
	if err != nil {
		return transfer.Progress{}, err
	}
	keep := false
	defer func() {
		if !keep {
			source.Close()
			releaseSource()
		}
	}()
	info, err := source.Stat()
	if err != nil {
		return transfer.Progress{}, err
	}
	if info.IsDir() {
		return transfer.Progress{}, fmt.Errorf("directory transfer requires a file walk")
	}
	target, releaseTarget, err := s.openFile(request.TargetSessionID, request.TargetPath, os.O_CREATE|os.O_RDWR)
	if err != nil {
		return transfer.Progress{}, err
	}
	defer func() {
		if !keep {
			target.Close()
			releaseTarget()
		}
	}()
	targetInfo, err := target.Stat()
	if err != nil {
		return transfer.Progress{}, err
	}
	if request.SourceSessionID == "" && request.TargetSessionID == "" && os.SameFile(info, targetInfo) {
		return transfer.Progress{}, fmt.Errorf("source and destination refer to the same file")
	}
	if err := target.Truncate(info.Size()); err != nil {
		return transfer.Progress{}, err
	}
	spec := transfer.TaskSpec{TaskID: request.TaskID, HostKey: request.SourceSessionID + ":" + request.TargetSessionID, SourcePath: request.SourcePath, SinkPath: request.TargetPath, TotalBytes: info.Size()}
	if _, err := s.scheduler.Enqueue(spec); err != nil {
		return transfer.Progress{}, err
	}
	files := transferFileIO{source, target}
	if err := s.scheduler.Start(context.Background(), request.TaskID, files, files); err != nil {
		return transfer.Progress{}, err
	}
	if s.requests == nil {
		s.requests = make(map[string]TransferStartRequest)
	}
	s.requests[request.TaskID] = request
	keep = true
	go func() {
		defer source.Close()
		defer target.Close()
		defer releaseSource()
		defer releaseTarget()
		_ = s.scheduler.Wait(context.Background(), request.TaskID)
	}()
	return s.scheduler.Progress(request.TaskID)
}

type compressedTransfer struct {
	progress transfer.Progress
	phase    string
	folder   string
}

// StartCompressed stages ZIP bytes under the shared managed temp root, then uses
// the ordinary scheduler. Compression and upload share identity and controls.
func (s *TransferService) StartCompressed(request TransferStartRequest) (TransferSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.temp == nil {
		return TransferSnapshot{}, fmt.Errorf("managed temp service unavailable")
	}
	if request.TaskID == "" || request.SourcePath == "" || request.TargetPath == "" {
		return TransferSnapshot{}, fmt.Errorf("task and paths required")
	}
	if s.compressed[request.TaskID] != nil {
		return TransferSnapshot{}, fmt.Errorf("duplicate transfer task")
	}
	if _, err := s.scheduler.Progress(request.TaskID); err == nil {
		return TransferSnapshot{}, fmt.Errorf("duplicate transfer task")
	}
	info, err := os.Stat(request.SourcePath)
	if err != nil {
		return TransferSnapshot{}, err
	}
	if !info.IsDir() {
		return TransferSnapshot{}, fmt.Errorf("compressed source must be a directory")
	}
	if s.compressed == nil {
		s.compressed = make(map[string]*compressedTransfer)
	}
	if s.requests == nil {
		s.requests = make(map[string]TransferStartRequest)
	}
	job := &compressedTransfer{phase: "compressing", folder: request.SourcePath, progress: transfer.Progress{TaskID: request.TaskID, State: transfer.StateRunning}}
	s.compressed[request.TaskID] = job
	s.requests[request.TaskID] = request
	go s.compressAndStart(request, job)
	return s.snapshotLocked(request.TaskID)
}

func (s *TransferService) compressionGate(job *compressedTransfer) error {
	for {
		s.mu.Lock()
		state := job.progress.State
		s.mu.Unlock()
		if state == transfer.StateCancelled {
			return context.Canceled
		}
		if state != transfer.StatePaused {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type compressionReader struct {
	io.Reader
	service *TransferService
	job     *compressedTransfer
}

func (r compressionReader) Read(buffer []byte) (int, error) {
	if err := r.service.compressionGate(r.job); err != nil {
		return 0, err
	}
	n, err := r.Reader.Read(buffer)
	r.service.mu.Lock()
	r.job.progress.DoneBytes += int64(n)
	r.service.mu.Unlock()
	return n, err
}

func (s *TransferService) compressAndStart(request TransferStartRequest, job *compressedTransfer) {
	var runErr error
	defer func() {
		if runErr != nil {
			s.mu.Lock()
			if job.progress.State != transfer.StateCancelled {
				job.progress.State = transfer.StateFailed
				job.progress.Err = runErr.Error()
			}
			s.mu.Unlock()
		}
	}()
	// Active archives live in a private subdirectory that ClearTemp preserves.
	staging, err := os.MkdirTemp(s.temp.Root(), "active-transfer-")
	if err != nil {
		runErr = err
		return
	}
	defer os.RemoveAll(staging)
	archivePath := filepath.Join(staging, "upload.zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		runErr = err
		return
	}
	defer archive.Close()
	writer := zip.NewWriter(archive)
	runErr = filepath.Walk(request.SourcePath, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := s.compressionGate(job); err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("compressed upload does not follow symlinks: %s", path)
		}
		if info.Mode().IsRegular() {
			s.mu.Lock()
			job.progress.TotalBytes += info.Size()
			s.mu.Unlock()
		}
		return nil
	})
	if runErr == nil {
		runErr = filepath.Walk(request.SourcePath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := s.compressionGate(job); err != nil {
				return err
			}
			rel, err := filepath.Rel(request.SourcePath, path)
			if err != nil || rel == "." {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("compressed upload does not follow symlinks: %s", path)
			}
			name := filepath.ToSlash(rel)
			if info.IsDir() {
				_, err = writer.Create(name + "/")
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("unsupported file: %s", path)
			}
			entry, err := writer.Create(name)
			if err != nil {
				return err
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(entry, compressionReader{Reader: file, service: s, job: job})
			return err
		})
	}
	if err := writer.Close(); runErr == nil {
		runErr = err
	}
	if err := archive.Close(); runErr == nil {
		runErr = err
	}
	if runErr != nil {
		return
	}
	for {
		if runErr = s.compressionGate(job); runErr != nil {
			return
		}
		s.mu.Lock()
		if job.progress.State == transfer.StatePaused {
			s.mu.Unlock()
			continue
		}
		if job.progress.State == transfer.StateCancelled {
			s.mu.Unlock()
			return
		}
		request.SourcePath = archivePath
		request.SourceSessionID = ""
		_, runErr = s.startLocked(request)
		if runErr == nil {
			job.phase = "uploading"
			job.progress.LifecycleEpoch++
		}
		s.mu.Unlock()
		break
	}
	if runErr == nil {
		runErr = s.scheduler.Wait(context.Background(), request.TaskID)
	}
}

func newTransferService() *TransferService {
	return &TransferService{scheduler: transfer.New()}
}

func (s *TransferService) Enqueue(spec transfer.TaskSpec) (transfer.Progress, error) {
	return s.scheduler.Enqueue(spec)
}

func (s *TransferService) Pause(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job := s.compressed[taskID]; job != nil && job.phase == "compressing" {
		if job.progress.State != transfer.StateRunning {
			return transfer.ErrTaskNotRunning
		}
		job.progress.State = transfer.StatePaused
		job.progress.LifecycleEpoch++
		return nil
	}
	return s.scheduler.Pause(taskID)
}

func (s *TransferService) Resume(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job := s.compressed[taskID]; job != nil && job.phase == "compressing" {
		if job.progress.State != transfer.StatePaused {
			return transfer.ErrTaskNotRunning
		}
		job.progress.State = transfer.StateRunning
		job.progress.LifecycleEpoch++
		return nil
	}
	return s.scheduler.Resume(taskID)
}

func (s *TransferService) Cancel(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if job := s.compressed[taskID]; job != nil && job.phase == "compressing" {
		job.progress.State = transfer.StateCancelled
		job.progress.LifecycleEpoch++
		return nil
	}
	return s.scheduler.Cancel(taskID)
}

func (s *TransferService) snapshotLocked(taskID string) (TransferSnapshot, error) {
	job := s.compressed[taskID]
	var progress transfer.Progress
	var err error
	if job != nil && job.phase == "compressing" {
		progress = job.progress
	} else {
		progress, err = s.scheduler.Progress(taskID)
	}
	if err != nil {
		return TransferSnapshot{}, err
	}
	request := s.requests[taskID]
	phase, kind := "", ""
	if job != nil {
		phase, kind = job.phase, "compressed-upload"
		if job.phase == "uploading" {
			progress.LifecycleEpoch += job.progress.LifecycleEpoch
			request.SourcePath = job.folder
		}
	}
	return TransferSnapshot{Phase: phase, ControlKind: kind, Progress: progress, SourceSessionID: request.SourceSessionID, TargetSessionID: request.TargetSessionID,
		SourcePath: request.SourcePath, TargetPath: request.TargetPath, SourceHostID: request.SourceHostID, TargetHostID: request.TargetHostID,
		ParentTaskID: request.ParentTaskID, DirectoryEntryIndex: request.DirectoryEntryIndex, DirectoryEntryIdentity: request.DirectoryEntryIdentity}, nil
}

func (s *TransferService) Progress(taskID string) (TransferSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotLocked(taskID)
}

// List lets a replacement webview observe jobs without restarting their I/O.
func (s *TransferService) List() ([]TransferSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]TransferSnapshot, 0, len(s.requests))
	for id := range s.requests {
		snapshot, err := s.snapshotLocked(id)
		if err != nil {
			return nil, err
		}
		result = append(result, snapshot)
	}
	return result, nil
}
