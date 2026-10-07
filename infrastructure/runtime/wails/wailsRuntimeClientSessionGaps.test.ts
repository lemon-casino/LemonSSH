// Focused tests for the session-gap bridge mappings (F03/F04/F05/F31/F42):
// remote history, distro probe, busy-close children, attach primitives and the
// SSH debug log toggle. Uses the same stub-binding pattern as
// wailsRuntimeClient.test.ts with a local minimal builder.

import assert from "node:assert/strict";
import { test } from "node:test";

import { createWailsRuntimeClient } from "./wailsRuntimeClient";
import type { WailsBindingDeps } from "./wailsRuntimeClient";
import { RECEIVE_WINDOW_BYTES } from "../../terminal/dataplane/frame";

function stubBindings(overrides: Partial<WailsBindingDeps["terminal"]> = {}): WailsBindingDeps {
  return {
    terminal: {
      Connect: async () => "term-1",
      StartLocal: async () => "local-1",
      Write: () => 0,
      Resize: () => undefined,
      Signal: () => undefined,
      Close: async () => undefined,
      Bootstrap: async (sessionID) => ({
        SessionID: sessionID,
        Generation: 1,
        DataToken: "d".repeat(64),
        UrgentToken: "u".repeat(64),
        WindowBytes: RECEIVE_WINDOW_BYTES,
      }),
      ListenAddr: () => "127.0.0.1:9",
      ...overrides,
    },
    sftp: {
      Open: async () => "sftp-1",
      List: async () => [],
      Mkdir: async () => undefined,
      Remove: async () => undefined,
      Rename: async () => undefined,
      Stat: async () => ({ path: "/", isDir: true, size: 0, mode: "drwxr-xr-x", modTime: "2026-09-10T00:00:00Z" }),
      Close: async () => undefined,
    },
    openDataPlane: () => ({ dispose: () => undefined }),
  };
}

test("readRemoteHistory forwards session ids and degrades without bindings", async () => {
  const calls: Array<{ id: string; limit: number }> = [];
  const bindings = stubBindings({
    ReadRemoteHistory: async (id, limit) => {
      calls.push({ id, limit });
      return { success: true, shell: "bash", bash: "ls" };
    },
  }) as WailsBindingDeps;
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;

  const history = await bridge.readRemoteHistory?.("ui-1", 250);
  assert.equal(history?.success, true);
  assert.equal(history?.shell, "bash");
  assert.deepEqual(calls, [{ id: "ui-1", limit: 250 }]);

  await bridge.readRemoteHistory?.("ui-1");
  assert.deepEqual(calls[1], { id: "ui-1", limit: 1000 });

  const bare = createWailsRuntimeClient(stubBindings() as WailsBindingDeps).transitionBridge;
  const fallback = await bare.readRemoteHistory?.("ui-1");
  assert.equal(fallback?.success, false);
});

test("getSessionDistroInfo forwards and fails soft without bindings", async () => {
  const seen: string[] = [];
  const bindings = stubBindings({
    GetSessionDistroInfo: async (id) => {
      seen.push(id);
      return { success: true, stdout: "ID=ubuntu" };
    },
  }) as WailsBindingDeps;
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  const result = await bridge.getSessionDistroInfo?.("ui-1");
  assert.equal(result?.success, true);
  assert.deepEqual(seen, ["ui-1"]);

  const bare = createWailsRuntimeClient(stubBindings() as WailsBindingDeps).transitionBridge;
  const fallback = await bare.getSessionDistroInfo?.("ui-1");
  assert.equal(fallback?.success, false);
});

test("ptyGetChildProcesses resolves ids and swallows failures for the busy-close probe", async () => {
  const bindings = stubBindings({
    PtyGetChildProcesses: async () => [{ pid: 4242, command: "make" }],
  }) as WailsBindingDeps;
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  assert.deepEqual(await bridge.ptyGetChildProcesses?.("ui-1"), [{ pid: 4242, command: "make" }]);

  const failing = stubBindings({
    PtyGetChildProcesses: async () => {
      throw new Error("native failure");
    },
  }) as WailsBindingDeps;
  assert.deepEqual(await createWailsRuntimeClient(failing).transitionBridge.ptyGetChildProcesses?.("ui-1"), []);
});

test("rebind attaches the rotated route locally and restore forwards the authorization", async () => {
  const restores: Array<{ id: string; authorization: string }> = [];
  let openedPlanes = 0;
  const bindings = stubBindings({
    RebindSessionOutput: async (id, authorization) => ({
      success: true,
      authorization,
      route: {
        sessionID: id,
        generation: 7,
        dataToken: "d".repeat(64),
        urgentToken: "u".repeat(64),
        windowBytes: RECEIVE_WINDOW_BYTES,
      },
    }),
    RestoreSessionOutput: async (id, authorization) => {
      restores.push({ id, authorization });
      return { success: true };
    },
  }) as WailsBindingDeps;
  bindings.openDataPlane = () => {
    openedPlanes += 1;
    return { dispose: () => undefined };
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;

  const rebind = await bridge.rebindTerminalSessionOutput?.("ui-1", "attach-auth");
  assert.equal(rebind?.success, true);
  assert.equal(openedPlanes, 1, "the popup must attach the rotated route immediately");

  const restore = await bridge.restoreTerminalSessionOutput?.("ui-1", null, "attach-auth");
  assert.equal(restore?.success, true);
  assert.deepEqual(restores, [{ id: "ui-1", authorization: "attach-auth" }]);

  const refused = stubBindings({
    RebindSessionOutput: async () => ({ success: false, error: "terminal attach requires an active flow-pause lease" }),
  }) as WailsBindingDeps;
  const refusedResult = await createWailsRuntimeClient(refused).transitionBridge.rebindTerminalSessionOutput?.("ui-1", "");
  assert.equal(refusedResult?.success, false);
});

test("route handoff detaches the previous owner and only it re-arms on restore", async () => {
  let teardowns = 0;
  let opens = 0;
  const handlers = new Map<string, (event: { data?: unknown }) => void>();
  const bindings = stubBindings() as WailsBindingDeps;
  bindings.events = {
    On: (name, callback) => {
      handlers.set(name, callback);
      return () => undefined;
    },
  };
  bindings.openDataPlane = () => {
    opens += 1;
    return { dispose: () => { teardowns += 1; } };
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  // A session already streaming at generation 1 in this window (home owner).
  await bridge.startSSHSession({ hostname: "h", username: "root" });
  const dispatch = (payload: Record<string, unknown>) => handlers.get("terminal:route-handoff")!({ data: payload });

  dispatch({ sessionId: "term-1", phase: "detached", generation: 7 });
  assert.equal(teardowns, 1, "the previous owner's plane must be disposed");

  // A stale detached broadcast (generation below what a plane here holds —
  // e.g. the rebind initiator's own broadcast) must not tear anything down.
  dispatch({ sessionId: "term-1", phase: "detached", generation: 7 });

  // Restore re-arms only the window that was suspended by the detached phase.
  dispatch({ sessionId: "term-1", phase: "restored", route: { sessionID: "term-1", generation: 8, dataToken: "d".repeat(64), urgentToken: "u".repeat(64), windowBytes: RECEIVE_WINDOW_BYTES } });
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(opens, 2, "restore attaches exactly once with the provided route");
});

test("flow pause leases round-trip lease ids and keep-paused options", async () => {
  const released: Array<{ leaseId: string; keepPaused?: boolean }> = [];
  const bindings = stubBindings({
    AcquireSessionFlowPauseLease: async () => ({ success: true, leaseId: "lease-1", authorization: "attach-auth" }),
    WaitSessionFlowPauseLease: async (_id, leaseId) => ({ success: leaseId === "lease-1" }),
    ReleaseSessionFlowPauseLease: async (_id, leaseId, options) => {
      released.push({ leaseId, keepPaused: options?.keepPaused });
      return { success: true };
    },
  }) as WailsBindingDeps;
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;

  const acquired = await bridge.acquireSessionFlowPauseLease?.("ui-1");
  assert.equal(acquired?.success, true);
  assert.equal(acquired?.authorization, "attach-auth");
  assert.deepEqual(await bridge.waitSessionFlowPauseLease?.("ui-1", "lease-1"), { success: true });
  await bridge.releaseSessionFlowPauseLease?.("ui-1", "lease-1", { keepPaused: true });
  assert.deepEqual(released, [{ leaseId: "lease-1", keepPaused: true }]);

  const bare = createWailsRuntimeClient(stubBindings() as WailsBindingDeps).transitionBridge;
  const missingLease = await bare.acquireSessionFlowPauseLease?.("ui-1");
  assert.equal(missingLease?.success, false);
  assert.match(missingLease?.error ?? "", /flow pause leases unavailable/);
});

test("snapshot and apply-snapshot round-trips map onto the Go coordinator", async () => {
  const responses: Array<{ requestId: string; accepted: boolean }> = [];
  const bindings = stubBindings({
    RequestSessionSnapshot: async (id, authorization) => ({
      success: true,
      snapshot: "SNAP",
      passwordPromptActive: true,
      cwd: "/tmp",
      title: "title",
      kittyKeyboardProtocolEnabled: false,
      sessionId: id,
      authorization,
    }),
    ApplySessionSnapshot: async (_id, snapshot, context, authorization) => ({
      success: true,
      context,
      snapshot,
      authorization,
    }),
    RespondApplySnapshot: async (requestId, accepted) => {
      responses.push({ requestId, accepted });
      return undefined;
    },
  }) as WailsBindingDeps;
  bindings.events = {
    On: (name, callback) => {
      if (name === "terminal:session-apply-snapshot") {
        callback({ data: { requestId: "req-9", sessionId: "ui-1", snapshot: "S", contextSnapshot: "", contextViewportSnapshot: "", contextScrollbackSnapshot: "", alternateScreen: false } });
      }
      return () => undefined;
    },
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;

  const snapshot = await bridge.requestTerminalSessionSnapshot?.("ui-1", "");
  assert.equal(snapshot?.success, true);
  assert.equal(snapshot?.snapshot, "SNAP");

  const applied = await bridge.applyTerminalSessionSnapshot?.("ui-1", "SNAP", {
    contextSnapshot: "",
    contextViewportSnapshot: "",
    contextScrollbackSnapshot: "",
    alternateScreen: false,
    passwordPromptActive: true,
    cwd: "/tmp",
    title: null,
  }, "");
  assert.equal(applied?.success, true);

  let accepted: boolean | undefined;
  const unsubscribe = bridge.onTerminalSessionApplySnapshot?.((payload) => {
    accepted = payload.requestId === "req-9";
    return accepted === true;
  });
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(accepted, true);
  assert.deepEqual(responses, [{ requestId: "req-9", accepted: true }]);
  unsubscribe?.();
});

test("setSshDebugLogsEnabled forwards the toggle and degrades without the Go method", async () => {
  const toggles: boolean[] = [];
  const bindings = stubBindings() as WailsBindingDeps;
  bindings.diagnosticLog = {
    SetSshDebugLogEnabled: async (enabled) => {
      toggles.push(enabled);
      return { enabled, path: "ssh-debug.log", exists: false, size: 0 };
    },
  };
  const bridge = createWailsRuntimeClient(bindings).transitionBridge;
  const info = await bridge.setSshDebugLogsEnabled?.(true);
  assert.equal(info?.enabled, true);
  assert.deepEqual(toggles, [true]);

  const bare = createWailsRuntimeClient(stubBindings() as WailsBindingDeps).transitionBridge;
  const fallback = await bare.setSshDebugLogsEnabled?.(false);
  assert.equal(fallback?.enabled, false);
});
