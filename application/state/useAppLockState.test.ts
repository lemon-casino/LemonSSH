import assert from "node:assert/strict";
import test from "node:test";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";

import type { AppLockPasswordVerifier, AppLockSettings } from "../../domain/appLock.ts";
import {
  createIdleLockWatchdog,
  createOptimisticUnlockedRuntimeState,
  DEFAULT_APP_LOCK_SYSTEM_UNLOCK_STATUS,
  getIdleLockDelayMs,
  normalizeAppLockSystemUnlockStatus,
  normalizeAppLockSystemUnlockResult,
  resolveIdleLockDecision,
  resolveUnlockAttempt,
  resolveAppLockRuntimeWhenOwnerMissing,
  shouldLockAfterIdle,
  shouldLockOnStartup,
  useAppLockState,
} from "./useAppLockState.ts";
import { selectPreferredRuntimeAppLockState, type RuntimeAppLockState } from "./useAppLockRuntime.ts";

const verifier: AppLockPasswordVerifier = {
  version: 1,
  algorithm: "PBKDF2-SHA256",
  iterations: 210000,
  salt: Buffer.alloc(16, 1).toString("base64"),
  hash: Buffer.alloc(32, 2).toString("base64"),
};

test("resolveAppLockRuntimeWhenOwnerMissing treats a missing owner as unlocked", () => {
  const uninitialized = {
    initialized: false,
    locked: false,
    reason: null,
    version: 0,
    lastLockedAt: null,
    lastUnlockedAt: null,
    lastActivityAt: null,
  };
  assert.deepEqual(
    resolveAppLockRuntimeWhenOwnerMissing(uninitialized, false),
    { ...uninitialized, initialized: true },
  );
  assert.equal(resolveAppLockRuntimeWhenOwnerMissing(uninitialized, true), uninitialized);

  const wailsStub = new Proxy({}, {
    get: () => () => {
      throw new Error("not available");
    },
  });
  assert.equal('getAppLockRuntimeState' in wailsStub, false);
  assert.equal(typeof (wailsStub as { getAppLockRuntimeState?: unknown }).getAppLockRuntimeState, 'function');
});

test("shouldLockOnStartup locks only when enabled with a verifier", () => {
  const enabled: AppLockSettings = {
    enabled: true,
    timeoutMinutes: 15,
    systemUnlockEnabled: false,
    systemUnlockAutoPromptEnabled: false,
    passwordVerifier: verifier,
  };

  assert.equal(shouldLockOnStartup(enabled), true);
  assert.equal(shouldLockOnStartup({ ...enabled, enabled: false }), false);
  assert.equal(shouldLockOnStartup({ ...enabled, passwordVerifier: null }), false);
});

test("shouldLockAfterIdle honors the configured timeout", () => {
  const settings: AppLockSettings = {
    enabled: true,
    timeoutMinutes: 5,
    systemUnlockEnabled: false,
    systemUnlockAutoPromptEnabled: false,
    passwordVerifier: verifier,
  };

  assert.equal(shouldLockAfterIdle(settings, 1_000, 1_000 + 5 * 60_000 - 1), false);
  assert.equal(shouldLockAfterIdle(settings, 1_000, 1_000 + 5 * 60_000), true);
  assert.equal(shouldLockAfterIdle({ ...settings, timeoutMinutes: 0 }, 1_000, 1_000 + 60 * 60_000), false);
  assert.equal(shouldLockAfterIdle({ ...settings, enabled: false }, 1_000, 1_000 + 60 * 60_000), false);
  assert.equal(shouldLockAfterIdle({ ...settings, passwordVerifier: null }, 1_000, 1_000 + 60 * 60_000), false);
});

test("getIdleLockDelayMs schedules the next check after remaining idle time", () => {
  const settings: AppLockSettings = {
    enabled: true,
    timeoutMinutes: 5,
    systemUnlockEnabled: false,
    systemUnlockAutoPromptEnabled: false,
    passwordVerifier: verifier,
  };

  assert.equal(getIdleLockDelayMs(settings, 1_000, 1_000), 5 * 60_000);
  assert.equal(getIdleLockDelayMs(settings, 1_000, 1_000 + 4 * 60_000), 60_000);
  assert.equal(getIdleLockDelayMs(settings, 1_000, 1_000 + 5 * 60_000), 0);
  assert.equal(getIdleLockDelayMs({ ...settings, timeoutMinutes: 0 }, 1_000, 1_000), null);
  assert.equal(getIdleLockDelayMs({ ...settings, enabled: false }, 1_000, 1_000), null);
  assert.equal(getIdleLockDelayMs({ ...settings, passwordVerifier: null }, 1_000, 1_000), null);
});

test("resolveIdleLockDecision defers to fresher cross-window activity before locking", () => {
  const settings: AppLockSettings = {
    enabled: true,
    timeoutMinutes: 5,
    systemUnlockEnabled: false,
    systemUnlockAutoPromptEnabled: false,
    passwordVerifier: verifier,
  };

  // Local input is stale but another window reported activity 1 minute ago:
  // no lock, and the watchdog re-arms for the remaining budget.
  assert.deepEqual(
    resolveIdleLockDecision({
      settings,
      localLastActivityAt: 1_000,
      crossWindowLastActivityAt: 1_000 + 4 * 60_000,
      now: 1_000 + 5 * 60_000,
    }),
    { lock: false, activityAt: 1_000 + 4 * 60_000, delayMs: 4 * 60_000 },
  );

  // No cross-window data and the local clock passed the deadline: lock now.
  assert.deepEqual(
    resolveIdleLockDecision({
      settings,
      localLastActivityAt: 1_000,
      crossWindowLastActivityAt: null,
      now: 1_000 + 5 * 60_000,
    }),
    { lock: true, activityAt: 1_000, delayMs: 0 },
  );

  // Idle locking disabled by the timeout setting: nothing to schedule, but
  // the merged activity timestamp is still reported.
  assert.deepEqual(
    resolveIdleLockDecision({
      settings: { ...settings, timeoutMinutes: 0 },
      localLastActivityAt: 1_000,
      crossWindowLastActivityAt: 2_000,
      now: 3_000,
    }),
    { lock: false, activityAt: 2_000, delayMs: null },
  );

  // Without a configured verifier idle locking stays off.
  assert.deepEqual(
    resolveIdleLockDecision({
      settings: { ...settings, passwordVerifier: null },
      localLastActivityAt: 1_000,
      crossWindowLastActivityAt: null,
      now: 60_000_000,
    }),
    { lock: false, activityAt: 1_000, delayMs: null },
  );
});

test("createOptimisticUnlockedRuntimeState clears stale locked state after successful unlock", () => {
  const nextState = createOptimisticUnlockedRuntimeState(
    {
      initialized: false,
      locked: true,
      reason: "startup",
      version: 7,
      lastLockedAt: 2_000,
      lastUnlockedAt: null,
      lastActivityAt: 1_000,
    },
    5_000,
  );

  assert.deepEqual(nextState, {
    initialized: true,
    locked: false,
    reason: null,
    // Optimistic unlock must not invent a higher version than main has sent.
    version: 7,
    lastLockedAt: 2_000,
    lastUnlockedAt: 5_000,
    lastActivityAt: 5_000,
  });
});

test("createOptimisticUnlockedRuntimeState keeps observed version so concurrent re-lock can win", () => {
  const optimistic = createOptimisticUnlockedRuntimeState(
    {
      initialized: true,
      locked: true,
      reason: "startup",
      version: 7,
      lastLockedAt: 2_000,
      lastUnlockedAt: null,
      lastActivityAt: 1_000,
    },
    5_000,
  );

  assert.equal(optimistic.version, 7);
  assert.equal(optimistic.locked, false);
  // A concurrent main re-lock at version 8 must outrank the optimistic unlock.
  const preferred = selectPreferredRuntimeAppLockState(optimistic, {
    ...optimistic,
    locked: true,
    reason: "manual",
    version: 8,
    lastLockedAt: 6_000,
  });
  assert.equal(preferred.locked, true);
  assert.equal(preferred.version, 8);
});

test("normalizes app lock system unlock bridge status and results", () => {
  assert.deepEqual(DEFAULT_APP_LOCK_SYSTEM_UNLOCK_STATUS, {
    supported: false,
    available: false,
    enabled: false,
    platform: "unsupported",
    label: null,
    reason: null,
  });
  assert.deepEqual(
    normalizeAppLockSystemUnlockStatus({
      supported: true,
      available: true,
      enabled: true,
      platform: "win32",
      label: "Windows Hello",
      reason: "",
    }),
    {
      supported: true,
      available: true,
      enabled: true,
      platform: "win32",
      label: "Windows Hello",
      reason: null,
    },
  );
  assert.deepEqual(normalizeAppLockSystemUnlockResult({ ok: true }), { ok: true });
  assert.deepEqual(normalizeAppLockSystemUnlockResult({ ok: false, error: "cancelled" }), {
    ok: false,
    error: "cancelled",
  });
  assert.deepEqual(normalizeAppLockSystemUnlockResult({ ok: false, error: "unknown" }), {
    ok: false,
    error: "failed",
  });
});


test("resolveUnlockAttempt validates empty, incorrect, and correct passwords", async () => {
  const originalWindow = globalThis.window;
  globalThis.window = {
    lemonssh: {
      requestAppLockUnlock: async (password: string) =>
        password === "secret"
          ? { ok: true as const }
          : { ok: false as const, error: "incorrect" as const },
    },
  } as typeof window;

  try {
    assert.deepEqual(await resolveUnlockAttempt(""), { ok: false, error: "empty" });
    assert.deepEqual(await resolveUnlockAttempt("wrong"), { ok: false, error: "incorrect" });
    assert.deepEqual(await resolveUnlockAttempt("secret"), { ok: true });
  } finally {
    globalThis.window = originalWindow;
  }
});

test("createIdleLockWatchdog re-arms on fresher cross-window activity and locks through Go when idle", async () => {
  const flush = () => new Promise<void>((resolve) => setImmediate(resolve));
  let nowMs = 1_000;
  type FakeTimer = { fn: () => void; at: number };
  const pending = new Set<FakeTimer>();
  const scheduleTimeout = (fn: () => void, delayMs: number) => {
    const timer: FakeTimer = { fn, at: nowMs + delayMs };
    pending.add(timer);
    return timer;
  };
  const cancelTimeout = (handle: unknown) => { pending.delete(handle as FakeTimer); };
  const fire = async (timer: FakeTimer) => {
    assert.ok(pending.has(timer), "timer must be pending");
    pending.delete(timer);
    nowMs = timer.at;
    timer.fn();
    await flush();
  };

  const lockReasons: string[] = [];
  const crossWindowActivity = { at: null as number | null };
  const bridge = {
    getAppLockRuntimeState: async () => ({ lastActivityAt: crossWindowActivity.at }),
    setAppLockRuntimeLocked: async (reason: string) => { lockReasons.push(reason); },
  } as unknown as Pick<LemonSSHBridge, "getAppLockRuntimeState" | "setAppLockRuntimeLocked">;
  const lastActivityRef = { current: 1_000 };
  const settings: AppLockSettings = {
    enabled: true,
    timeoutMinutes: 5,
    systemUnlockEnabled: false,
    systemUnlockAutoPromptEnabled: false,
    passwordVerifier: verifier,
  };

  const dispose = createIdleLockWatchdog({
    settings,
    bridge,
    lastActivityRef,
    scheduleTimeout,
    cancelTimeout,
    now: () => nowMs,
  });
  assert.equal(pending.size, 1);
  const first = [...pending][0];
  assert.equal(first.at, 1_000 + 5 * 60_000, "armed with the full idle budget");

  // Another window reported activity 30 seconds before the deadline fires:
  // no lock; the watchdog re-arms to the new deadline (anchor + timeout).
  crossWindowActivity.at = first.at - 30_000;
  await fire(first);
  assert.deepEqual(lockReasons, []);
  assert.equal(pending.size, 1);
  const rearmed = [...pending][0];
  assert.equal(rearmed.at, first.at + (5 * 60_000 - 30_000));
  assert.equal(lastActivityRef.current, first.at - 30_000);

  // Deadline fires with no fresher activity anywhere: request the idle lock.
  await fire(rearmed);
  assert.deepEqual(lockReasons, ["idle"]);
  assert.equal(pending.size, 0, "no re-arm after the lock request");

  dispose();
});

test("createIdleLockWatchdog schedules nothing when idle locking is disabled and disposal cancels", async () => {
  let nowMs = 5_000;
  const pending = new Set<{ fn: () => void }>();
  const dispose = createIdleLockWatchdog({
    settings: {
      enabled: true,
      timeoutMinutes: 0,
      systemUnlockEnabled: false,
      systemUnlockAutoPromptEnabled: false,
      passwordVerifier: verifier,
    },
    bridge: undefined,
    lastActivityRef: { current: nowMs },
    scheduleTimeout: (fn) => { const timer = { fn }; pending.add(timer); return timer; },
    cancelTimeout: (handle) => { pending.delete(handle as { fn: () => void }); },
    now: () => nowMs,
  });
  assert.equal(pending.size, 0, "no watchdog scheduled when the timeout is disabled");
  dispose();

  // With a live timeout, disposal must cancel the pending evaluation.
  const liveDispose = createIdleLockWatchdog({
    settings: {
      enabled: true,
      timeoutMinutes: 5,
      systemUnlockEnabled: false,
      systemUnlockAutoPromptEnabled: false,
      passwordVerifier: verifier,
    },
    bridge: undefined,
    lastActivityRef: { current: nowMs },
    scheduleTimeout: (fn) => { const timer = { fn }; pending.add(timer); return timer; },
    cancelTimeout: (handle) => { pending.delete(handle as { fn: () => void }); },
    now: () => nowMs,
  });
  assert.equal(pending.size, 1);
  liveDispose();
  assert.equal(pending.size, 0, "disposal cancels the pending timer");
});

const RESET_SETTINGS: AppLockSettings = {
  enabled: true,
  timeoutMinutes: 15,
  systemUnlockEnabled: false,
  systemUnlockAutoPromptEnabled: false,
  passwordVerifier: verifier,
};

const LOCKED_STARTUP_STATE: RuntimeAppLockState = {
  initialized: true,
  locked: true,
  reason: "startup",
  version: 1,
  lastLockedAt: 1_000,
  lastUnlockedAt: null,
  lastActivityAt: 1_000,
};

/** The bridge surface useAppLockState touches on the reset path. */
type AppLockResetBridgeStub = Pick<
  LemonSSHBridge,
  | "getAppLockRuntimeState"
  | "onAppLockRuntimeStateChanged"
  | "requestAppLockReset"
  | "getAppLockSystemUnlockStatus"
  | "reportAppLockActivity"
>;

/** Mounts useAppLockState against a JSDOM window whose bridge is `bridge`. */
async function mountAppLockState(bridge: AppLockResetBridgeStub) {
  const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/" });
  const define = (target: Record<string, unknown>, key: string, value: unknown) => {
    Object.defineProperty(target, key, { configurable: true, writable: true, value });
  };
  const originalWindow = globalThis.window;
  define(globalThis, "document", dom.window.document);
  define(globalThis, "IS_REACT_ACT_ENVIRONMENT", true);
  if (!globalThis.navigator) define(globalThis, "navigator", dom.window.navigator);
  globalThis.window = Object.assign(dom.window, { lemonssh: bridge }) as unknown as typeof window;

  const container = dom.window.document.createElement("div");
  dom.window.document.body.appendChild(container);
  const root = createRoot(container);
  let appLock!: ReturnType<typeof useAppLockState>;

  function Host() {
    appLock = useAppLockState(RESET_SETTINGS);
    return null;
  }

  await act(async () => {
    root.render(React.createElement(Host));
  });

  return {
    appLock: () => appLock,
    async dispose() {
      await act(async () => {
        root.unmount();
      });
      container.remove();
      globalThis.window = originalWindow;
    },
  };
}

function createResetBridgeHarness(): { bridge: AppLockResetBridgeStub; getResetAttempts: () => string[] } {
  let runtimeState: RuntimeAppLockState = { ...LOCKED_STARTUP_STATE };
  const runtimeListeners = new Set<(state: RuntimeAppLockState) => void>();
  const resetAttempts: string[] = [];
  const bridge: AppLockResetBridgeStub = {
    getAppLockRuntimeState: async () => ({ ...runtimeState }),
    onAppLockRuntimeStateChanged: (listener) => {
      runtimeListeners.add(listener);
      return () => runtimeListeners.delete(listener);
    },
    requestAppLockReset: async (currentPassword) => {
      resetAttempts.push(currentPassword);
      if (!currentPassword) return { ok: false as const, error: "empty-current" as const };
      if (currentPassword !== "secret") return { ok: false as const, error: "incorrect" as const };
      runtimeState = {
        ...runtimeState,
        locked: false,
        reason: null,
        version: runtimeState.version + 1,
        lastUnlockedAt: 5_000,
        lastActivityAt: 5_000,
      };
      for (const listener of runtimeListeners) listener({ ...runtimeState });
      return {
        enabled: false,
        timeoutMinutes: 15,
        systemUnlockEnabled: false,
        systemUnlockAutoPromptEnabled: false,
        passwordVerifier: null,
      };
    },
    getAppLockSystemUnlockStatus: async () => ({
      supported: false,
      available: false,
      enabled: false,
      platform: "unsupported" as const,
      label: null,
      reason: null,
    }),
    reportAppLockActivity: async () => {},
  };
  return { bridge, getResetAttempts: () => [...resetAttempts] };
}

test("useAppLockState reset unlocks through the bridge on the normal path", async () => {
  const { bridge, getResetAttempts } = createResetBridgeHarness();
  const mounted = await mountAppLockState(bridge);

  try {
    assert.equal(mounted.appLock().locked, true, "hook starts locked from the runtime state");

    await assert.rejects(
      async () => { await act(async () => { await mounted.appLock().reset("wrong"); }); },
      /incorrect/,
    );
    assert.equal(mounted.appLock().locked, true, "a wrong password keeps the app locked");

    await act(async () => { await mounted.appLock().reset("secret"); });
    assert.deepEqual(getResetAttempts(), ["wrong", "secret"]);
    assert.equal(mounted.appLock().locked, false, "the correct password resets and unlocks the app");
  } finally {
    await mounted.dispose();
  }
});

test("useAppLockState reset keeps the lock when the reset bridge is unavailable", async () => {
  const { bridge } = createResetBridgeHarness();
  const bridgeWithoutReset = { ...bridge };
  delete bridgeWithoutReset.requestAppLockReset;
  const mounted = await mountAppLockState(bridgeWithoutReset);

  try {
    await assert.rejects(
      async () => { await act(async () => { await mounted.appLock().reset("secret"); }); },
      /App Lock reset bridge is unavailable/,
    );
    assert.equal(mounted.appLock().locked, true, "the app must stay locked without a reset bridge");
  } finally {
    await mounted.dispose();
  }
});
