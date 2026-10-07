import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import * as ts from "typescript";

import {
  CancellationError,
  CancellationTokenSource,
  buildPluginViewDataRequest,
  buildWasmDispatchRequest,
  definePlugin,
  DisposableStore,
  parsePluginProviderInvokeRequest,
  parsePluginProviderListResult,
  parsePluginViewDataResult,
  parseWasmDispatchResponse,
  PROVIDER_INVOKE_DISPATCH_METHOD,
  PROVIDER_SESSION_EVENT_DISPATCH_METHOD,
  PROVIDERS_LIST_DISPATCH_METHOD,
  PluginError,
  PLUGIN_ERROR_WIRE_CODES,
  pluginErrorToRpcError,
  PLUGIN_VIEW_DATA_MAX_BINDINGS,
  throwIfCancellationRequested,
  WASM_ABI,
  WASM_HOST_IMPORT_STATUS,
} from "./index.ts";
import type { PluginSecretStore, SecretRef } from "./index.ts";

const testSecretRef: SecretRef = {
  kind: "secret",
  id: "secret-reference-1",
  key: "token",
};

const testSecretStore: PluginSecretStore = {
  async get() {
    return testSecretRef;
  },
  async set() {
    return testSecretRef;
  },
  async delete() {},
};

function assertSdkTypeChecks(source: string) {
  const sdkDirectory = dirname(fileURLToPath(import.meta.url));
  // TypeScript normalizes paths to forward slashes, so compare against a
  // forward-slash fixture path — a backslash join() result never matches on
  // Windows and the injected fixture would be reported as missing (TS6053).
  const fixturePath = join(sdkDirectory, "__provider-overload-fixture.ts").replaceAll("\\", "/");
  const compilerOptions: ts.CompilerOptions = {
    allowImportingTsExtensions: true,
    module: ts.ModuleKind.NodeNext,
    moduleResolution: ts.ModuleResolutionKind.NodeNext,
    noEmit: true,
    skipLibCheck: true,
    strict: true,
    target: ts.ScriptTarget.ES2022,
  };
  const host = ts.createCompilerHost(compilerOptions, true);
  const fileExists = host.fileExists.bind(host);
  const readCompilerFile = host.readFile.bind(host);
  host.fileExists = (fileName) => fileName === fixturePath || fileExists(fileName);
  host.readFile = (fileName) => fileName === fixturePath ? source : readCompilerFile(fileName);

  const program = ts.createProgram([fixturePath], compilerOptions, host);
  const diagnostics = ts.getPreEmitDiagnostics(program)
    .filter((diagnostic) => diagnostic.category === ts.DiagnosticCategory.Error);
  assert.deepEqual(
    diagnostics.map((diagnostic) => {
      const message = ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n");
      if (!diagnostic.file || diagnostic.start === undefined) {
        return `TS${diagnostic.code}: ${message}`;
      }
      const { line, character } = diagnostic.file.getLineAndCharacterOfPosition(diagnostic.start);
      return `${diagnostic.file.fileName}:${line + 1}:${character + 1} TS${diagnostic.code}: ${message}`;
    }),
    [],
  );
}

test("PluginError maps stable SDK codes to stable JSON-RPC wire errors", () => {
  const error = new PluginError("permission_denied", "Approval required", { scope: "terminal" });
  assert.deepEqual(pluginErrorToRpcError(error), {
    code: -32007,
    message: "Approval required",
    data: {
      pluginCode: "permission_denied",
      details: { scope: "terminal" },
    },
  });
  assert.equal(PLUGIN_ERROR_WIRE_CODES.cancelled, -32001);
  assert.equal(PLUGIN_ERROR_WIRE_CODES.internal, -32013);
  assert.equal(new Set(Object.values(PLUGIN_ERROR_WIRE_CODES)).size, 16);
  for (const code of Object.keys(PLUGIN_ERROR_WIRE_CODES)) {
    const mapped = pluginErrorToRpcError(new PluginError(
      code as keyof typeof PLUGIN_ERROR_WIRE_CODES,
      code,
    ));
    assert.equal(mapped.code, PLUGIN_ERROR_WIRE_CODES[code as keyof typeof PLUGIN_ERROR_WIRE_CODES]);
    assert.deepEqual(mapped.data, { pluginCode: code });
  }
});

test("PluginError wire mapping covers the exact contract schema enums", async () => {
  const schema = JSON.parse(await readFile(
    new URL("../../plugin-contract/schema/plugin-contract.schema.json", import.meta.url),
    "utf8",
  ));
  assert.deepEqual(
    Object.keys(PLUGIN_ERROR_WIRE_CODES).sort(),
    [...schema.$defs.PluginErrorName.enum].sort(),
  );
  assert.deepEqual(
    Object.values(PLUGIN_ERROR_WIRE_CODES).sort((left, right) => left - right),
    [...schema.$defs.PluginWireErrorCode.enum].sort((left, right) => left - right),
  );
});

test("definePlugin preserves the exact plugin object", () => {
  const plugin = definePlugin({ activate() {} });
  assert.equal(typeof plugin.activate, "function");
});

test("PluginSecretStore exposes opaque references instead of plaintext reads", async () => {
  assert.deepEqual(await testSecretStore.get("token"), testSecretRef);
  assert.deepEqual(await testSecretStore.set("token", "already-known-value"), testSecretRef);
  assert.equal("value" in testSecretRef, false);
  assert.equal(testSecretRef.key, "token");
});

test("terminal interceptor typing stays specialized while broad ProviderKind helpers remain compatible", async () => {
  const source = await readFile(new URL("./index.ts", import.meta.url), "utf8");
  assert.match(
    source,
    /kind: Exclude<\s*ProviderKind,\s*TerminalInterceptorKind \| OrdinaryTerminalProviderKind \| "connection" \| "authentication" \| "importer" \| "sync"\s*>,\s*handler: PluginProviderHandler/u,
  );
  assert.match(
    source,
    /type ProviderHandlerForKind<[\s\S]*K extends TerminalInterceptorKind[\s\S]*TerminalInterceptorHandler/u,
  );
  assert.match(
    source,
    /kind: K,\s*handler: ProviderHandlerForKind<NoInfer<K>, TPayload, TResult>/u,
  );
});

test("provider registrations infer typed connection importer and sync stream invocations", () => {
  assertSdkTypeChecks(`
    import { definePlugin } from "./index.ts";
    import type {
      ConnectionProviderHandler,
      ConnectionProviderResultByOperation,
      AuthenticationResult,
      ImporterKeyDraft,
      ImporterProviderHandler,
      SyncProviderHandler,
      SyncProviderResultByOperation,
    } from "./index.ts";

    const resizeAck: ConnectionProviderResultByOperation["resize"] = null;
    void resizeAck;
    // @ts-expect-error connection control operations acknowledge with JSON null, never object payloads.
    const invalidResizeAck: ConnectionProviderResultByOperation["resize"] = { ok: true };
    void invalidResizeAck;

    const invalidConnectionProvider: ConnectionProviderHandler = {
      validateConfiguration: () => ({ valid: true, issues: [] }),
      probe: () => ({ available: true }),
      open: () => ({ connectionId: "connection-1", status: "connected" }),
      // @ts-expect-error resize must return the resize control acknowledgement, not a probe result.
      resize: () => ({ available: true }),
      signal: () => null,
      reconnect: () => null,
      close: () => null,
      getStatus: () => ({ status: "connected" }),
    };
    void invalidConnectionProvider;

    const inlineImporterKey: ImporterKeyDraft = {
      label: "Inline key",
      type: "ED25519",
      privateKey: "private",
    };
    const fileImporterKey: ImporterKeyDraft = {
      label: "File key",
      type: "ED25519",
      filePath: "/keys/id_ed25519",
    };
    void inlineImporterKey;
    void fileImporterKey;
    // @ts-expect-error runtime validation requires exactly one key source.
    const ambiguousImporterKey: ImporterKeyDraft = {
      label: "Ambiguous key",
      type: "ED25519",
      privateKey: "private",
      filePath: "/keys/id_ed25519",
    };
    void ambiguousImporterKey;

    const invalidImporterProvider: ImporterProviderHandler = {
      // @ts-expect-error detect must return a detection result, not parse counters.
      detect: () => ({ parsed: 0, warnings: 0, errors: 0 }),
      parse: () => ({ parsed: 0, warnings: 0, errors: 0 }),
    };
    void invalidImporterProvider;

    const disconnectAck: SyncProviderResultByOperation["disconnect"] = null;
    void disconnectAck;
    // @ts-expect-error disconnect acknowledges with JSON null.
    const invalidDisconnectAck: SyncProviderResultByOperation["disconnect"] = { ok: true };
    void invalidDisconnectAck;

    const invalidSyncProvider: SyncProviderHandler = {
      connect: () => ({ account: { id: "a" } }),
      disconnect: () => null,
      getAccount: () => ({ account: null }),
      getCapabilities: () => ({ revisions: true, conditionalWrites: true, atomicReplacement: true }),
      // @ts-expect-error readObject must return a SyncReadObjectResult, not write result.
      readObject: () => ({ created: true }),
      writeObject: () => ({ created: true }),
      deleteObject: () => ({ deleted: true }),
    };
    void invalidSyncProvider;

    // @ts-expect-error challenge results must include the exact challenge payload.
    const incompleteAuthenticationResult: AuthenticationResult = { status: "challenge" };
    void incompleteAuthenticationResult;

    definePlugin({
      activate(context) {
        context.providers.register("com.example.connection", "connection", {
          async open(invocation) {
            const input = await invocation.input;
            const chunk: Uint8Array | null = await input.read();
            if (chunk) {
              await invocation.output.write(chunk);
            }
            await invocation.output.end();
            return { connectionId: "connection-1", status: "connected" };
          },
          validateConfiguration(invocation) {
            const configuration = invocation.payload.configuration;
            void configuration;
            return { valid: true, issues: [] };
          },
          probe() {
            return { available: true };
          },
          resize() {
            return null;
          },
          signal() {
            return null;
          },
          reconnect() {
            return null;
          },
          close() {
            return null;
          },
          getStatus() {
            return {
              status: "connected",
              diagnostics: [{ severity: "warning", message: "using fallback host key algorithm" }],
            };
          },
        });

        // @ts-expect-error connection Providers use operation-keyed handlers so each operation has its exact result.
        context.providers.register("com.example.connection.invalid", "connection", async () => ({ available: true }));

        context.providers.register("com.example.importer", "importer", {
          async parse(invocation) {
            const input = await invocation.input;
            await invocation.output.write(new Uint8Array([65]));
            await input.read();
            return { parsed: 0, warnings: 0, errors: 0 };
          },
          detect(invocation) {
            const sampleData: string = invocation.payload.sample.data;
            void sampleData;
            return { confidence: 1 };
          },
        });

        context.providers.register("com.example.sync", "sync", {
          connect(invocation) {
            void invocation.payload.configuration;
            return { account: { id: "acct" } };
          },
          disconnect() {
            return null;
          },
          getAccount() {
            return { account: { id: "acct" } };
          },
          getCapabilities() {
            return {
              revisions: true,
              conditionalWrites: true,
              atomicReplacement: true,
              maxObjectBytes: 1024,
            };
          },
          async readObject(invocation) {
            if (invocation.output) {
              await invocation.output.write(new Uint8Array([1, 2, 3]));
              await invocation.output.end();
              return { found: true, byteLength: 3, streamed: true, revision: "r1" };
            }
            return {
              found: true,
              byteLength: 3,
              encoding: "base64",
              data: "AQID",
              revision: "r1",
            };
          },
          async writeObject(invocation) {
            if (invocation.input) {
              const stream = await invocation.input;
              await stream.read();
            }
            return { created: true, revision: "r2" };
          },
          deleteObject() {
            return { deleted: true };
          },
        });

        // @ts-expect-error sync Providers use operation-keyed handlers.
        context.providers.register("com.example.sync.invalid", "sync", async () => ({ account: { id: "x" } }));
      },
    });
  `);
});

test("DisposableStore disposes every item once", () => {
  const store = new DisposableStore();
  const calls: string[] = [];
  store.add({ dispose: () => calls.push("first") });
  store.add({ dispose: () => calls.push("second") });

  store.dispose();
  store.dispose();

  assert.deepEqual(calls, ["first", "second"]);
});

test("DisposableStore disposes rejected late additions", () => {
  const store = new DisposableStore();
  store.dispose();
  let disposed = false;

  assert.throws(
    () => store.add({ dispose: () => { disposed = true; } }),
    (error) => error instanceof PluginError && error.code === "unavailable",
  );
  assert.equal(disposed, true);
});

test("CancellationTokenSource notifies listeners once", () => {
  const source = new CancellationTokenSource();
  let count = 0;
  source.token.onCancellationRequested(() => count += 1);

  source.cancel();
  source.cancel();

  assert.equal(count, 1);
  assert.equal(source.token.isCancellationRequested, true);
  assert.throws(
    () => throwIfCancellationRequested(source.token),
    CancellationError,
  );
});

test("CancellationTokenSource notifies every listener before reporting failures", () => {
  const source = new CancellationTokenSource();
  const calls: string[] = [];
  source.token.onCancellationRequested(() => {
    calls.push("failing");
    throw new Error("listener failed");
  });
  source.token.onCancellationRequested(() => calls.push("surviving"));

  assert.throws(
    () => source.cancel(),
    (error) => error instanceof AggregateError
      && error.errors.length === 1
      && error.errors[0] instanceof Error
      && error.errors[0].message === "listener failed",
  );
  assert.deepEqual(calls, ["failing", "surviving"]);
  assert.equal(source.token.isCancellationRequested, true);
  assert.doesNotThrow(() => source.cancel());
});

test("CancellationTokenSource finishes disposal when a cancellation listener fails", () => {
  const source = new CancellationTokenSource();
  source.token.onCancellationRequested(() => {
    throw new Error("listener failed");
  });

  assert.throws(() => source.dispose(true), AggregateError);
  assert.doesNotThrow(() => source.dispose(true));
});

test("WASM dispatch request encoding pins the lemonssh-wasm-abi v1 surface", () => {
  assert.equal(WASM_ABI.version, 1);
  assert.equal(WASM_ABI.hostModule, "lemonssh");
  assert.deepEqual(WASM_ABI.guestExports, {
    alloc: "lemonssh_alloc",
    free: "lemonssh_free",
    dispatch: "lemonssh_dispatch",
  });
  assert.deepEqual(WASM_ABI.hostImports, {
    log: "lemonssh_host_log",
    settingGet: "lemonssh_host_setting_get",
  });
  assert.deepEqual(WASM_HOST_IMPORT_STATUS, {
    ok: 0,
    permissionDenied: -1,
    invalidArgument: -2,
    unavailable: -3,
  });

  const request = new TextDecoder().decode(
    buildWasmDispatchRequest("ping", { from: "host" }),
  );
  assert.equal(request, `{"method":"ping","payload":{"from":"host"}}`);
  const bare = new TextDecoder().decode(buildWasmDispatchRequest("ping"));
  assert.equal(bare, `{"method":"ping"}`);
  assert.throws(() => buildWasmDispatchRequest(""), PluginError);
  assert.throws(
    () => buildWasmDispatchRequest("ping", { blob: "x".repeat(WASM_ABI.maxRequestBytes) }),
    (error) => error instanceof PluginError && error.code === "invalid_argument",
  );
});

test("WASM dispatch response parsing validates both envelope shapes", () => {
  const success = parseWasmDispatchResponse(
    new TextEncoder().encode(`{"ok":true,"result":{"pong":true}}`),
  );
  assert.deepEqual(success, { ok: true, result: { pong: true } });
  assert.deepEqual(
    parseWasmDispatchResponse(new TextEncoder().encode(`{"ok":true}`)),
    { ok: true },
  );
  const failure = parseWasmDispatchResponse(
    new TextEncoder().encode(
      `{"ok":false,"error":{"code":"not_found","message":"unknown method"}}`,
    ),
  );
  assert.deepEqual(failure, {
    ok: false,
    error: { code: "not_found", message: "unknown method" },
  });

  assert.throws(
    () => parseWasmDispatchResponse(new TextEncoder().encode(`{"ok":false}`)),
    (error) => error instanceof PluginError && error.code === "internal",
  );
  assert.throws(
    () => parseWasmDispatchResponse(new TextEncoder().encode(`{"ok":true,"error":{}}`)),
    PluginError,
  );
  assert.throws(
    () => parseWasmDispatchResponse(new TextEncoder().encode(`{"ok":1}`)),
    PluginError,
  );
  assert.throws(
    () => parseWasmDispatchResponse(new TextEncoder().encode(`{broken`)),
    (error) => error instanceof PluginError && error.code === "internal",
  );
  assert.throws(
    () =>
      parseWasmDispatchResponse(new Uint8Array(WASM_ABI.maxResponseBytes + 1)),
    (error) => error instanceof PluginError && error.code === "invalid_argument",
  );
});

test("view.data request encoding and result narrowing follow the canonical channel", () => {
  const request = new TextDecoder().decode(
    buildPluginViewDataRequest({
      viewId: "status",
      bindings: ["demo.greeting", "demo.greeting", "", "rows"],
    }),
  );
  assert.equal(
    request,
    `{"method":"view.data","payload":{"viewId":"status","bindings":["demo.greeting","rows"]}}`,
  );
  assert.throws(() => buildPluginViewDataRequest({ viewId: "", bindings: [] }), PluginError);
  assert.throws(
    () =>
      buildPluginViewDataRequest({
        viewId: "status",
        bindings: Array.from({ length: PLUGIN_VIEW_DATA_MAX_BINDINGS + 1 }, (_, i) => `b${i}`),
      }),
    (error) => error instanceof PluginError && error.code === "invalid_argument",
  );

  assert.deepEqual(
    parsePluginViewDataResult({ ok: true, result: { "demo.greeting": "hi" } }),
    { "demo.greeting": "hi" },
  );
  assert.equal(parsePluginViewDataResult({ ok: true }), null);
  assert.equal(
    parsePluginViewDataResult({ ok: false, error: { code: "not_found", message: "unknown" } }),
    null,
  );
  assert.equal(parsePluginViewDataResult({ ok: true, result: [1, 2] }), null);
});

test("provider registry protocol constants and decoders mirror the Go host", () => {
  assert.equal(PROVIDERS_LIST_DISPATCH_METHOD, "providers.list");
  assert.equal(PROVIDER_INVOKE_DISPATCH_METHOD, "provider.invoke");
  assert.equal(PROVIDER_SESSION_EVENT_DISPATCH_METHOD, "provider.sessionEvent");

  assert.deepEqual(
    parsePluginProviderListResult({
      ok: true,
      result: { providers: [{ id: "com.demo.accent", label: "Accent", kind: "terminal.theme" }, "junk", { nope: true }] },
    }),
    [{ id: "com.demo.accent", label: "Accent", kind: "terminal.theme" }],
  );
  assert.deepEqual(parsePluginProviderListResult({ ok: true }), []);
  assert.deepEqual(
    parsePluginProviderListResult({ ok: false, error: { code: "not_found", message: "unknown" } }),
    [],
  );
  assert.deepEqual(parsePluginProviderListResult({ ok: true, result: { providers: "nope" } }), []);

  const invoke = parsePluginProviderInvokeRequest({
    providerId: "com.demo.accent",
    kind: "terminal.theme",
    operation: "provideTheme",
    requestId: "terminal-1",
    session: { sessionId: "session-1", protocol: "ssh", status: "connected" },
    payload: { reason: "session-state" },
    deadlineMs: 1500,
  });
  assert.equal(invoke.providerId, "com.demo.accent");
  assert.equal(invoke.kind, "terminal.theme");
  assert.equal(invoke.operation, "provideTheme");
  assert.equal(invoke.requestId, "terminal-1");
  assert.deepEqual(invoke.session, { sessionId: "session-1", protocol: "ssh", status: "connected" });
  assert.deepEqual(invoke.payload, { reason: "session-state" });
  assert.equal(invoke.deadlineMs, 1500);

  assert.throws(() => parsePluginProviderInvokeRequest(undefined), PluginError);
  assert.throws(() => parsePluginProviderInvokeRequest("nope" as never), PluginError);
  assert.throws(
    () => parsePluginProviderInvokeRequest({ kind: "terminal.theme", operation: "x", requestId: "r" }),
    PluginError,
  );
  assert.throws(
    () =>
      parsePluginProviderInvokeRequest({
        providerId: "com.demo.accent",
        kind: "terminal.theme",
        operation: "x",
        requestId: "r",
      }),
    PluginError,
  );
});
