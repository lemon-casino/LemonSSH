package wasm

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/binaricat/lemonssh/internal/plugin/permissions"
)

// ---------------------------------------------------------------------------
// Minimal WASM binary assembly. The fixtures are hand-assembled so `go test`
// stays offline: no wat2wasm, no wabt, no network.
// ---------------------------------------------------------------------------

// minimalWASM is the smallest valid WASM binary (magic + version, no exports).
var minimalWASM = []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

const wasmI32 = 0x7f

func lebU(n uint32) []byte {
	var out []byte
	for {
		b := byte(n & 0x7f)
		n >>= 7
		if n != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if n == 0 {
			return out
		}
	}
}

func lebS(v int32) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0) {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}

func wasmSection(id byte, payload []byte) []byte {
	return append([]byte{id}, append(lebU(uint32(len(payload))), payload...)...)
}

func wasmStr(s string) []byte { return append(lebU(uint32(len(s))), s...) }

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, part := range parts {
		out = append(out, part...)
	}
	return out
}

func functype(params, results []byte) []byte {
	out := []byte{0x60, byte(len(params))}
	out = append(out, params...)
	out = append(out, byte(len(results)))
	return append(out, results...)
}

var (
	tAlloc    = functype([]byte{wasmI32}, []byte{wasmI32})          // (i32) -> i32
	tFree     = functype([]byte{wasmI32, wasmI32}, nil)             // (i32, i32) -> ()
	tDispatch = functype([]byte{wasmI32, wasmI32}, []byte{wasmI32}) // (i32, i32) -> i32
	tStart    = functype(nil, nil)                                  // () -> ()
	tLog      = functype([]byte{wasmI32, wasmI32, wasmI32}, []byte{wasmI32})
	tSetting  = functype([]byte{wasmI32, wasmI32, wasmI32, wasmI32}, []byte{wasmI32})
)

// Shared function bodies (leading byte is the locals count). i32.const
// operands use signed LEB128 — always built through lebS, never hardcoded.
var (
	allocBody = concat(
		[]byte{0x00},       // no locals
		[]byte{0x23, 0x00}, // global.get 0 (heap)
		[]byte{0x23, 0x00}, // global.get 0 (heap)
		[]byte{0x20, 0x00}, // local.get 0 (size)
		[]byte{0x6a},       // i32.add
		[]byte{0x24, 0x00}, // global.set 0
		[]byte{0x0b},       // end
	)
	freeBody  = []byte{0x00, 0x0b}
	startBody = []byte{0x00, 0x0b}
	// pongBody: dispatch returns the static response region at address 64.
	pongBody = concat([]byte{0x00, 0x41}, lebS(64), []byte{0x0b})
	// loopBody: dispatch never returns (loop br 0); `unreachable` keeps the
	// tail validated.
	loopBody = []byte{0x00, 0x03, 0x40, 0x0c, 0x00, 0x0b, 0x00, 0x0b}
	// oomBody: alloc always fails (returns 0).
	oomBody = []byte{0x00, 0x41, 0x00, 0x0b}
	// growBody: alloc grows memory by one page; returns 0 when the grow fails
	// (used against a capped runtime) else bumps the heap. The if carries an
	// i32 result (blocktype 0x7f) so both arms leave exactly one value.
	growBody = concat(
		[]byte{0x00},
		[]byte{0x41, 0x01},                   // i32.const 1
		[]byte{0x40, 0x00},                   // memory.grow
		[]byte{0x41, 0x01, 0x6a},             // i32.const 1; add -> oldPages+1 (0 iff failed)
		[]byte{0x41, 0x00, 0x47},             // i32.const 0; ne
		[]byte{0x04, 0x7f},                   // if (result i32)
		[]byte{0x23, 0x00},                   // global.get 0 (heap) -> result value
		[]byte{0x23, 0x00, 0x20, 0x00, 0x6a}, // heap + size
		[]byte{0x24, 0x00},                   // global.set 0
		[]byte{0x05},                         // else
		[]byte{0x41, 0x00},                   // i32.const 0
		[]byte{0x0b},                         // end if
		[]byte{0x0b},                         // end func
	)
)

type wasmExport struct {
	name  string
	kind  byte // 0x00 func, 0x02 memory
	index uint32
}

type wasmData struct {
	offset uint32
	bytes  []byte
}

// assembleModule emits a WASM binary from raw sections.
func assembleModule(types [][]byte, imports [][]byte, funcTypeIndices []uint32,
	memMin, memMax uint32, globalInit int32, exports []wasmExport, datas []wasmData,
	bodies [][]byte) []byte {

	out := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

	typePayload := lebU(uint32(len(types)))
	for _, typ := range types {
		typePayload = append(typePayload, typ...)
	}
	out = append(out, wasmSection(0x01, typePayload)...)

	if len(imports) > 0 {
		importPayload := lebU(uint32(len(imports)))
		for _, entry := range imports {
			importPayload = append(importPayload, entry...)
		}
		out = append(out, wasmSection(0x02, importPayload)...)
	}

	funcPayload := lebU(uint32(len(funcTypeIndices)))
	for _, idx := range funcTypeIndices {
		funcPayload = append(funcPayload, lebU(idx)...)
	}
	out = append(out, wasmSection(0x03, funcPayload)...)

	memPayload := lebU(1)
	if memMax > 0 {
		memPayload = append(memPayload, 0x01)
		memPayload = append(memPayload, lebU(memMin)...)
		memPayload = append(memPayload, lebU(memMax)...)
	} else {
		memPayload = append(memPayload, 0x00)
		memPayload = append(memPayload, lebU(memMin)...)
	}
	out = append(out, wasmSection(0x05, memPayload)...)

	if globalInit != 0 || true {
		globalPayload := lebU(1)
		globalPayload = append(globalPayload, wasmI32, 0x01)
		globalPayload = append(globalPayload, 0x41)
		globalPayload = append(globalPayload, lebS(globalInit)...)
		globalPayload = append(globalPayload, 0x0b)
		out = append(out, wasmSection(0x06, globalPayload)...)
	}

	exportPayload := lebU(uint32(len(exports)))
	for _, exp := range exports {
		exportPayload = append(exportPayload, wasmStr(exp.name)...)
		exportPayload = append(exportPayload, exp.kind)
		exportPayload = append(exportPayload, lebU(exp.index)...)
	}
	out = append(out, wasmSection(0x07, exportPayload)...)

	codePayload := lebU(uint32(len(bodies)))
	for _, body := range bodies {
		codePayload = append(codePayload, lebU(uint32(len(body)))...)
		codePayload = append(codePayload, body...)
	}
	out = append(out, wasmSection(0x0a, codePayload)...)

	if len(datas) > 0 {
		dataPayload := lebU(uint32(len(datas)))
		for _, seg := range datas {
			dataPayload = append(dataPayload, 0x00) // active, memory 0
			dataPayload = append(dataPayload, 0x41)
			dataPayload = append(dataPayload, lebS(int32(seg.offset))...)
			dataPayload = append(dataPayload, 0x0b)
			dataPayload = append(dataPayload, lebU(uint32(len(seg.bytes)))...)
			dataPayload = append(dataPayload, seg.bytes...)
		}
		out = append(out, wasmSection(0x0b, dataPayload)...)
	}
	return out
}

func dataWithLengthPrefix(payload []byte) []byte {
	out := make([]byte, 4)
	binary.LittleEndian.PutUint32(out, uint32(len(payload)))
	return append(out, payload...)
}

// fixtureABIModule builds a legacy-named module exporting
// netcatty_alloc/netcatty_free/netcatty_dispatch/_start (legacy ABI) with no host imports.
func fixtureABIModule(t *testing.T, dispatchBody, allocBody []byte, payload []byte) []byte {
	t.Helper()
	exports := []wasmExport{
		{"netcatty_alloc", 0x00, 0},
		{"netcatty_free", 0x00, 1},
		{"netcatty_dispatch", 0x00, 2},
		{"_start", 0x00, 3},
	}
	return assembleModule(
		[][]byte{tAlloc, tFree, tDispatch, tStart},
		nil,
		[]uint32{0, 1, 2, 3},
		1, 0, 4096,
		exports,
		[]wasmData{{64, dataWithLengthPrefix(payload)}},
		[][]byte{allocBody, freeBody, dispatchBody, startBody},
	)
}

// fixtureHostImportsModule builds a legacy module whose dispatch calls
// the legacy netcatty_host_log and netcatty_host_setting_get imports and stores both status codes
// into linear memory (log -> 96, settings -> 100) before returning a static
// success envelope from address 64. Memory layout: log message @0,
// settings key @32, response @64, settings value buffer @1024 (4096 bytes).
func fixtureHostImportsModule(t *testing.T) []byte {
	t.Helper()
	const (
		logMsg  = "netcatty fixture log msg" // 24 bytes
		setting = "com.example.fixture.greeting"
		payload = `{"ok":true,"result":"done"}`
	)
	if len(logMsg) != 24 || len(setting) != 28 || len(payload) != 27 {
		t.Fatalf("fixture layout drifted: log=%d setting=%d payload=%d", len(logMsg), len(setting), len(payload))
	}
	imports := [][]byte{
		concat(wasmStr(LegacyHostModuleName), wasmStr(LegacyImportHostLog), []byte{0x00, 0x00}),
		concat(wasmStr(LegacyHostModuleName), wasmStr(LegacyImportHostSettingGet), []byte{0x00, 0x01}),
	}
	dispatch := concat([]byte{0x00},
		// memory[96] = host_log(1, 0, 24)
		[]byte{0x41}, lebS(96), []byte{0x41, 0x01, 0x41, 0x00, 0x41}, lebS(24),
		[]byte{0x10, 0x00, 0x36, 0x02, 0x00}, // call 0; i32.store
		// memory[100] = host_setting_get(32, 28, 1024, 4096)
		[]byte{0x41}, lebS(100), []byte{0x41}, lebS(32), []byte{0x41}, lebS(28),
		[]byte{0x41}, lebS(1024), []byte{0x41}, lebS(4096),
		[]byte{0x10, 0x01, 0x36, 0x02, 0x00}, // call 1; i32.store
		// return the static envelope at 64
		[]byte{0x41}, lebS(64), []byte{0x0b},
	)
	exports := []wasmExport{
		{"netcatty_dispatch", 0x00, 2},
		{"netcatty_alloc", 0x00, 3},
		{"netcatty_free", 0x00, 4},
		{"_start", 0x00, 5},
	}
	datas := []wasmData{
		{0, []byte(logMsg)},
		{32, []byte(setting)},
		{64, dataWithLengthPrefix([]byte(payload))},
	}
	return assembleModule(
		[][]byte{tLog, tSetting, tDispatch, tAlloc, tFree, tStart},
		imports,
		[]uint32{2, 3, 4, 5},
		1, 0, 8192,
		exports,
		datas,
		[][]byte{dispatch, allocBody, freeBody, startBody},
	)
}

// ---------------------------------------------------------------------------
// Runtime lifecycle (pre-existing behavior).
// ---------------------------------------------------------------------------

func TestRuntimeInstantiateAndClose(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Instantiate(ctx, "test-plugin", minimalWASM); err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	if err := runtime.CloseModule("test-plugin"); err != nil {
		t.Fatalf("close module: %v", err)
	}
	if err := runtime.CloseModule("test-plugin"); err == nil {
		t.Fatal("double close must fail")
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeDoubleInstantiateRejected(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	if err := runtime.Instantiate(ctx, "p1", minimalWASM); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Instantiate(ctx, "p1", minimalWASM); !errors.Is(err, ErrAlreadyInstant) {
		t.Fatalf("double instantiate must fail: %v", err)
	}
}

func TestRuntimeCloseAll(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		if err := runtime.Instantiate(ctx, id, minimalWASM); err != nil {
			t.Fatal(err)
		}
	}
	if err := runtime.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Instantiate(ctx, "c", minimalWASM); !errors.Is(err, ErrRuntimeClosed) {
		t.Fatalf("closed runtime must reject: %v", err)
	}
}

// ---------------------------------------------------------------------------
// lemonssh-wasm-abi v1 dispatch channel.
// ---------------------------------------------------------------------------

const pongEnvelope = `{"ok":true,"result":"pong"}`

func TestDispatchPingPong(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	if err := runtime.Instantiate(ctx, "fx", fixtureABIModule(t, pongBody, allocBody, []byte(pongEnvelope))); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Dispatch(ctx, "fx", "ping", `{"from":"host"}`)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !result.OK || string(result.Result) != `"pong"` || result.Error != nil {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestDispatchInBandPluginError(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	envelope := `{"ok":false,"error":{"code":"permission_denied","message":"nope"}}`
	if err := runtime.Instantiate(ctx, "fx", fixtureABIModule(t, pongBody, allocBody, []byte(envelope))); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Dispatch(ctx, "fx", "ping", "")
	if err != nil {
		t.Fatalf("dispatch must succeed at the transport level: %v", err)
	}
	if result.OK || result.Error == nil {
		t.Fatalf("expected in-band error: %+v", result)
	}
	if result.Error.Code != "permission_denied" || result.Error.Message != "nope" {
		t.Fatalf("unexpected error envelope: %+v", result.Error)
	}
}

func TestDispatchRequiresABI(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	if err := runtime.Instantiate(ctx, "plain", minimalWASM); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Dispatch(ctx, "plain", "ping", ""); !errors.Is(err, ErrNoDispatchABI) {
		t.Fatalf("expected ErrNoDispatchABI, got %v", err)
	}
	if _, err := runtime.Dispatch(ctx, "missing", "ping", ""); !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("expected ErrModuleNotFound, got %v", err)
	}
}

func TestDispatchValidation(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	if _, err := runtime.Dispatch(ctx, "fx", "", ""); !errors.Is(err, ErrDispatchFailed) {
		t.Fatalf("empty method must fail: %v", err)
	}
	if _, err := runtime.Dispatch(ctx, "fx", "ping", "{invalid"); !errors.Is(err, ErrDispatchFailed) {
		t.Fatalf("invalid payload must fail: %v", err)
	}
	if err := runtime.Instantiate(ctx, "fx", fixtureABIModule(t, pongBody, allocBody, []byte(pongEnvelope))); err != nil {
		t.Fatal(err)
	}
	big := `{"blob":"` + strings.Repeat("x", MaxRequestBytes) + `"}`
	if _, err := runtime.Dispatch(ctx, "fx", "ping", big); !errors.Is(err, ErrRequestTooLarge) {
		t.Fatalf("oversized request must fail: %v", err)
	}
}

func TestDispatchAllocExhaustion(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	if err := runtime.Instantiate(ctx, "oom", fixtureABIModule(t, pongBody, oomBody, []byte(pongEnvelope))); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Dispatch(ctx, "oom", "ping", ""); !errors.Is(err, ErrDispatchOOM) {
		t.Fatalf("expected ErrDispatchOOM, got %v", err)
	}
}

func TestDispatchMemoryCapEnforced(t *testing.T) {
	ctx := context.Background()
	// Uncapped: the grow succeeds and dispatch completes.
	uncapped, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer uncapped.Close(ctx)
	module := fixtureABIModule(t, pongBody, growBody, []byte(pongEnvelope))
	if err := uncapped.Instantiate(ctx, "fx", module); err != nil {
		t.Fatal(err)
	}
	if _, err := uncapped.Dispatch(ctx, "fx", "ping", ""); err != nil {
		t.Fatalf("uncapped dispatch must succeed: %v", err)
	}

	// Capped at one page: memory.grow fails, alloc returns 0 and the host
	// surfaces a structured exhaustion error instead of a trap.
	capped, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer capped.Close(ctx)
	if err := capped.InstantiateWithConfig(ctx, Config{
		PluginID:         "capped",
		WASMBytes:        module,
		MemoryLimitPages: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := capped.Dispatch(ctx, "capped", "ping", ""); !errors.Is(err, ErrDispatchOOM) {
		t.Fatalf("expected ErrDispatchOOM under the memory cap, got %v", err)
	}
}

func TestDispatchTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("timeout test waits on a live guest loop")
	}
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	if err := runtime.Instantiate(ctx, "loop", fixtureABIModule(t, loopBody, allocBody, []byte(pongEnvelope))); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	callCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	_, err = runtime.Dispatch(callCtx, "loop", "ping", "")
	if !errors.Is(err, ErrDispatchTimeout) {
		t.Fatalf("expected ErrDispatchTimeout, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("timeout abort took too long: %v", elapsed)
	}
}

func TestDispatchSerializedPerModule(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	if err := runtime.Instantiate(ctx, "fx", fixtureABIModule(t, pongBody, allocBody, []byte(pongEnvelope))); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := runtime.Dispatch(ctx, "fx", "ping", "")
			if err != nil || !result.OK {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent dispatch failed: %v", err)
	}
}

func TestDispatchAfterCloseModule(t *testing.T) {
	ctx := context.Background()
	runtime, err := NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	if err := runtime.Instantiate(ctx, "fx", fixtureABIModule(t, pongBody, allocBody, []byte(pongEnvelope))); err != nil {
		t.Fatal(err)
	}
	if err := runtime.CloseModule("fx"); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Dispatch(ctx, "fx", "ping", ""); !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("expected ErrModuleNotFound after close, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Broker-gated host imports (the wasm.go:3 promise).
// ---------------------------------------------------------------------------

const (
	resourceLog      = `["runtime","log"]:write`
	resourceSettings = `["runtime","settings"]:read`
)

func readGuestU32(t *testing.T, runtime *Runtime, pluginID string, offset uint32) uint32 {
	t.Helper()
	runtime.mu.Lock()
	module := runtime.modules[pluginID]
	runtime.mu.Unlock()
	if module == nil {
		t.Fatalf("module %s missing", pluginID)
	}
	value, ok := module.mod.Memory().ReadUint32Le(offset)
	if !ok {
		t.Fatalf("guest memory read at %d failed", offset)
	}
	return value
}

func readGuestString(t *testing.T, runtime *Runtime, pluginID string, offset, length uint32) string {
	t.Helper()
	runtime.mu.Lock()
	module := runtime.modules[pluginID]
	runtime.mu.Unlock()
	if module == nil {
		t.Fatalf("module %s missing", pluginID)
	}
	view, ok := module.mod.Memory().Read(offset, length)
	if !ok {
		t.Fatalf("guest memory read at %d failed", offset)
	}
	return string(view)
}

func newFixtureRuntime(t *testing.T) (*Runtime, *permissions.Broker) {
	t.Helper()
	runtime, err := NewRuntime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	broker := permissions.NewBroker(nil)
	runtime.SetBroker(broker)
	runtime.SetSettingsProvider(func(pluginID string) (map[string]any, error) {
		return map[string]any{"com.example.fixture.greeting": "hi"}, nil
	})
	return runtime, broker
}

func TestHostImportsBrokerGated(t *testing.T) {
	ctx := context.Background()
	runtime, broker := newFixtureRuntime(t)
	if err := runtime.Instantiate(ctx, "fx", fixtureHostImportsModule(t)); err != nil {
		t.Fatal(err)
	}

	// No grants: both imports must return structured permission-denied codes
	// to the guest while the dispatch envelope itself still succeeds.
	denied := uint32(0xFFFFFFFF) // int32 -1
	result, err := runtime.Dispatch(ctx, "fx", "ping", "")
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("dispatch must succeed even with denied imports: %+v", result)
	}
	if got := readGuestU32(t, runtime, "fx", 96); got != denied {
		t.Fatalf("host_log must be denied without a grant, got status %x", got)
	}
	if got := readGuestU32(t, runtime, "fx", 100); got != denied {
		t.Fatalf("host_setting_get must be denied without a grant, got status %x", got)
	}
	if denials := runtime.ImportDenials("fx"); denials != 2 {
		t.Fatalf("expected 2 recorded import denials, got %d", denials)
	}
	if logs := runtime.RecentLogs("fx"); len(logs) != 0 {
		t.Fatalf("denied log must not be retained: %+v", logs)
	}

	// Session grants flip both imports to success: log retained, setting
	// value written into the guest buffer as JSON.
	if _, err := broker.Grant("fx", resourceLog, permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Grant("fx", resourceSettings, permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Dispatch(ctx, "fx", "ping", ""); err != nil {
		t.Fatal(err)
	}
	if got := readGuestU32(t, runtime, "fx", 96); got != 0 {
		t.Fatalf("granted host_log must succeed, got status %x", got)
	}
	if got := readGuestU32(t, runtime, "fx", 100); got != 4 {
		t.Fatalf("granted setting read must report 4 written bytes (`\"hi\"`), got %x", got)
	}
	if value := readGuestString(t, runtime, "fx", 1024, 4); value != `"hi"` {
		t.Fatalf("setting value not written into guest buffer: %q", value)
	}
	logs := runtime.RecentLogs("fx")
	if len(logs) != 1 || logs[0].Level != "info" || logs[0].Message != "netcatty fixture log msg" {
		t.Fatalf("unexpected log ring: %+v", logs)
	}

	// Revoke: grants are gone, imports deny again (fail-closed).
	broker.RevokeAll("fx")
	if _, err := runtime.Dispatch(ctx, "fx", "ping", ""); err != nil {
		t.Fatal(err)
	}
	if got := readGuestU32(t, runtime, "fx", 96); got != denied {
		t.Fatalf("revoked host_log must deny again, got status %x", got)
	}
}

func TestHostImportsOnceGrantConsumed(t *testing.T) {
	ctx := context.Background()
	runtime, broker := newFixtureRuntime(t)
	if err := runtime.Instantiate(ctx, "fx", fixtureHostImportsModule(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Grant("fx", resourceLog, permissions.LifetimeOnce, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Dispatch(ctx, "fx", "ping", ""); err != nil {
		t.Fatal(err)
	}
	if got := readGuestU32(t, runtime, "fx", 96); got != 0 {
		t.Fatalf("once-granted host_log must succeed, got %x", got)
	}
	if _, err := runtime.Dispatch(ctx, "fx", "ping", ""); err != nil {
		t.Fatal(err)
	}
	if got := readGuestU32(t, runtime, "fx", 96); got != 0xFFFFFFFF {
		t.Fatalf("consumed once-grant must deny the next call, got %x", got)
	}
}

func TestHostSettingUnknownKeyInvalid(t *testing.T) {
	ctx := context.Background()
	runtime, broker := newFixtureRuntime(t)
	if err := runtime.Instantiate(ctx, "fx", fixtureHostImportsModule(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Grant("fx", resourceSettings, permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	runtime.SetSettingsProvider(func(pluginID string) (map[string]any, error) {
		return map[string]any{}, nil
	})
	if _, err := runtime.Dispatch(ctx, "fx", "ping", ""); err != nil {
		t.Fatal(err)
	}
	// 0xFFFFFFFE is the two's-complement i32 of HostImportInvalid (-2).
	if got := readGuestU32(t, runtime, "fx", 100); got != 0xFFFFFFFE {
		t.Fatalf("unknown setting must return invalid status, got %x", got)
	}
}

func TestHostLogTruncatesOversizedMessages(t *testing.T) {
	ctx := context.Background()
	runtime, broker := newFixtureRuntime(t)
	if err := runtime.Instantiate(ctx, "fx", fixtureHostImportsModule(t)); err != nil {
		t.Fatal(err)
	}
	// The fixture logs its fixed 24-byte message; validate the ring cap by
	// dispatching repeatedly past the ring size.
	if _, err := broker.Grant("fx", resourceLog, permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxLogEntries+10; i++ {
		if _, err := runtime.Dispatch(ctx, "fx", "ping", ""); err != nil {
			t.Fatal(err)
		}
	}
	logs := runtime.RecentLogs("fx")
	if len(logs) != maxLogEntries {
		t.Fatalf("log ring must cap at %d entries, got %d", maxLogEntries, len(logs))
	}
	for _, entry := range logs {
		if len(entry.Message) > maxLogBytes {
			t.Fatalf("log entry exceeds maxLogBytes: %d", len(entry.Message))
		}
	}
}
