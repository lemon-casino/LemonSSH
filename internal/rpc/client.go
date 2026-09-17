package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// UnavailableError reports the typed "app not running / not reachable"
// condition first-party launchers surface as a clean message instead of a
// stack trace (W07: typed unavailable).
type UnavailableError struct {
	Message string
}

func (e *UnavailableError) Error() string { return e.Message }

// RPCError is a failure reported by the host as a typed response code.
type RPCError struct {
	Code    string
	Message string
}

func (e *RPCError) Error() string { return e.Code + ": " + e.Message }

// DialTimeout bounds the TCP connect; CallTimeout applies when the caller
// passes a context without a deadline.
const (
	DialTimeout = 3 * time.Second
	CallTimeout = 30 * time.Second
)

// Client is one authenticated connection to the host RPC. Calls are
// serialized; a CLI uses one call per process but tests and launchers may
// reuse the connection.
type Client struct {
	conn      net.Conn
	discovery *Discovery

	mu     sync.Mutex
	nextID int
}

// Dial connects to 127.0.0.1:<discovery port> using the discovery file.
// Missing file, bad payload or refused connection become UnavailableError.
func Dial(discoveryPath string) (*Client, error) {
	discovery, err := LoadDiscovery(discoveryPath)
	if err != nil {
		message := fmt.Sprintf("Netcatty is not running or discovery file is missing at %s. Start Netcatty first.", discoveryPath)
		if errors.Is(err, ErrDiscoveryInvalid) {
			message = fmt.Sprintf("Netcatty discovery file at %s is invalid.", discoveryPath)
		}
		return nil, &UnavailableError{Message: message}
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", discovery.Port), DialTimeout)
	if err != nil {
		return nil, &UnavailableError{Message: fmt.Sprintf("Netcatty is not accepting tool connections on port %d: %v", discovery.Port, err)}
	}
	return &Client{conn: conn, discovery: discovery}, nil
}

// Close tears down the connection.
func (c *Client) Close() error { return c.conn.Close() }

// Call executes one request/response round trip. Errors are *RPCError for
// host-reported failures; transport failures surface raw.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, CallTimeout)
		defer cancel()
	}

	rawParams, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("rpc: params encoding failed: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	id := fmt.Sprintf("cli-%d", c.nextID)

	envelope := Envelope{
		Version: ProtocolVersion,
		ID:      id,
		Token:   c.discovery.Token,
		Method:  method,
		Params:  rawParams,
	}
	rawEnvelope, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	if _, err := c.conn.Write(append(rawEnvelope, '\n')); err != nil {
		return nil, fmt.Errorf("rpc: request write failed: %w", err)
	}

	responseCh := make(chan struct {
		frame []byte
		err   error
	}, 1)
	go func() {
		reader := bufio.NewReader(c.conn)
		frame, err := ReadFrame(reader, DefaultMaxFrameBytes)
		responseCh <- struct {
			frame []byte
			err   error
		}{frame, err}
	}()

	select {
	case <-ctx.Done():
		return nil, &RPCError{Code: CodeDeadline, Message: "call deadline exceeded while waiting for the host"}
	case result := <-responseCh:
		if result.err != nil {
			return nil, fmt.Errorf("rpc: response read failed: %w", result.err)
		}
		var response Response
		if err := json.Unmarshal(result.frame, &response); err != nil {
			return nil, fmt.Errorf("rpc: response is not valid JSON: %w", err)
		}
		if response.ID != id {
			return nil, fmt.Errorf("rpc: response id %q does not match request %q", response.ID, id)
		}
		if response.Error != nil {
			return nil, &RPCError{Code: response.Error.Code, Message: response.Error.Message}
		}
		return response.Result, nil
	}
}
