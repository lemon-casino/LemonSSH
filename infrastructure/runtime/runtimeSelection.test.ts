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

test("wails adapter rejects un-migrated ports fail-closed", () => {
  const client = createWailsRuntimeClient();
  assert.throws(() => client.terminal.startSSHSession);
  assert.throws(() => client.files.readClipboardText);
  assert.throws(() => client.app.quitApp());
});

test("wails transition bridge fails closed on call, not on access", () => {
  const client = createWailsRuntimeClient();
  const bridge = client.transitionBridge;
  const method = (bridge as unknown as Record<string, () => unknown>).startSSHSession;
  assert.equal(typeof method, "function");
  assert.throws(() => method());
});

test("generated wails bindings expose the skeleton service surface", async () => {
  const bindings = await import("./wails/bindings/github.com/binaricat/netcatty/cmd/netcatty/netcattyservice.js");
  for (const method of ["Health", "Version", "ResolveWindowRole"]) {
    assert.equal(typeof (bindings as Record<string, unknown>)[method], "function", method);
  }
});
