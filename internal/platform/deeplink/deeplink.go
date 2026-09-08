// Package deeplink owns deep-link intent parsing and the pre-ready intent
// queue (P4-04, SYS-03): netcatty://ssh/user@host:port?key=value URLs are
// parsed strictly, queued exactly once per launch, and delivered only after
// the app is unlocked/ready. Malformed intents fail closed.
package deeplink

import (
	"errors"
	"net/url"
	"strings"
	"sync"
)

var (
	ErrUnsupportedScheme = errors.New("deeplink: unsupported scheme")
	ErrMalformedIntent   = errors.New("deeplink: malformed intent")
)

// Action is the parsed intent.
type Action struct {
	Kind      string     `json:"kind"` // "ssh" | "telnet"
	Host      string     `json:"host"`
	Port      string     `json:"port"`
	Username  string     `json:"username,omitempty"`
	RawParams url.Values `json:"-"`
}

// Parse validates and parses one deep-link URL. Accepted forms:
//
//	ssh://[user@]host[:port]
//	telnet://host[:port]
//	netcatty://[ssh|telnet/][user@]host[:port]?params
func Parse(rawURL string) (*Action, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, ErrMalformedIntent
	}
	scheme, rest, found := strings.Cut(strings.TrimSpace(rawURL), "://")
	if !found {
		return nil, ErrMalformedIntent
	}
	scheme = strings.ToLower(scheme)
	kind := "ssh"
	switch scheme {
	case "netcatty":
	case "ssh":
	case "telnet":
		kind = "telnet"
	default:
		return nil, ErrUnsupportedScheme
	}
	// netcatty://ssh/user@host:port — an explicit kind path segment wins.
	if scheme == "netcatty" {
		if slash := strings.Index(rest, "/"); slash >= 0 {
			segment := strings.ToLower(rest[:slash])
			if segment == "ssh" || segment == "telnet" {
				kind = segment
			}
			rest = rest[slash+1:]
		}
	}
	authority := rest
	query := ""
	if q := strings.IndexAny(authority, "?#"); q >= 0 {
		query = authority[q+1:]
		authority = authority[:q]
	}
	var username string
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		username = authority[:at]
		if strings.ContainsAny(username, " \t\r\n") {
			return nil, ErrMalformedIntent
		}
		authority = authority[at+1:]
	}
	host, port := authority, ""
	if colon := strings.LastIndex(authority, ":"); colon >= 0 && !strings.Contains(authority[colon:], "]") {
		host = authority[:colon]
		port = authority[colon+1:]
	}
	if host == "" || strings.ContainsAny(host, " \t\r\n") {
		return nil, ErrMalformedIntent
	}
	action := &Action{Kind: kind, Host: host, Port: port}
	if username != "" {
		action.Username = username
	}
	if query != "" {
		values, err := url.ParseQuery(query)
		if err != nil {
			return nil, ErrMalformedIntent
		}
		for key := range values {
			switch strings.ToLower(key) {
			case "password", "secret", "token":
				return nil, ErrMalformedIntent // passwords never ride deep links
			}
		}
		action.RawParams = values
	}
	return action, nil
}

// Queue buffers intents that arrive before the app is ready. At-most-once
// delivery: duplicates are dropped, and Drain hands them to the app exactly
// once in arrival order.
type Queue struct {
	mu       sync.Mutex
	ready    bool
	handlers []func(*Action)
	pending  []*Action
	seen     map[string]bool
}

func NewQueue() *Queue {
	return &Queue{seen: make(map[string]bool)}
}

// Enqueue records an intent. Before Ready it is buffered; after, it is
// dispatched immediately.
func (q *Queue) Enqueue(rawURL string) error {
	action, err := Parse(rawURL)
	if err != nil {
		return err
	}
	dedupeKey := rawURL
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.seen[dedupeKey] {
		return nil // duplicate cold-start intents are dropped
	}
	q.seen[dedupeKey] = true
	if q.ready {
		for _, handler := range q.handlers {
			handler(action)
		}
		return nil
	}
	q.pending = append(q.pending, action)
	return nil
}

// Ready marks the app ready and flushes buffered intents in order.
func (q *Queue) Ready(handlers ...func(*Action)) {
	q.mu.Lock()
	q.ready = true
	q.handlers = append(q.handlers, handlers...)
	pending := q.pending
	q.pending = nil
	q.mu.Unlock()
	for _, handler := range handlers {
		for _, action := range pending {
			handler(action)
		}
	}
}

// Pending reports how many intents are buffered (pre-ready).
func (q *Queue) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}
