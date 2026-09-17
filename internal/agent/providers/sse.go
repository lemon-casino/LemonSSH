package providers

import (
	"bytes"
	"strings"
)

// SSEParser decodes a text/event-stream byte stream into complete events.
// Feeding arbitrary byte chunks — including splits inside multi-byte UTF-8
// sequences — is safe: bytes are buffered and only complete lines are
// turned into strings (T17).
type SSEParser struct {
	buffer []byte
	frame  sseFrame
	sawBOM bool
}

// SSEEvent is one decoded server event: the joined data payload plus the
// optional event name and id fields.
type SSEEvent struct {
	Event string
	ID    string
	Data  string
}

type sseFrame struct {
	event string
	id    string
	data  []string
}

func (p *SSEParser) resetFrame() { p.frame = sseFrame{} }

// Feed consumes one chunk and returns every event completed by it. A
// trailing partial line stays buffered until more bytes arrive. Blank
// lines terminate frames; a frame with no data is a keep-alive and is
// dropped.
func (p *SSEParser) Feed(chunk []byte) []SSEEvent {
	if !p.sawBOM {
		p.sawBOM = true
		if bytes.HasPrefix(chunk, []byte{0xEF, 0xBB, 0xBF}) {
			chunk = chunk[3:]
		}
	}
	p.buffer = append(p.buffer, chunk...)

	var events []SSEEvent
	for {
		line, rest, ok := cutLine(p.buffer)
		if !ok {
			break
		}
		p.buffer = rest

		if len(line) == 0 {
			if len(p.frame.data) > 0 {
				events = append(events, SSEEvent{
					Event: p.frame.event,
					ID:    p.frame.id,
					Data:  strings.Join(p.frame.data, "\n"),
				})
			}
			p.resetFrame()
			continue
		}

		field, value, _ := bytes.Cut(line, []byte{':'})
		value = bytes.TrimPrefix(value, []byte(" "))
		switch string(field) {
		case "data":
			p.frame.data = append(p.frame.data, string(value))
		case "event":
			p.frame.event = string(value)
		case "id":
			p.frame.id = string(value)
		default:
			// ':' comments and unknown fields are ignored per spec.
		}
	}
	return events
}

// cutLine splits buffer at the first LF, tolerating CRLF. ok is false when
// no complete line is buffered yet.
func cutLine(buffer []byte) (line, rest []byte, ok bool) {
	index := bytes.IndexByte(buffer, '\n')
	if index < 0 {
		return nil, nil, false
	}
	line = buffer[:index]
	line = bytes.TrimSuffix(line, []byte("\r"))
	return line, buffer[index+1:], true
}
