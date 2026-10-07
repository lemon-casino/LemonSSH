package ssh

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const x11AuthProtocol = "MIT-MAGIC-COOKIE-1"

type X11Display struct {
	Network, Address, Authority string
	Screen                      uint32
}

// ParseX11Display only permits local X servers, never a remote proxy target.
// DISPLAY numbers always map to 6000+n, including numbers greater than 99.
func ParseX11Display(spec, platform string) (X11Display, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return X11Display{}, errors.New("X11: DISPLAY is empty")
	}
	// XQuartz launchd DISPLAY names are local Unix sockets with a :screen suffix.
	if platform == "darwin" && strings.HasPrefix(spec, "/private/tmp/com.apple.launchd.") && strings.Contains(spec, "/org.xquartz:") {
		i := strings.LastIndex(spec, ":")
		n, err := strconv.ParseUint(spec[i+1:], 10, 32)
		if err == nil && filepath.Clean(spec) == spec {
			return X11Display{"unix", spec, spec, uint32(n)}, nil
		}
	}
	i := strings.LastIndex(spec, ":")
	if i < 0 {
		return X11Display{}, errors.New("X11: invalid DISPLAY")
	}
	host, number := spec[:i], spec[i+1:]
	parts := strings.Split(number, ".")
	if len(parts) > 2 || parts[0] == "" {
		return X11Display{}, errors.New("X11: invalid display number")
	}
	n, err := strconv.ParseUint(parts[0], 10, 16)
	if err != nil || n > 59535 {
		return X11Display{}, errors.New("X11: display number out of range")
	}
	var screen uint64
	if len(parts) == 2 {
		screen, err = strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return X11Display{}, errors.New("X11: invalid screen")
		}
	}
	d := X11Display{Authority: spec, Screen: uint32(screen)}
	if (host == "" || host == "unix") && platform != "windows" {
		d.Network = "unix"
		d.Address = fmt.Sprintf("/tmp/.X11-unix/X%d", n)
		return d, nil
	}
	if host == "" || host == "localhost" || host == "unix" {
		host = "127.0.0.1"
	}
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return X11Display{}, errors.New("X11: DISPLAY must name a local Unix socket or loopback address")
	}
	d.Network = "tcp"
	d.Address = net.JoinHostPort(host, strconv.Itoa(6000+int(n)))
	return d, nil
}

// ReadX11Cookie queries only the selected authority entry. Missing or ambiguous
// credentials are errors: forwarding must never fall back to unauthenticated IO.
func ReadX11Cookie(ctx context.Context, d X11Display) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	command := "xauth"
	if runtime.GOOS == "darwin" {
		if _, err := os.Stat("/opt/X11/bin/xauth"); err == nil {
			command = "/opt/X11/bin/xauth"
		}
	}
	cmd := exec.CommandContext(ctx, command, "list", d.Authority)
	hideXauthWindow(cmd)
	var output limitedXauthOutput
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("X11: cannot read local xauth cookie (install xauth and configure X server authentication): %w", err)
	}
	return parseXauthCookie(output.String())
}

type limitedXauthOutput struct{ bytes.Buffer }

func (b *limitedXauthOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 65536 {
		return 0, errors.New("X11: xauth output exceeds limit")
	}
	return b.Buffer.Write(p)
}
func parseXauthCookie(output string) ([]byte, error) {
	var cookie []byte
	for _, line := range strings.Split(output, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[1] != x11AuthProtocol {
			continue
		}
		c, err := hex.DecodeString(f[2])
		if err != nil || len(c) != 16 {
			return nil, errors.New("X11: invalid MIT cookie")
		}
		if cookie != nil && !bytes.Equal(cookie, c) {
			return nil, errors.New("X11: ambiguous authority cookies")
		}
		cookie = c
	}
	if cookie == nil {
		return nil, errors.New("X11: no MIT-MAGIC-COOKIE-1 authority for selected DISPLAY")
	}
	return cookie, nil
}

func readX11Setup(r io.Reader, fake, real []byte) ([]byte, error) {
	h := make([]byte, 12)
	if _, err := io.ReadFull(r, h); err != nil {
		return nil, err
	}
	var order binary.ByteOrder
	switch h[0] {
	case 'l':
		order = binary.LittleEndian
	case 'B':
		order = binary.BigEndian
	default:
		return nil, errors.New("X11: invalid byte order")
	}
	if order.Uint16(h[2:]) != 11 || order.Uint16(h[4:]) != 0 || order.Uint16(h[6:]) != 18 || order.Uint16(h[8:]) != 16 || len(fake) != 16 || len(real) != 16 {
		return nil, errors.New("X11: unsupported setup/authentication")
	}
	p := append(h, make([]byte, 36)...)
	if _, err := io.ReadFull(r, p[12:]); err != nil {
		return nil, err
	}
	if string(p[12:30]) != x11AuthProtocol || subtle.ConstantTimeCompare(p[32:48], fake) != 1 {
		return nil, errors.New("X11: rejected authentication cookie")
	}
	copy(p[32:48], real)
	return p, nil
}
