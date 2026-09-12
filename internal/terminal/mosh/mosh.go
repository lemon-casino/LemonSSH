// Package mosh owns the Mosh/ET session bootstrap (P3-08.3, TERM-03.3): the
// remote handshake that starts mosh-server over SSH and scrapes the
// "MOSH CONNECT <port> <key>" line, plus the argument construction for the
// locally supervised mosh-client. The client process itself is supervised by
// internal/terminal/supervised; this package owns only the protocol wiring.
package mosh

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrNoConnectLine = errors.New("mosh: no MOSH CONNECT line")
	ErrBadPort       = errors.New("mosh: invalid port in MOSH CONNECT line")
	ErrBadKey        = errors.New("mosh: invalid key in MOSH CONNECT line")
)

// connectMarker is the announcement that carries the port and key. "MOSH IP"
// is optional metadata that may precede or follow it, so the parser keys on the
// CONNECT line and reads any IP separately.
const connectMarker = "MOSH CONNECT"

// protocolMarkers are the prefixes mosh-server may emit. A partial trailing
// marker must be retained by the caller so a match split across reads is not
// lost.
var protocolMarkers = []string{"MOSH CONNECT", "MOSH IP"}

// Connect describes a parsed mosh-server announcement.
type Connect struct {
	Port uint16
	Key  string
	// IP is set when the server emitted "MOSH IP <address>"; most servers omit
	// it and the client resolves the address from the SSH connection.
	IP string
}

// ParseConnect scans a buffer for the mosh-server announcement. It returns the
// parsed connection and the byte offset immediately after the matched line, so
// the caller can strip the internal protocol line from the user-visible
// stream. ok is false when the line is not yet complete, which means the caller
// should buffer and read more.
func ParseConnect(data []byte) (connect Connect, matchEnd int, ok bool, err error) {
	text := string(data)
	index := strings.Index(text, connectMarker)
	if index < 0 {
		return Connect{}, 0, false, nil
	}
	lineEnd := strings.IndexByte(text[index:], '\n')
	if lineEnd < 0 {
		// The line is not complete yet.
		return Connect{}, 0, false, nil
	}
	lineEnd += index
	line := text[index:lineEnd]
	end := lineEnd + 1

	rest := strings.TrimSpace(strings.TrimPrefix(line, connectMarker))
	// Strip surrounding terminal control sequences (mosh-server may emit the
	// line amid cursor-control output).
	rest = stripControl(rest)
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return Connect{}, 0, false, fmt.Errorf("%w: %q", ErrNoConnectLine, line)
	}
	portValue, parseErr := strconv.Atoi(fields[0])
	if parseErr != nil || portValue <= 0 || portValue > 65535 {
		return Connect{}, 0, false, fmt.Errorf("%w: %q", ErrBadPort, fields[0])
	}
	key := strings.TrimSpace(fields[1])
	if key == "" {
		return Connect{}, 0, false, ErrBadKey
	}
	return Connect{Port: uint16(portValue), Key: key, IP: extractIP(text[:end])}, end, true, nil
}

// TrailingMarkerPartial reports whether the buffer's tail could still grow into
// a protocol marker, so callers keep that tail instead of discarding it while
// draining a stream. The full marker counts as partial: it may still gain the
// port and key that follow it.
func TrailingMarkerPartial(text string) bool {
	for _, marker := range protocolMarkers {
		max := len(marker)
		if len(text) < max {
			max = len(text)
		}
		for length := max; length > 0; length-- {
			if strings.HasPrefix(marker, text[len(text)-length:]) {
				return true
			}
		}
	}
	return false
}

// extractIP pulls an address from a "MOSH IP <address>" fragment.
func extractIP(text string) string {
	index := strings.Index(text, "MOSH IP")
	if index < 0 {
		return ""
	}
	rest := strings.TrimSpace(text[index+len("MOSH IP"):])
	rest = stripControl(rest)
	if fields := strings.Fields(rest); len(fields) > 0 {
		return fields[0]
	}
	return ""
}

// stripControl removes ANSI escape sequences and carriage returns so a line
// like "MOSH CONNECT 60001 KEY==\x1b[?25h" still parses.
func stripControl(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		b := value[i]
		if b == 0x1b {
			// Skip CSI/OSC introducers and their parameter bytes.
			i++
			if i < len(value) && (value[i] == '[' || value[i] == ']') {
				i++
				for i < len(value) && !(value[i] >= 0x40 && value[i] <= 0x7e) {
					i++
				}
			}
			continue
		}
		if b == '\r' {
			continue
		}
		out.WriteByte(b)
	}
	return out.String()
}

// ServerCommand builds the remote command that starts mosh-server. A custom
// path is quoted so an operator-supplied path with spaces stays one argv word.
func ServerCommand(moshServerPath string) string {
	trimmed := strings.TrimSpace(moshServerPath)
	// -c 256 makes mosh-server set TERM=xterm-256color, matching a client that
	// reports 256 colours; the default (8) would force TERM=xterm.
	if trimmed == "" {
		return "mosh-server new -s -c 256"
	}
	return shellQuote(trimmed) + " new -s -c 256"
}

// shellQuote wraps a value in single quotes, escaping embedded quotes.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// ClientLaunch delegates roaming to the native clients. ET bootstraps its own
// persistent transport over SSH and must never receive MOSH credentials.
func ClientLaunch(kind, host, user string, sshPort uint16, connect Connect) ([]string, map[string]string, error) {
	switch kind {
	case "mosh":
		if connect.Port == 0 || connect.Key == "" {
			return nil, nil, ErrNoConnectLine
		}
		return ClientArgs(host, connect), map[string]string{"MOSH_KEY": connect.Key}, nil
	case "et":
		if sshPort == 0 {
			sshPort = 22
		}
		return []string{user + "@" + host, "--ssh-port", strconv.Itoa(int(sshPort))}, nil, nil
	default:
		return nil, nil, fmt.Errorf("unknown helper %q", kind)
	}
}

// ClientArgs builds the arguments for the locally supervised mosh-client. The
// port is the one mosh-server announced; the key travels via MOSH_KEY, not
// argv, so it does not leak into the process table.
func ClientArgs(host string, connect Connect) []string {
	target := host
	if connect.IP != "" {
		target = connect.IP
	}
	return []string{target, strconv.Itoa(int(connect.Port))}
}
