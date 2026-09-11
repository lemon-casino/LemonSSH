// Package native implements the plugin native child-process runtime (P5-05,
// PLUG-03): hash-pinned, broker-authorized supervised processes with process-
// group/job-object containment, length-prefixed framed RPC, stdout flood
// bounds and quarantine on containment failure. Native plugins never execute
// inside the Go host process.
package native

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/binaricat/netcatty/internal/plugin/permissions"
)

var (
	ErrNotAuthorized      = errors.New("native: plugin not authorized for native execution")
	ErrAlreadyRunning     = errors.New("native: process already running")
	ErrNotRunning         = errors.New("native: process not running")
	ErrMaxNativeprocs     = errors.New("native: max concurrent native processes reached")
	ErrHashMismatch       = errors.New("native: binary hash mismatch")
	ErrBinaryMissing      = errors.New("native: binary missing")
	ErrBinaryUnsafe       = errors.New("native: binary path unsafe")
	ErrNodeForbidden      = errors.New("native: node runtime or shebang wrapper forbidden")
	ErrQuarantined        = errors.New("native: plugin is quarantined")
	ErrMalformedRPC       = errors.New("native: malformed RPC frame")
	ErrFlood              = errors.New("native: stdout flood")
	ErrContainmentFailure = errors.New("native: process-tree containment failure")
	ErrRPCTimeout         = errors.New("native: rpc timeout")
)

const (
	maxFrameBytes   = 1 << 20 // 1 MiB per RPC frame
	maxStdoutBytes  = 8 << 20 // 8 MiB stdout flood cap
	rpcTimeout      = 10 * time.Second
	stopGrace       = 2 * time.Second
	defaultMaxProcs = 8
)

// Spec describes one native child process to spawn.
type Spec struct {
	PluginID   string
	BinaryPath string
	SHA256     string
	Args       []string
	WorkDir    string
	Env        map[string]string
}

// NativeProcess is one live supervised child.
type NativeProcess struct {
	PluginID string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	cleanup  func() error
	mu       sync.Mutex
	nextID   uint64
	pending  map[uint64]chan rpcResponse
	flood    atomic.Int64
	exited   chan struct{}
	exitErr  error
}

type rpcRequest struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Runtime manages native child processes for all plugins.
type Runtime struct {
	mu         sync.Mutex
	max        int
	process    map[string]*NativeProcess
	quarantine map[string]string
	Authorize  func(pluginID string) error
	broker     *permissions.Broker
	resource   string
}

func NewRuntime(authorize func(pluginID string) error, max int) *Runtime {
	if max <= 0 {
		max = defaultMaxProcs
	}
	return &Runtime{
		max:        max,
		process:    make(map[string]*NativeProcess),
		quarantine: make(map[string]string),
		Authorize:  authorize,
		resource:   "companion.execute:write",
	}
}

// SetBroker attaches the permission broker used for spawn authorization.
func (r *Runtime) SetBroker(broker *permissions.Broker) { r.broker = broker }

// IsRunning reports whether a plugin has a live native process.
func (r *Runtime) IsRunning(pluginID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.process[pluginID]
	return ok
}

// IsQuarantined reports whether a plugin is blocked after a containment failure.
func (r *Runtime) IsQuarantined(pluginID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.quarantine[pluginID]
	return ok
}

// Start verifies the binary, authorizes the plugin and spawns a contained
// child. The binary must be a hash-pinned native executable: Node shebangs
// and wrappers are rejected before exec.
func (r *Runtime) Start(ctx context.Context, spec Spec) error {
	if spec.PluginID == "" {
		return errors.New("native: plugin id required")
	}
	if err := validateBinary(spec.BinaryPath, spec.SHA256); err != nil {
		return err
	}
	r.mu.Lock()
	if _, exists := r.process[spec.PluginID]; exists {
		r.mu.Unlock()
		return ErrAlreadyRunning
	}
	if reason, quarantined := r.quarantine[spec.PluginID]; quarantined {
		r.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrQuarantined, reason)
	}
	if len(r.process) >= r.max {
		r.mu.Unlock()
		return ErrMaxNativeprocs
	}
	if r.Authorize != nil {
		if err := r.Authorize(spec.PluginID); err != nil {
			r.mu.Unlock()
			return fmt.Errorf("%w: %v", ErrNotAuthorized, err)
		}
	}
	if r.broker != nil {
		if err := r.broker.Check(spec.PluginID, r.resource, "write"); err != nil {
			r.mu.Unlock()
			return fmt.Errorf("%w: %v", ErrNotAuthorized, err)
		}
	}
	r.mu.Unlock()

	process, err := spawnContained(ctx, spec)
	if err != nil {
		return err
	}
	r.mu.Lock()
	if _, exists := r.process[spec.PluginID]; exists {
		r.mu.Unlock()
		_ = killTree(process.cmd)
		return ErrAlreadyRunning
	}
	r.process[spec.PluginID] = process
	r.mu.Unlock()
	go r.watch(spec.PluginID, process)
	go r.readLoop(spec.PluginID, process)
	return nil
}

// Stop terminates a plugin's native process: graceful SIGTERM/CTRL_BREAK then
// forced tree kill if the process has not exited within the grace period.
func (r *Runtime) Stop(pluginID string) error {
	r.mu.Lock()
	process, ok := r.process[pluginID]
	if !ok {
		r.mu.Unlock()
		return ErrNotRunning
	}
	delete(r.process, pluginID)
	r.mu.Unlock()
	return stopProcess(process)
}

// StopAll terminates every native process (shutdown path).
func (r *Runtime) StopAll() {
	r.mu.Lock()
	processes := make([]*NativeProcess, 0, len(r.process))
	for id, process := range r.process {
		processes = append(processes, process)
		delete(r.process, id)
	}
	r.mu.Unlock()
	for _, process := range processes {
		_ = stopProcess(process)
	}
}

// Call sends one framed JSON-RPC request and waits for the matching response.
func (r *Runtime) Call(ctx context.Context, pluginID, method string, params json.RawMessage) (json.RawMessage, error) {
	r.mu.Lock()
	process, ok := r.process[pluginID]
	r.mu.Unlock()
	if !ok {
		return nil, ErrNotRunning
	}
	id := atomic.AddUint64(&process.nextID, 1)
	reply := make(chan rpcResponse, 1)
	process.mu.Lock()
	if process.pending == nil {
		process.pending = make(map[uint64]chan rpcResponse)
	}
	process.pending[id] = reply
	process.mu.Unlock()
	defer func() {
		process.mu.Lock()
		delete(process.pending, id)
		process.mu.Unlock()
	}()
	body, err := json.Marshal(rpcRequest{ID: id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	if err := writeFrame(process.stdin, body); err != nil {
		return nil, err
	}
	timeout := rpcTimeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(timeout):
		return nil, ErrRPCTimeout
	case response := <-reply:
		if response.Error != "" {
			return nil, fmt.Errorf("native rpc: %s", response.Error)
		}
		return response.Result, nil
	}
}

func (r *Runtime) watch(pluginID string, process *NativeProcess) {
	<-process.exited
	r.mu.Lock()
	current, ok := r.process[pluginID]
	if ok && current == process {
		delete(r.process, pluginID)
	}
	r.mu.Unlock()
}

func (r *Runtime) readLoop(pluginID string, process *NativeProcess) {
	reader := bufio.NewReader(process.stdout)
	for {
		frame, err := readFrame(reader)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, ErrFlood) && !errors.Is(err, ErrMalformedRPC) {
				r.quarantinePlugin(pluginID, err.Error())
			}
			if errors.Is(err, ErrFlood) || errors.Is(err, ErrMalformedRPC) {
				r.quarantinePlugin(pluginID, err.Error())
				_ = killTree(process.cmd)
			}
			return
		}
		process.flood.Add(int64(len(frame)))
		if process.flood.Load() > maxStdoutBytes {
			r.quarantinePlugin(pluginID, ErrFlood.Error())
			_ = killTree(process.cmd)
			return
		}
		var response rpcResponse
		if err := json.Unmarshal(frame, &response); err != nil {
			r.quarantinePlugin(pluginID, ErrMalformedRPC.Error())
			_ = killTree(process.cmd)
			return
		}
		process.mu.Lock()
		pending, ok := process.pending[response.ID]
		process.mu.Unlock()
		if !ok {
			continue
		}
		select {
		case pending <- response:
		default:
		}
	}
}

func (r *Runtime) quarantinePlugin(pluginID, reason string) {
	r.mu.Lock()
	r.quarantine[pluginID] = reason
	r.mu.Unlock()
}

func validateBinary(path, digest string) error {
	if path == "" {
		return ErrBinaryMissing
	}
	cleaned := filepath.Clean(path)
	if cleaned != path && filepath.Clean(cleaned) != cleaned {
		return fmt.Errorf("%w: unclean path", ErrBinaryUnsafe)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBinaryMissing, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: symlink rejected", ErrBinaryUnsafe)
	}
	if info.IsDir() {
		return fmt.Errorf("%w: directory", ErrBinaryUnsafe)
	}
	base := filepath.Base(path)
	lower := toLower(base)
	if lower == "node" || lower == "node.exe" || lower == "nodejs" {
		return ErrNodeForbidden
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBinaryMissing, err)
	}
	if len(data) >= 2 && data[0] == '#' && data[1] == '!' {
		head := string(data)
		if len(head) > 128 {
			head = head[:128]
		}
		if containsNodeShebang(head) {
			return ErrNodeForbidden
		}
	}
	if digest != "" {
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != digest {
			return ErrHashMismatch
		}
	}
	return nil
}

func containsNodeShebang(head string) bool {
	return containsFold(head, "node") || containsFold(head, "nodejs")
}

func containsFold(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(haystack) > 0 && (indexFold(haystack, needle) >= 0))
}

func indexFold(haystack, needle string) int {
	h, n := toLower(haystack), toLower(needle)
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

func toLower(value string) string {
	out := make([]byte, len(value))
	for i := 0; i < len(value); i++ {
		b := value[i]
		if b >= 'A' && b <= 'Z' {
			b += 'a' - 'A'
		}
		out[i] = b
	}
	return string(out)
}

func spawnContained(ctx context.Context, spec Spec) (*NativeProcess, error) {
	cmd := exec.CommandContext(ctx, spec.BinaryPath, spec.Args...)
	if spec.WorkDir != "" {
		cmd.Dir = spec.WorkDir
	}
	cmd.Env = minimalEnv(spec.Env)
	applyContainment(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}
	cleanup, err := assignJob(cmd)
	if err != nil {
		_ = killTree(cmd)
		_ = stdin.Close()
		return nil, err
	}
	process := &NativeProcess{
		PluginID: spec.PluginID,
		cmd:      cmd,
		stdin:    stdin,
		stdout:   stdout,
		cleanup:  cleanup,
		pending:  make(map[uint64]chan rpcResponse),
		exited:   make(chan struct{}),
	}
	go func() {
		process.exitErr = cmd.Wait()
		close(process.exited)
	}()
	return process, nil
}

func minimalEnv(extra map[string]string) []string {
	env := []string{
		"LANG=C",
		"LC_ALL=C",
		"PATH=",
	}
	if runtime.GOOS == "windows" {
		env = append(env, "SystemRoot="+os.Getenv("SystemRoot"))
	}
	keys := make([]string, 0, len(extra))
	for key := range extra {
		keys = append(keys, key)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	for _, key := range keys {
		env = append(env, key+"="+extra[key])
	}
	return env
}

func stopProcess(process *NativeProcess) error {
	_ = process.stdin.Close()
	if process.cleanup != nil {
		_ = process.cleanup()
		process.cleanup = nil
	}
	if process.cmd.Process == nil {
		return nil
	}
	_ = signalGraceful(process.cmd)
	select {
	case <-process.exited:
		return process.exitErr
	case <-time.After(stopGrace):
		if err := killTree(process.cmd); err != nil {
			return fmt.Errorf("%w: %v", ErrContainmentFailure, err)
		}
		select {
		case <-process.exited:
			return nil
		case <-time.After(stopGrace):
			return ErrContainmentFailure
		}
	}
}

func writeFrame(w io.Writer, body []byte) error {
	if len(body) > maxFrameBytes {
		return ErrMalformedRPC
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(body)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(header[:])
	if length == 0 || length > maxFrameBytes {
		return nil, ErrMalformedRPC
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, err
	}
	return body, nil
}

// HashFile returns the SHA-256 hex digest of a file. Tests and the Wails
// facade use it to pin a helper before Start.
func HashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
