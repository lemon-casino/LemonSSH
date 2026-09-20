package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/binaricat/netcatty/internal/agent/providers"
)

type providerStream struct {
	cancel   context.CancelFunc
	idle     *time.Timer
	timedOut atomic.Bool
}

type ProviderStreamResult struct {
	OK         bool
	StatusCode int
	StatusText string
	Error      string
	Aborted    bool
}

// ChatStream returns when response headers arrive; the response body continues
// on its own cancellable context because a Wails call ends after this return.
func (s *ProviderFetchService) ChatStream(id string, request ProviderFetchRequest, idleTimeoutMs int) ProviderStreamResult {
	if id == "" || s.emit == nil {
		return ProviderStreamResult{Error: "Native AI streaming is not available"}
	}
	idle := 2 * time.Minute
	if idleTimeoutMs > 0 {
		idle = min(time.Duration(idleTimeoutMs)*time.Millisecond, 30*time.Minute)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	stream := &providerStream{cancel: cancel}
	s.mu.Lock()
	for key, at := range s.cancelled {
		if time.Since(at) > 5*time.Minute {
			delete(s.cancelled, key)
		}
	}
	if _, cancelled := s.cancelled[id]; cancelled {
		s.mu.Unlock()
		cancel()
		return ProviderStreamResult{Aborted: true, Error: "Request cancelled"}
	}
	if s.streams[id] != nil {
		s.mu.Unlock()
		cancel()
		return ProviderStreamResult{Error: "Stream request is already running"}
	}
	s.streams[id] = stream
	s.mu.Unlock()
	stream.idle = time.AfterFunc(idle, func() { stream.timedOut.Store(true); cancel() })
	request.Method = http.MethodPost
	var err error
	request, err = s.authorizeProviderRequest(request)
	if err != nil {
		s.finishStream(id, stream)
		return ProviderStreamResult{Error: err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, request.URL, strings.NewReader(request.Body))
	if err != nil {
		s.finishStream(id, stream)
		return ProviderStreamResult{Error: "Invalid provider URL"}
	}
	for name, value := range request.Headers {
		req.Header.Set(name, value)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "text/event-stream")
	response, err := s.client(request).Do(req)
	if err != nil {
		s.finishStream(id, stream)
		return ProviderStreamResult{Error: streamFailure(stream, err), Aborted: errors.Is(err, context.Canceled) && !stream.timedOut.Load()}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		defer s.finishStream(id, stream)
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return ProviderStreamResult{OK: true, StatusCode: response.StatusCode, StatusText: providerResponseError(response.StatusCode, body)}
	}
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		defer response.Body.Close()
		defer s.finishStream(id, stream)
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return ProviderStreamResult{Error: "AI provider did not return an event stream: " + providerResponseError(response.StatusCode, body)}
	}
	go func() {
		defer response.Body.Close()
		defer s.finishStream(id, stream)
		parser := &providers.SSEParser{}
		buffer := make([]byte, 16<<10)
		for {
			n, readErr := response.Body.Read(buffer)
			if n > 0 {
				stream.idle.Reset(idle)
				for _, event := range parser.Feed(buffer[:n]) {
					s.emit("ai:stream-data", map[string]any{"requestId": id, "data": event.Data, "event": event.Event})
					if event.Data == "[DONE]" {
						s.emit("ai:stream-end", map[string]any{"requestId": id})
						return
					}
				}
			}
			if readErr != nil {
				if readErr == io.EOF {
					s.emit("ai:stream-end", map[string]any{"requestId": id})
				} else {
					s.emit("ai:stream-error", map[string]any{"requestId": id, "error": streamFailure(stream, readErr)})
				}
				return
			}
		}
	}()
	return ProviderStreamResult{OK: true, StatusCode: response.StatusCode, StatusText: http.StatusText(response.StatusCode)}
}

func streamFailure(stream *providerStream, err error) string {
	if stream.timedOut.Load() {
		return "AI provider response timed out waiting for stream data"
	}
	if errors.Is(err, context.Canceled) {
		return "Request cancelled"
	}
	return providerFetchError(err)
}

func (s *ProviderFetchService) finishStream(id string, stream *providerStream) {
	stream.idle.Stop()
	stream.cancel()
	s.mu.Lock()
	if s.streams[id] == stream {
		delete(s.streams, id)
	}
	s.mu.Unlock()
}

func (s *ProviderFetchService) ChatCancel(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" {
		return false
	}
	if len(s.cancelled) >= 1024 {
		oldest, at := "", time.Now()
		for key, when := range s.cancelled {
			if oldest == "" || when.Before(at) {
				oldest, at = key, when
			}
		}
		delete(s.cancelled, oldest)
	}
	s.cancelled[id] = time.Now()
	if stream := s.streams[id]; stream != nil {
		stream.cancel()
		return true
	}
	return false
}

func (s *ProviderFetchService) closeStreams() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, stream := range s.streams {
		stream.cancel()
	}
}

func providerResponseError(status int, data []byte) string {
	var body map[string]any
	if json.Unmarshal(data, &body) == nil {
		if nested, ok := body["error"].(map[string]any); ok {
			if message, ok := nested["message"].(string); ok {
				return message
			}
		}
		for _, field := range []string{"error", "message", "detail"} {
			if message, ok := body[field].(string); ok {
				return message
			}
		}
	}
	if text := strings.TrimSpace(string(data)); text != "" {
		if len(text) > 4096 {
			text = text[:4096]
		}
		return text
	}
	return fmt.Sprintf("HTTP %d %s", status, http.StatusText(status))
}
