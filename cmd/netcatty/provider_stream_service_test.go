package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type streamCredentialFixture struct{}

func (streamCredentialFixture) Name() string                                { return "fixture" }
func (streamCredentialFixture) Available() bool                             { return true }
func (streamCredentialFixture) Seal(value []byte, _ string) ([]byte, error) { return value, nil }
func (streamCredentialFixture) Open(value []byte, purpose string) ([]byte, error) {
	if string(value) != "encrypted-fixture" || purpose != "cloud-sync-credentials" {
		return nil, fmt.Errorf("invalid fixture envelope")
	}
	return []byte("fixture-key"), nil
}

type capturedStreamEvent struct {
	name    string
	payload map[string]any
}

func streamTestService(t *testing.T) (*ProviderFetchService, chan capturedStreamEvent) {
	t.Helper()
	s := newTestProviderFetchService()
	events := make(chan capturedStreamEvent, 30)
	s.emit = func(name string, payload any) { events <- capturedStreamEvent{name, payload.(map[string]any)} }
	t.Cleanup(s.closeStreams)
	return s, events
}

func nextStreamEvent(t *testing.T, events <-chan capturedStreamEvent) capturedStreamEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(3 * time.Second):
		t.Fatal("stream event timed out")
		return capturedStreamEvent{}
	}
}

func TestNativeProviderStreamHeadersCredentialsAndNamedUnicodeEvents(t *testing.T) {
	s, events := streamTestService(t)
	s.credentials = streamCredentialFixture{}
	proceed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-key" || r.Header.Get("X-Custom") != "configured" {
			t.Error("configured credentials or headers did not reach provider")
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		select {
		case <-proceed:
		case <-r.Context().Done():
			return
		}
		data := []byte("event: content_block_delta\r\ndata: {\"text\":\"你好\"}\r\ndata: second-line\r\n\r\ndata: [DONE]\n\n")
		for _, b := range data {
			_, _ = w.Write([]byte{b})
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()
	s.SyncProviders([]ProviderEndpointConfig{{ID: "p", BaseURL: server.URL, Enabled: true, APIKey: "enc:v1:" + base64.StdEncoding.EncodeToString([]byte("encrypted-fixture")), CustomHeaders: map[string]string{"X-Custom": "configured"}}})
	result := s.ChatStream("request", ProviderFetchRequest{URL: server.URL, ProviderID: "p", Headers: map[string]string{"Authorization": "Bearer " + providerKeyPlaceholder}}, 1000)
	if !result.OK || result.StatusCode != 200 {
		t.Fatalf("stream headers: %+v", result)
	}
	close(proceed)
	first := nextStreamEvent(t, events)
	if first.name != "ai:stream-data" || first.payload["event"] != "content_block_delta" || first.payload["data"] != "{\"text\":\"你好\"}\nsecond-line" {
		t.Fatalf("SSE data damaged: %v", first)
	}
	if nextStreamEvent(t, events).payload["data"] != "[DONE]" {
		t.Fatal("completion event missing")
	}
	if nextStreamEvent(t, events).name != "ai:stream-end" {
		t.Fatal("stream did not end")
	}
}

func TestNativeProviderStreamPreservesHttpErrors(t *testing.T) {
	s, _ := streamTestService(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"message":"key rejected"}}`))
	}))
	defer server.Close()
	s.AllowlistAddHost(server.URL)
	result := s.ChatStream("auth", ProviderFetchRequest{URL: server.URL}, 1000)
	if !result.OK || result.StatusCode != 401 || result.StatusText != "key rejected" {
		t.Fatalf("real status lost: %+v", result)
	}
}

func TestNativeProviderStreamCancelAndIdleDeadline(t *testing.T) {
	for _, idle := range []bool{false, true} {
		t.Run(fmt.Sprint(idle), func(t *testing.T) {
			s, events := streamTestService(t)
			closed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(closed)
			}))
			defer server.Close()
			s.AllowlistAddHost(server.URL)
			timeout := 10000
			if idle {
				timeout = 60
			}
			if result := s.ChatStream("stalled", ProviderFetchRequest{URL: server.URL}, timeout); !result.OK {
				t.Fatal(result)
			}
			if !idle {
				s.ChatCancel("stalled")
			}
			event := nextStreamEvent(t, events)
			if event.name != "ai:stream-error" {
				t.Fatalf("missing stream error: %v", event)
			}
			if idle && !strings.Contains(event.payload["error"].(string), "timed out") {
				t.Fatal(event)
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("provider connection was not cancelled")
			}
		})
	}
}

func TestNativeProviderStreamCancelBeforeStartAndEndpointKeyBinding(t *testing.T) {
	s, _ := streamTestService(t)
	s.ChatCancel("cancelled")
	if result := s.ChatStream("cancelled", ProviderFetchRequest{URL: "https://api.openai.com/v1/chat/completions"}, 0); !result.Aborted {
		t.Fatal(result)
	}
	s.SyncProviders([]ProviderEndpointConfig{{ID: "p", BaseURL: "https://api.openai.com/v1", Enabled: true, APIKey: "fixture-key"}})
	result := s.Fetch(ProviderFetchRequest{URL: "https://api.anthropic.com/v1/messages", ProviderID: "p", Headers: map[string]string{"x-api-key": providerKeyPlaceholder}})
	if result.OK || !strings.Contains(result.Error, "configured endpoint") {
		t.Fatal(result)
	}
	request, err := s.authorizeProviderRequest(ProviderFetchRequest{URL: "https://api.openai.com/v1/models?key=" + providerKeyPlaceholder, ProviderID: "p"})
	if err != nil || !strings.Contains(request.URL, "key=fixture-key") {
		t.Fatal("query authentication did not resolve")
	}
	message := providerFetchError(&url.Error{Op: "Get", URL: request.URL, Err: context.DeadlineExceeded})
	if strings.Contains(message, "fixture-key") || message != "Request timeout" {
		t.Fatal("credential leaked through transport error")
	}
}
