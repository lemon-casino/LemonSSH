package monitoring

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

const MaxOutput = 4 * 1024 * 1024

// Channel is a dedicated SSH exec channel, never the interactive PTY.
type Channel interface {
	SetOutput(io.Writer, io.Writer)
	Run(string) error
	Close() error
}
type boundedBuffer struct {
	mu       sync.Mutex
	b        bytes.Buffer
	overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.b.Len()+len(p) > MaxOutput {
		b.overflow = true
		return 0, fmt.Errorf("monitoring output exceeds limit")
	}
	return b.b.Write(p)
}

// Keep unresponsive SSH channel-open requests bounded without closing a shared transport.
var execSlots = make(chan struct{}, 8)

func Execute(ctx context.Context, open func() (Channel, error), command string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if e := ctx.Err(); e != nil {
		return "", e
	}
	select {
	case execSlots <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		defer func() { <-execSlots }()
		ch, e := open()
		if e != nil {
			done <- result{err: e}
			return
		}
		defer ch.Close()
		if ctx.Err() != nil {
			done <- result{err: ctx.Err()}
			return
		}
		stopped := make(chan struct{})
		defer close(stopped)
		go func() {
			select {
			case <-ctx.Done():
				_ = ch.Close()
			case <-stopped:
			}
		}()
		var out, stderr boundedBuffer
		ch.SetOutput(&out, &stderr)
		e = ch.Run(command)
		if ctx.Err() != nil {
			e = ctx.Err()
		} else if out.overflow || stderr.overflow {
			e = fmt.Errorf("monitoring output exceeds limit")
		} else if e != nil {
			e = fmt.Errorf("monitoring command failed: %w: %s", e, stderr.b.String())
		}
		done <- result{out.b.String(), e}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-done:
		return r.text, r.err
	}
}

// ExecuteLocal uses a fresh noninteractive shell, never the local terminal PTY.
func ExecuteLocal(ctx context.Context, command string) (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("local monitoring is unsupported on %s; Linux is required", runtime.GOOS)
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.WaitDelay = 250 * time.Millisecond
	var out, stderr boundedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	e := cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if out.overflow || stderr.overflow {
		return "", fmt.Errorf("monitoring output exceeds limit")
	}
	if e != nil {
		return "", fmt.Errorf("monitoring command failed: %w: %s", e, stderr.b.String())
	}
	return out.b.String(), nil
}

const ProbeCommand = `export LC_ALL=C; uname -s; for tool in tmux docker; do if command -v "$tool" >/dev/null 2>&1; then printf '%s=1\n' "$tool"; else printf '%s=0\n' "$tool"; fi; done; if test -r /proc/stat -a -r /proc/meminfo; then printf 'proc=1\n'; else printf 'proc=0\n'; fi; if command -v ps >/dev/null 2>&1; then printf 'ps=1\n'; else printf 'ps=0\n'; fi`
const ProcessesCommand = `export LC_ALL=C; command -v ps >/dev/null 2>&1 || { printf 'ps unavailable' >&2; exit 127; }; ps -eo pid=,ppid=,user=,stat=,pcpu=,pmem=,rss=,vsz=,etime=,args= --sort=-pmem`
const TmuxCommand = "export LC_ALL=C; command -v tmux >/dev/null 2>&1 || { printf 'tmux unavailable' >&2; exit 127; }; tmux list-sessions -F '#{session_name}\t#{session_windows}\t#{session_attached}\t#{session_created}'"
const ContainersCommand = `export LC_ALL=C; command -v docker >/dev/null 2>&1 || { printf 'docker unavailable' >&2; exit 127; }; docker ps -a --no-trunc --format '{{json .}}'`
const ImagesCommand = `export LC_ALL=C; command -v docker >/dev/null 2>&1 || { printf 'docker unavailable' >&2; exit 127; }; docker images --no-trunc --digests --format '{{json .}}'`
const DockerStatsCommand = `export LC_ALL=C; command -v docker >/dev/null 2>&1 || { printf 'docker unavailable' >&2; exit 127; }; docker stats --no-stream --no-trunc --format '{{json .}}'`
const StatsCommand = `export LC_ALL=C; test "$(uname -s)" = Linux || { printf 'server stats require Linux /proc' >&2; exit 1; }; set -e
printf '===cpu1===\n'; cat /proc/stat
printf '===net1===\n'; cat /proc/net/dev
printf '===time1===\n'; cat /proc/uptime
sleep 0.2
printf '===cpu2===\n'; cat /proc/stat
printf '===net2===\n'; cat /proc/net/dev
printf '===time2===\n'; cat /proc/uptime
printf '===mem===\n'; cat /proc/meminfo
printf '===hostname===\n'; hostname
printf '===kernel===\n'; uname -r
printf '===os===\n'; uname -s
printf '===load===\n'; cat /proc/loadavg
if command -v df >/dev/null 2>&1; then printf '===df===\n'; df -kPT; fi
if command -v ps >/dev/null 2>&1; then printf '===ps===\n'; ps -eo pid=,ppid=,user=,stat=,pcpu=,pmem=,rss=,vsz=,etime=,args= --sort=-pmem; fi`
