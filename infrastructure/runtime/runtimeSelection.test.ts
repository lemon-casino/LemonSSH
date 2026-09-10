import assert from "node:assert/strict";
import { test } from "node:test";

import { getActiveRuntimeClient, setActiveRuntimeClient } from "./runtimeClient";
import { installRuntimeClient } from "./bootstrap";
import { createWailsRuntimeClient, isWailsRuntime } from "./wails/wailsRuntimeClient";

test.afterEach(() => {
  setActiveRuntimeClient(undefined);
});

test("node test runtime selects the Electron adapter", () => {
  assert.equal(isWailsRuntime(), false);
  installRuntimeClient();
  // Under plain Node there is no window.netcatty, so the Electron adapter
  // refuses to install and no client is active.
  assert.equal(getActiveRuntimeClient(), undefined);
});

test("wails adapter routes migrated ports and rejects un-migrated fail-closed", () => {
  const client = createWailsRuntimeClient();
  // Slice A/B/C: SSH terminal sessions and SFTP browsing route to Go.
  assert.equal(typeof client.terminal.startSSHSession, "function");
  assert.equal(typeof client.sftp.listSftp, "function");
  assert.equal(typeof client.transitionBridge.startSSHSession, "function");
  assert.equal(typeof client.transitionBridge.onSessionData, "function");
  // Electron-owned capabilities of the same ports still fail closed.
  assert.equal(typeof client.terminal.startLocalSession, "function");
  assert.equal(typeof client.terminal.startTelnetSession, "function");
  assert.equal(typeof client.terminal.startSerialSession, "function");
  assert.throws(
    () => (client.terminal as unknown as Record<string, unknown>).startMoshSession,
    /not migrated to the Wails runtime yet/,
  );
  assert.throws(() => client.files.readClipboardText);
  assert.throws(() => client.app.quitApp());
});

test("wails transition bridge leaves unmigrated methods undefined for optional chaining", () => {
  const client = createWailsRuntimeClient();
  const bridge = client.transitionBridge as unknown as Record<string, unknown>;
  assert.equal(bridge.startMoshSession, undefined);
  assert.equal(bridge.setLanguage, undefined);
  assert.doesNotThrow(() => (bridge.setLanguage as undefined)?.("en"));
});

test("generated wails bindings expose the skeleton service surface", async () => {
  const bindings = await import("./wails/bindings/github.com/binaricat/netcatty/cmd/netcatty/netcattyservice.js");
  for (const method of ["Health", "Version", "ResolveWindowRole"]) {
    assert.equal(typeof (bindings as Record<string, unknown>)[method], "function", method);
  }
});
