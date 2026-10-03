package terminaluse

import (
	"fmt"

	"github.com/binaricat/lemonssh/internal/platform/charset"
)

// Terminal input charset support.
//
// Legacy devices (network equipment, serial consoles) often speak GB18030.
// The renderer decodes output on the data plane with the same charset, so
// here we only need to encode the input side: keystrokes arrive as UTF-8 from
// the renderer and must be transcoded before reaching the PTY/telnet/serial
// stream, otherwise typed text shows up garbled on the device (the asymmetry
// the Electron shell fixed in its terminal input path). UTF-8 sessions skip
// the codec entirely.

// SetSessionEncoding pins the input charset for one session (native or
// renderer alias id). Supported values: utf-8 (default) and gb18030.
func (s *Service) SetSessionEncoding(sessionID, encoding string) error {
	term, ok := s.lookup(sessionID)
	if !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}
	normalized := charset.Normalize(encoding)
	if normalized == charset.Auto {
		normalized = charset.UTF8
	}
	term.encoding = normalized
	return nil
}

// SessionEncoding returns the pinned input charset for one session.
func (s *Service) SessionEncoding(sessionID string) string {
	term, ok := s.lookup(sessionID)
	if !ok {
		return charset.UTF8
	}
	if term.encoding == "" {
		return charset.UTF8
	}
	return term.encoding
}

// encodeInput transcodes renderer keystrokes (UTF-8) into the session's
// charset. ASCII bytes pass through untouched for every supported charset
// (they are ASCII supersets), so control sequences stay intact on the hot
// path.
func encodeInput(data []byte, encoding string) []byte {
	if charset.IsUTF8(encoding) {
		return data
	}
	if isASCIIBytes(data) {
		return data
	}
	return []byte(charset.Encode(string(data), encoding))
}

func isASCIIBytes(data []byte) bool {
	for _, b := range data {
		if b >= 0x80 {
			return false
		}
	}
	return true
}
