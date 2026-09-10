import assert from "node:assert/strict";
import { test } from "node:test";

import { createWailsRuntimeClient } from "./wailsRuntimeClient";
import type { WailsBindingDeps } from "./wailsRuntimeClient";
import { RECEIVE_WINDOW_BYTES } from "../../terminal/dataplane/frame";

function stubBindings(overrides: Partial<WailsBindingDeps["terminal"]> = {}): WailsBindingDeps {
  const attached: string[] = [];
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
    openDataPlane: ({ onComplete }) => {
      attached.push("open");
      return {
        dispose: () => {
          attached.push("dispose");
          onComplete?.();
        },
      };
    },
  };
}

test("transitionBridge startSSHSession attaches the data plane", async () => {
  const bindings = stubBindings();
  const client = createWailsRuntimeClient(bindings);
  const chunks: string[] = [];
  client.transitionBridge.onSessionData("term-1", (data) => chunks.push(data));
  const id = await client.transitionBridge.startSSHSession({ hostname: "h", username: "u" });
  assert.equal(id, "term-1");
});

test("transitionBridge closeSession disposes the data plane", async () => {
  let disposed = false;
  const bindings = stubBindings();
  bindings.openDataPlane = () => ({
    dispose: () => {
      disposed = true;
    },
  });
  const client = createWailsRuntimeClient(bindings);
  await client.transitionBridge.startSSHSession({ hostname: "h", username: "u" });
  await client.transitionBridge.closeSession("term-1");
  assert.equal(disposed, true);
});

test("optional-chain startup calls do not throw on missing bridge methods", () => {
  const client = createWailsRuntimeClient(stubBindings());
  const bridge = client.transitionBridge;
  assert.doesNotThrow(() => {
    bridge.setLanguage?.("en");
    void bridge.getAppLockSettings?.();
    void bridge.rendererReady?.();
    void bridge.notifySettingsChanged?.({ key: "x", value: "y" });
  });
});

test("startLocalSession attaches the data plane", async () => {
  const bindings = stubBindings();
  const client = createWailsRuntimeClient(bindings);
  const id = await client.transitionBridge.startLocalSession?.({ shell: "cmd.exe" });
  assert.equal(id, "local-1");
});

test("onSessionData fans out chunks from the data plane", async () => {
  let deliver: ((chunk: string) => void) | undefined;
  const bindings = stubBindings();
  bindings.openDataPlane = ({ onData }) => {
    deliver = onData;
    return { dispose: () => undefined };
  };
  const client = createWailsRuntimeClient(bindings);
  const seen: string[] = [];
  const dispose = client.transitionBridge.onSessionData("term-1", (data) => seen.push(data));
  await client.transitionBridge.startSSHSession({ hostname: "h", username: "u" });
  deliver?.("prompt$ ");
  assert.deepEqual(seen, ["prompt$ "]);
  dispose();
  deliver?.("ignored");
  assert.deepEqual(seen, ["prompt$ "]);
});
