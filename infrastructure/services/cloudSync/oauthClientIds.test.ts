import assert from "node:assert/strict";
import { test } from "node:test";

import { SYNC_CONSTANTS } from "../../../domain/sync";
import {
  getOAuthClientIds,
  requireOAuthClientId,
  resolveOAuthClientId,
  setOAuthClientId,
  subscribeOAuthClientIds,
} from "./oauthClientIds";

// The store persists through hostStorageAdapter -> localStorage; the plain
// Node environment has no localStorage, so tests install a minimal stub.
const storage = new Map<string, string>();
Object.defineProperty(globalThis, "localStorage", {
  configurable: true,
  value: {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => void storage.set(key, value),
    removeItem: (key: string) => void storage.delete(key),
  },
});

test("runtime override wins over the build-time constant and blank clears it", () => {
  const fallback = SYNC_CONSTANTS.GOOGLE_CLIENT_ID || "";
  setOAuthClientId("google", "runtime-id.apps.googleusercontent.com");
  assert.equal(resolveOAuthClientId("google"), "runtime-id.apps.googleusercontent.com");

  setOAuthClientId("google", "   ");
  assert.equal(resolveOAuthClientId("google"), fallback, "blank override must fall back to the build constant");

  assert.deepEqual(getOAuthClientIds().google, undefined);
});

test("requireOAuthClientId refuses to open a provider authorize URL without an ID", () => {
  const hadGithub = Boolean(SYNC_CONSTANTS.GITHUB_CLIENT_ID);
  if (hadGithub) {
    // A distribution build with a pinned ID always resolves.
    assert.ok(resolveOAuthClientId("github").length > 0);
    return;
  }
  assert.throws(() => requireOAuthClientId("github"), /client ID is not configured/);
});

test("store changes notify subscribers and keep the snapshot immutable", () => {
  let notified = 0;
  const unsubscribe = subscribeOAuthClientIds(() => {
    notified += 1;
  });
  const before = getOAuthClientIds();
  setOAuthClientId("onedrive", "00000000-1111-2222-3333-444444444444");
  assert.equal(notified, 1);
  assert.notEqual(getOAuthClientIds(), before, "snapshot must be a fresh object");
  assert.equal(before.onedrive, undefined, "previous snapshot must be untouched");
  assert.equal(resolveOAuthClientId("onedrive"), "00000000-1111-2222-3333-444444444444");
  unsubscribe();
  setOAuthClientId("onedrive", "");
  assert.equal(notified, 1, "unsubscribed listeners are not called");
});
