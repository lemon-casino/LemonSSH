package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/binaricat/netcatty/internal/terminal/zmodem"
)

type terminalZmodemStream struct {
	*io.PipeReader
	writer  *io.PipeWriter
	ctx     context.Context
	cancel  context.CancelFunc
	pending bool
	initial []byte
}

// detectZmodem retains only a possible initial hex header; non-protocol bytes
// are delivered unchanged. Folder selection has a bounded capture deadline.
func (s *TerminalService) detectZmodem(id string, data []byte) bool {
	marker := []byte("**\x18B")
	s.mu.Lock()
	term := s.sessions[id]
	if term == nil {
		s.mu.Unlock()
		return false
	}
	buf := append(term.zmodemPrefix, data...)
	term.zmodemPrefix = nil
	index := bytes.Index(buf, marker)
	if index < 0 {
		keep := 0
		for n := 1; n < len(marker) && n <= len(buf); n++ {
			if bytes.Equal(buf[len(buf)-n:], marker[:n]) {
				keep = n
			}
		}
		if keep > 0 {
			term.zmodemPrefix = append([]byte(nil), buf[len(buf)-keep:]...)
			s.mu.Unlock()
			if len(buf) > keep {
				_ = s.publishTerminalBytes(id, buf[:len(buf)-keep])
			}
			return true
		}
		s.mu.Unlock()
		if len(buf) != len(data) {
			_ = s.publishTerminalBytes(id, buf)
			return true
		}
		return false
	}
	if len(buf)-index < 18 {
		term.zmodemPrefix = append([]byte(nil), buf[index:]...)
		s.mu.Unlock()
		if index > 0 {
			_ = s.publishTerminalBytes(id, buf[:index])
		}
		return true
	}
	frame, _, err := zmodem.ParseBinaryHeader(buf[index:])
	s.mu.Unlock()
	if err != nil || frame.Type != zmodem.FrameZRQINIT {
		_ = s.publishTerminalBytes(id, buf)
		return true
	}
	if index > 0 {
		_ = s.publishTerminalBytes(id, buf[:index])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	stream, err := s.beginZmodem(id, ctx, cancel)
	if err != nil {
		cancel()
		return false
	}
	s.mu.Lock()
	stream.pending = true
	stream.initial = append([]byte(nil), buf[index:]...)
	s.mu.Unlock()
	go func() {
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		s.mu.Lock()
		pending := stream.pending
		s.mu.Unlock()
		if pending {
			s.endZmodem(id, stream)
		}
	}()
	s.emit("terminal:zmodem", map[string]any{"type": "detect", "sessionId": id, "transferType": "download"})
	return true
}

func (stream *terminalZmodemStream) Read(data []byte) (int, error) {
	if len(stream.initial) > 0 {
		n := copy(data, stream.initial)
		stream.initial = stream.initial[n:]
		return n, nil
	}
	return stream.PipeReader.Read(data)
}

func (stream *terminalZmodemStream) readContext(ctx context.Context, data []byte) (int, error) {
	stop := context.AfterFunc(ctx, func() { _ = stream.PipeReader.CloseWithError(ctx.Err()) })
	defer stop()
	return stream.Read(data)
}

func (s *TerminalService) beginZmodem(id string, ctx context.Context, cancel context.CancelFunc) (*terminalZmodemStream, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	term, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	if term.zmodem != nil {
		return nil, fmt.Errorf("ZMODEM transfer already active")
	}
	r, w := io.Pipe()
	stream := &terminalZmodemStream{PipeReader: r, writer: w, ctx: ctx, cancel: cancel}
	term.zmodem = stream
	go func() { <-ctx.Done(); _ = w.CloseWithError(ctx.Err()); _ = r.CloseWithError(ctx.Err()) }()
	return stream, nil
}
func (s *TerminalService) endZmodem(id string, stream *terminalZmodemStream) {
	stream.cancel()
	_ = stream.Close()
	_ = stream.writer.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	if term := s.sessions[id]; term != nil && term.zmodem == stream {
		term.zmodem = nil
	}
}

// SendZmodem holds the session's raw byte stream until the peer acknowledges
// completion. The native terminal writer is shared by data and protocol replies.
func (s *TerminalService) SendZmodem(sessionID, filePath, remoteName, command string) (resultErr error) {
	defer func() {
		kind := "complete"
		if resultErr != nil {
			kind = "error"
		}
		payload := map[string]any{"type": kind, "sessionId": sessionID, "transferType": "upload", "filename": remoteName}
		if resultErr != nil {
			payload["error"] = resultErr.Error()
		}
		s.emit("terminal:zmodem", payload)
	}()
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	if remoteName == "" {
		remoteName = filepath.Base(filePath)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	stream, err := s.beginZmodem(sessionID, ctx, cancel)
	if err != nil {
		cancel()
		return err
	}
	defer s.endZmodem(sessionID, stream)
	if command != "" {
		if _, err := s.Write(sessionID, []byte(command+"\r")); err != nil {
			return err
		}
	}
	s.emit("terminal:zmodem", map[string]any{"type": "detect", "sessionId": sessionID, "transferType": "upload", "filename": remoteName, "total": len(data)})
	sender := zmodem.Sender{Write: func(data []byte) error { _, err := s.Write(sessionID, data); return err }, Read: func(ctx context.Context, data []byte) (int, error) { return stream.readContext(ctx, data) }, OnProgress: func(transferred, total int64) {
		s.emit("terminal:zmodem", map[string]any{"type": "progress", "sessionId": sessionID, "transferType": "upload", "filename": remoteName, "transferred": transferred, "total": total})
	}}
	return sender.SendFile(ctx, zmodem.FileMeta{Name: remoteName, Size: int64(len(data))}, data)
}

// ReceiveZmodem refuses existing targets; transfer bytes are written only after
// validated metadata, and partial files are removed if negotiation fails.
func (s *TerminalService) ReceiveZmodem(sessionID, destinationDir string) (resultErr error) {
	defer func() {
		kind := "complete"
		if resultErr != nil {
			kind = "error"
		}
		payload := map[string]any{"type": kind, "sessionId": sessionID, "transferType": "download"}
		if resultErr != nil {
			payload["error"] = resultErr.Error()
		}
		s.emit("terminal:zmodem", payload)
	}()
	var filename string
	var transferred, total int64
	info, err := os.Stat(destinationDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("destination must be a directory")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	s.mu.Lock()
	var stream *terminalZmodemStream
	if term := s.sessions[sessionID]; term != nil && term.zmodem != nil && term.zmodem.pending {
		stream = term.zmodem
		stream.pending = false
	}
	s.mu.Unlock()
	if stream != nil {
		cancel()
		ctx = stream.ctx
		cancel = stream.cancel
	} else {
		stream, err = s.beginZmodem(sessionID, ctx, cancel)
	}
	if err != nil {
		cancel()
		return err
	}
	defer s.endZmodem(sessionID, stream)
	var file *os.File
	var partial string
	defer func() {
		if file != nil {
			file.Close()
			os.Remove(partial)
		}
	}()
	receiver := zmodem.Receiver{
		Write: func(data []byte) error { _, err := s.Write(sessionID, data); return err },
		OnFileStart: func(meta zmodem.FileMeta) error {
			filename = meta.Name
			transferred = 0
			total = meta.Size
			s.emit("terminal:zmodem", map[string]any{"type": "detect", "sessionId": sessionID, "transferType": "download", "filename": filename, "total": total})
			if file != nil {
				return fmt.Errorf("previous file not complete")
			}
			if filepath.Base(meta.Name) != meta.Name || meta.Name == "." || meta.Name == ".." {
				return fmt.Errorf("invalid transfer filename")
			}
			partial = filepath.Join(destinationDir, meta.Name)
			var err error
			file, err = os.OpenFile(partial, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			return err
		},
		OnChunk: func(data []byte) error {
			if file == nil {
				return fmt.Errorf("no transfer file")
			}
			n, err := file.Write(data)
			transferred += int64(n)
			s.emit("terminal:zmodem", map[string]any{"type": "progress", "sessionId": sessionID, "transferType": "download", "filename": filename, "transferred": transferred, "total": total})
			return err
		},
		OnFileEnd: func(meta zmodem.FileMeta) error {
			if file == nil {
				return fmt.Errorf("no transfer file")
			}
			if err := file.Close(); err != nil {
				return err
			}
			file = nil
			return nil
		},
	}
	if err := receiver.Start(ctx); err != nil {
		return err
	}
	buf := make([]byte, 32*1024)
	for !receiver.Done {
		n, err := stream.Read(buf)
		if n > 0 {
			if _, err := receiver.FeedSession(ctx, buf[:n]); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}
