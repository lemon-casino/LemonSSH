// Package charset owns filename/output transcoding between UTF-8 (the
// application's canonical text form) and legacy remote charsets such as
// GB18030. The Wails JSON channel replaces invalid UTF-8 bytes with U+FFFD
// (documented encoding/json behavior), so legacy byte sequences must be
// converted to valid UTF-8 before crossing the bridge — this package is the
// single source of truth for that conversion, shared by the SFTP filename
// path and the terminal input path.
package charset

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// Canonical encoding identifiers. Auto is only meaningful for SFTP listings
// where the concrete charset is probed from directory entry names.
const (
	UTF8    = "utf-8"
	GB18030 = "gb18030"
	Auto    = "auto"
)

// gbTokens mirrors the user-facing aliases the frontend ships for the GB
// family; GB18030 is the superset we standardize on.
var gbTokens = map[string]struct{}{
	"gb18030": {},
	"gbk":     {},
	"gb2312":  {},
	"cp936":   {},
	"ms936":   {},
}

// Normalize maps user-facing charset labels onto a canonical identifier:
// UTF8, GB18030 or Auto. Unknown labels fall back to UTF-8 so an unexpected
// value can never silently corrupt traffic; "auto" is preserved for the
// SFTP probe path.
func Normalize(encoding string) string {
	raw := strings.ToLower(strings.TrimSpace(encoding))
	if raw == "" {
		return UTF8
	}
	if raw == "utf8" {
		return UTF8
	}
	if raw == Auto {
		return Auto
	}
	token := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, raw)
	if _, ok := gbTokens[token]; ok {
		return GB18030
	}
	return UTF8
}

// IsUTF8 reports whether the normalized encoding is UTF-8. Callers use this
// to skip the codec round-trip on hot paths: for UTF-8 the Go string already
// carries the exact remote bytes.
func IsUTF8(encoding string) bool {
	return Normalize(encoding) == UTF8
}

// IsValidUTF8 reports whether raw is well-formed UTF-8. The SFTP auto probe
// uses it on directory entry names: any invalid sequence pins the session to
// GB18030.
func IsValidUTF8(raw string) bool {
	return utf8.ValidString(raw)
}

// Decode converts raw remote bytes to a UTF-8 string. Auto resolves like
// UTF8 when the bytes already are valid UTF-8 and falls back to GB18030
// otherwise. Invalid target sequences decode to U+FFFD (the x/text default),
// which keeps the result JSON-safe for the Wails channel.
func Decode(raw string, encoding string) string {
	if raw == "" {
		return raw
	}
	switch Normalize(encoding) {
	case Auto:
		if utf8.ValidString(raw) {
			return raw
		}
		return decodeGB18030(raw)
	case GB18030:
		return decodeGB18030(raw)
	default:
		if utf8.ValidString(raw) {
			return raw
		}
		// UTF-8 asked-for but bytes are broken: sanitize so the Wails JSON
		// channel cannot double-replace and log pipelines stay lossless.
		return strings.ToValidUTF8(raw, "\uFFFD")
	}
}

// Encode converts a UTF-8 string to remote bytes for the encoding, returned
// as a Go string carrying the raw bytes. Auto encodes ASCII input unchanged
// and assumes GB18030 for non-ASCII text (the auto probe has already pinned
// the session by the time encode paths run). Runes the target charset cannot
// represent fall back to their UTF-8 bytes rather than failing the whole
// operation.
func Encode(text string, encoding string) string {
	if text == "" {
		return text
	}
	normalized := Normalize(encoding)
	if normalized == UTF8 {
		return text
	}
	if normalized == Auto && isASCII(text) {
		return text
	}
	encoded, _, err := transform.String(simplifiedchinese.GB18030.NewEncoder(), text)
	if err != nil {
		// Unmappable runes: keep the original UTF-8 text so the operation
		// still addresses a well-formed path.
		return text
	}
	return encoded
}

func isASCII(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func decodeGB18030(raw string) string {
	decoded, _, err := transform.String(simplifiedchinese.GB18030.NewDecoder(), raw)
	if err != nil {
		// Decoders replace invalid bytes instead of erroring; a failure here
		// means the transform itself was misused. Sanitize as a last resort.
		return strings.ToValidUTF8(raw, "\uFFFD")
	}
	return decoded
}

// DetectListingEncoding implements the SFTP auto probe over raw directory
// entry names: any name that is not valid UTF-8 proves a legacy charset
// (GB18030); otherwise the previous resolution cannot be disproven and the
// caller preserves it.
func DetectListingEncoding(names []string) (string, bool) {
	for _, name := range names {
		if !utf8.ValidString(name) {
			return GB18030, true
		}
	}
	return "", false
}
