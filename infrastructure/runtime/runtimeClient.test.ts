import assert from "node:assert/strict";
import { test } from "node:test";

import {
  getActiveRuntimeClient,
  setActiveRuntimeClient,
} from "./runtimeClient";
import type { RuntimeClient } from "./runtimeClient";
import {
  createElectronRuntimeClient,
  getElectronBridge,
  installElectronRuntimeClient,
} from "./electron/electronRuntimeClient";
import { BridgeUnavailableError, netcattyBridge } from "../services/netcattyBridge";

test.afterEach(() => {
  setActiveRuntimeClient(undefined);
});

function stubBridge(methods: Record<string, unknown> = {}): NetcattyBridge {
  return methods as unknown as NetcattyBridge;
}

test("facade falls back to the Electron bridge and preserves semantics", () => {
  // Node test runtime has no window: the Electron fallback resolves to
  // undefined and require() surfaces BridgeUnavailableError.
  if (typeof window === "undefined") {
    assert.equal(getElectronBridge(), undefined);
    assert.equal(netcattyBridge.get(), undefined);
    assert.throws(() => netcattyBridge.require(), BridgeUnavailableError);
  }
});

test("registered RuntimeClient wins over the direct window fallback", () => {
  const bridge = stubBridge({ quitApp: () => "bridge" });
  const client = createElectronRuntimeClient(bridge);
  setActiveRuntimeClient(client);
  assert.equal(getActiveRuntimeClient(), client);
  assert.equal(netcattyBridge.get(), bridge);
  assert.equal(netcattyBridge.require(), bridge);
  assert.equal(netcattyBridge.get()?.quitApp?.(), "bridge");
});

test("clearing the active client restores the previous resolution", () => {
  setActiveRuntimeClient(createElectronRuntimeClient(stubBridge()));
  setActiveRuntimeClient(undefined);
  assert.equal(getActiveRuntimeClient(), undefined);
  assert.equal(netcattyBridge.get(), getElectronBridge());
});

test("electron adapter groups every domain port on the same bridge", () => {
  const bridge = stubBridge();
  const client = createElectronRuntimeClient(bridge);
  const ports: Array<keyof RuntimeClient> = [
    "app", "agent", "files", "script", "terminal", "sftp", "sync", "system", "plugin", "transitionBridge",
  ];
  for (const port of ports) {
    assert.equal(client[port], bridge, `port ${port} must resolve to the electron bridge`);
  }
});

test("installElectronRuntimeClient registers only when the bridge exists", () => {
  const installed = installElectronRuntimeClient();
  if (getElectronBridge()) {
    assert.equal(installed, true);
    assert.ok(getActiveRuntimeClient());
    assert.equal(netcattyBridge.get(), getElectronBridge());
  } else {
    assert.equal(installed, false);
    assert.equal(getActiveRuntimeClient(), undefined);
  }
});
