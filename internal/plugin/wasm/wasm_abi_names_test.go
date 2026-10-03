package wasm

import (
	"context"
	"testing"

	"github.com/binaricat/lemonssh/internal/plugin/permissions"
)

// newABIHostImportsModule is fixtureHostImportsModule with the current ABI
// names: imports resolve against the lemonssh host module and the exports use
// the lemonssh_* names, assembled via the same hand-rolled binary helpers.
func newABIHostImportsModule(t *testing.T) []byte {
	t.Helper()
	const (
		logMsg  = "lemonssh fixture log msg" // 24 bytes
		setting = "com.example.fixture.greeting"
		payload = `{"ok":true,"result":"done"}`
	)
	if len(logMsg) != 24 || len(setting) != 28 || len(payload) != 27 {
		t.Fatalf("fixture layout drifted: log=%d setting=%d payload=%d", len(logMsg), len(setting), len(payload))
	}
	imports := [][]byte{
		concat(wasmStr(HostModuleName), wasmStr(ImportHostLog), []byte{0x00, 0x00}),
		concat(wasmStr(HostModuleName), wasmStr(ImportHostSettingGet), []byte{0x00, 0x01}),
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
		{ExportDispatch, 0x00, 2},
		{ExportAlloc, 0x00, 3},
		{ExportFree, 0x00, 4},
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

// TestLegacyABIModuleInstantiatesAndDispatches proves pre-rename plugin
// binaries (netcatty module imports + netcatty_* exports) keep working after
// the ABI rename: the host registers the legacy module and the guest export
// resolution falls back to the legacy names.
func TestLegacyABIModuleInstantiatesAndDispatches(t *testing.T) {
	ctx := context.Background()
	runtime, broker := newFixtureRuntime(t)
	// fixtureABIModule exports the legacy netcatty_* names.
	if err := runtime.Instantiate(ctx, "legacy-plugin", fixtureABIModule(t, pongBody, allocBody, []byte(pongEnvelope))); err != nil {
		t.Fatalf("legacy instantiate: %v", err)
	}
	result, err := runtime.Dispatch(ctx, "legacy-plugin", "ping", `{"from":"host"}`)
	if err != nil {
		t.Fatalf("legacy dispatch: %v", err)
	}
	if !result.OK || string(result.Result) != `"pong"` {
		t.Fatalf("unexpected legacy result: %+v", result)
	}

	// The host-import fixture proves the legacy netcatty host module still
	// serves netcatty_host_log / netcatty_host_setting_get. Grant both so the
	// gated imports route instead of returning structured denials.
	if _, err := broker.Grant("legacy-imports", resourceLog, permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Grant("legacy-imports", resourceSettings, permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Instantiate(ctx, "legacy-imports", fixtureHostImportsModule(t)); err != nil {
		t.Fatalf("legacy imports instantiate: %v", err)
	}
	if _, err := runtime.Dispatch(ctx, "legacy-imports", "ping", ""); err != nil {
		t.Fatalf("legacy import dispatch: %v", err)
	}
	logs := runtime.RecentLogs("legacy-imports")
	if len(logs) != 1 || logs[0].Level != "info" {
		t.Fatalf("legacy host_log must route to the same broker-gated sink: %+v", logs)
	}
}

// TestNewABIModuleInstantiatesAndDispatches proves the current lemonssh ABI
// names work end to end: imports resolve against the lemonssh host module and
// the lemonssh_* exports drive dispatch.
func TestNewABIModuleInstantiatesAndDispatches(t *testing.T) {
	ctx := context.Background()
	runtime, broker := newFixtureRuntime(t)
	// Grant both resources so the gated imports route instead of denying.
	if _, err := broker.Grant("new-plugin", resourceLog, permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := broker.Grant("new-plugin", resourceSettings, permissions.LifetimeSession, 0); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Instantiate(ctx, "new-plugin", newABIHostImportsModule(t)); err != nil {
		t.Fatalf("new instantiate: %v", err)
	}
	result, err := runtime.Dispatch(ctx, "new-plugin", "ping", "")
	if err != nil {
		t.Fatalf("new dispatch: %v", err)
	}
	if !result.OK || string(result.Result) != `"done"` {
		t.Fatalf("unexpected new result: %+v", result)
	}
	logs := runtime.RecentLogs("new-plugin")
	if len(logs) != 1 || logs[0].Level != "info" || logs[0].Message != "lemonssh fixture log msg" {
		t.Fatalf("lemonssh_host_log must route through the broker: %+v", logs)
	}
}
