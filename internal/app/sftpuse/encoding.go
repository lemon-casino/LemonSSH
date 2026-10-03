package sftpuse

import (
	"github.com/binaricat/lemonssh/internal/platform/charset"
)

// Filename encoding support.
//
// Remote hosts do not all serve UTF-8 filenames; the renderer lets each SFTP
// pane pick auto / UTF-8 / GB18030. The Wails JSON channel replaces invalid
// UTF-8 bytes with U+FFFD (documented encoding/json behavior), so legacy names
// must be transcoded inside this package before entries cross the bridge, and
// renderer-provided paths must be encoded back to the remote charset on the
// way out. This mirrors the Electron bridge's sftpEncodingState design:
// explicit selections pin the session immediately, "auto" resolves to UTF-8
// until a directory listing proves a legacy charset.

// resolveEncoding maps the per-request filename encoding onto the concrete
// charset for this session. Explicit requests pin the session; auto and empty
// requests resolve to the pinned value (UTF-8 until a listing probes
// otherwise), matching the Electron bridge where an omitted encoding meant
// "auto".
func (s *Service) resolveEncoding(sessionID, requested string) string {
	normalized := charset.Normalize(requested)
	s.mu.Lock()
	defer s.mu.Unlock()
	if requested != "" && normalized != charset.Auto {
		s.encodings[sessionID] = normalized
		return normalized
	}
	if resolved, ok := s.encodings[sessionID]; ok {
		return resolved
	}
	return charset.UTF8
}

// pinEncoding stores an auto-probe result so subsequent operations reuse it.
func (s *Service) pinEncoding(sessionID, resolved string) {
	normalized := charset.Normalize(resolved)
	if normalized == charset.Auto {
		normalized = charset.UTF8
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.encodings[sessionID] = normalized
}

// ResolveEncoding exposes the per-session charset resolution for facades that
// run raw client operations outside the use-case methods (e.g. the chunked
// transfer scheduler and same-host directory copy).
func (s *Service) ResolveEncoding(sessionID, requested string) string {
	return s.resolveEncoding(sessionID, requested)
}

// resolvedEncoding returns the session's pinned charset without pinning side
// effects (used for read-only decoding, e.g. HomeDir).
func (s *Service) resolvedEncoding(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if resolved, ok := s.encodings[sessionID]; ok {
		return resolved
	}
	return charset.UTF8
}

// encodePath converts a renderer-provided UTF-8 path to the remote charset.
func (s *Service) encodePath(sessionID, requested, target string) string {
	return charset.Encode(target, s.resolveEncoding(sessionID, requested))
}

// probeAndDecodeNames applies the auto probe to raw directory entry names and
// returns them decoded to UTF-8. Each name is decoded independently (valid
// UTF-8 passes through, legacy bytes decode as GB18030) so a mixed listing
// never corrupts its well-formed names. When the probe proves a legacy
// charset the session is pinned so path arguments sent back by the renderer
// are re-encoded to the same bytes.
func (s *Service) probeAndDecodeNames(sessionID string, names []string) []string {
	if detected, ok := charset.DetectListingEncoding(names); ok {
		s.pinEncoding(sessionID, detected)
	}
	decoded := make([]string, len(names))
	for i, name := range names {
		decoded[i] = charset.Decode(name, charset.Auto)
	}
	return decoded
}
