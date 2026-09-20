import assert from "node:assert/strict";
import { test } from "node:test";

import { getActiveRuntimeClient, setActiveRuntimeClient } from "./runtimeClient";
import { installRuntimeClient } from "./bootstrap";
import { createWailsRuntimeClient, isWailsRuntime } from "./wails/wailsRuntimeClient";

test.afterEach(() => {
  setActiveRuntimeClient(undefined);
});

test("node test runtime refuses to install without the Wails host", () => {
  assert.equal(isWailsRuntime(), false);
  assert.throws(() => installRuntimeClient(), /requires the Wails runtime/);
  assert.equal(getActiveRuntimeClient(), undefined);
});

test("wails adapter exposes the desktop runtime ports", () => {
  const client = createWailsRuntimeClient();
  assert.equal(typeof client.terminal.startSSHSession, "function");
  assert.equal(typeof client.sftp.listSftp, "function");
  assert.equal(typeof client.transitionBridge.startSSHSession, "function");
  assert.equal(typeof client.transitionBridge.onSessionData, "function");
  assert.equal(typeof client.terminal.startLocalSession, "function");
  assert.equal(typeof client.terminal.startTelnetSession, "function");
  assert.equal(typeof client.terminal.startSerialSession, "function");
  assert.equal(typeof client.transitionBridge.startMoshSession, "function");
  assert.equal(typeof client.files.readClipboardText, "function");
  assert.equal(typeof client.app.quitApp, "function");
});

test("wails transition bridge exposes compatibility methods", () => {
  const client = createWailsRuntimeClient();
  const bridge = client.transitionBridge as unknown as Record<string, unknown>;
  assert.equal(typeof bridge.startMoshSession, "function");
  assert.equal(typeof bridge.startEtSession, "function");
  assert.equal(typeof bridge.setLanguage, "function");
});

test("generated wails bindings expose the skeleton service surface", async () => {
  const bindings = await import("./wails/bindings/github.com/binaricat/netcatty/cmd/netcatty/netcattyservice.js");
  for (const method of ["Health", "Version", "ResolveWindowRole"]) {
    assert.equal(typeof (bindings as Record<string, unknown>)[method], "function", method);
  }
});
