package rpc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Envelope is one request frame. Version and ID travel with every request;
// clients correlation is by ID and the server never reorders responses on a
// connection.
type Envelope struct {
	Version    int             `json:"v"`
	ID         string          `json:"id"`
	Token      string          `json:"token,omitempty"`
	Method     string          `json:"method"`
	DeadlineMS int64           `json:"deadlineMs,omitempty"`
	Params     json.RawMessage `json:"params,omitempty"`
}

// Response is one reply frame. Error carries a stable code so callers
// branch without parsing messages.
type Response struct {
	Version int             `json:"v"`
	ID      string          `json:"id"`
	OK      bool            `json:"ok"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ResponseError  `json:"error,omitempty"`
}

// ResponseError is a typed failure.
type ResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

func (e *ResponseError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

// Frame size and parse failure modes. Limits are byte-based so large
// Unicode payloads cannot smuggle past a line-count budget (T43).
const (
	DefaultMaxFrameBytes = 1 << 20 // 1 MiB per frame; handlers page large outputs themselves
)

var (
	// ErrFrameTooLarge reports a frame over the configured bound; the
	// connection is drained to the next newline and the request answered
	// with CODE-style BAD_REQUEST before continuing.
	ErrFrameTooLarge = errors.New("rpc: frame exceeds limit")
	// ErrFrameTruncated reports EOF inside a frame.
	ErrFrameTruncated = errors.New("rpc: frame truncated by EOF")
	// ErrFrameMalformed reports a frame that is not valid JSON.
	ErrFrameMalformed = errors.New("rpc: frame is not valid JSON")
	// ErrVersionUnsupported reports a frame speaking another protocol
	// version (T41: typed version error, never best-effort parsing).
	ErrVersionUnsupported = errors.New("rpc: protocol version unsupported")
	// ErrBadRequest covers remaining envelope-level failures (shape).
	ErrBadRequest = errors.New("rpc: bad request")
)

// ReadFrame reads one newline-terminated JSON frame with a hard byte bound.
// An oversized frame is drained through its terminator so the connection
// stays parseable, and the caller answers with a typed error.
func ReadFrame(reader *bufio.Reader, maxBytes int64) ([]byte, error) {
	var buffer bytes.Buffer
	for {
		chunk, err := reader.ReadSlice('\n')
		if int64(buffer.Len()+len(chunk)) > maxBytes {
			// Drain the remainder of the oversized line so the next frame
			// starts clean, then report.
			for err == nil && !bytes.HasSuffix(chunk, []byte{'\n'}) {
				chunk, err = reader.ReadSlice('\n')
			}
			return nil, ErrFrameTooLarge
		}
		buffer.Write(chunk)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if errors.Is(err, io.EOF) {
			if buffer.Len() == 0 {
				return nil, io.EOF
			}
			return nil, ErrFrameTruncated
		}
		if err != nil {
			return nil, err
		}
		line := bytes.TrimRight(buffer.Bytes(), "\r\n")
		if len(line) == 0 {
			// Heartbeat/blank line: keep reading.
			buffer.Reset()
			continue
		}
		return line, nil
	}
}

// DecodeEnvelope parses and validates one frame's envelope shape and
// protocol version.
func DecodeEnvelope(frame []byte) (*Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(frame, &envelope); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFrameMalformed, err)
	}
	if envelope.Version != ProtocolVersion {
		return nil, fmt.Errorf("%w: got v%d want v%d", ErrVersionUnsupported, envelope.Version, ProtocolVersion)
	}
	if envelope.ID == "" {
		return nil, fmt.Errorf("%w: missing id", ErrBadRequest)
	}
	if envelope.Method == "" {
		return nil, fmt.Errorf("%w: missing method", ErrBadRequest)
	}
	return &envelope, nil
}

// EncodeResponse writes one response frame followed by a newline. The only
// bytes ever written to a connection's writer are frames; diagnostics must
// go to a separate log sink so stdio clients never see interleaved logs.
func EncodeResponse(writer io.Writer, response *Response) error {
	raw, err := json.Marshal(response)
	if err != nil {
		return err
	}
	if _, err := writer.Write(append(raw, '\n')); err != nil {
		return err
	}
	return nil
}
