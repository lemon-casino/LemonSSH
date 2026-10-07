import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import {
  normalizeAppLockSettings,
  type AppLockSettings,
} from '../../domain/appLock';
import { lemonsshBridge } from '../../infrastructure/services/lemonsshBridge';
import {
  normalizeRuntimeAppLockState,
  useAppLockRuntime,
  type RuntimeAppLockReason,
  type RuntimeAppLockState,
} from './useAppLockRuntime';

export type AppLockReason = RuntimeAppLockReason;
export type AppLockUnlockResult =
  | { ok: true }
  | { ok: false; error: 'empty' | 'incorrect' };
export interface AppLockSystemUnlockStatus {
  supported: boolean;
  available: boolean;
  enabled: boolean;
  platform: 'darwin' | 'win32' | 'unsupported';
  label: 'Touch ID' | 'Windows Hello' | null;
  reason: string | null;
}
export type AppLockSystemUnlockResult =
  | { ok: true }
  | { ok: false; error: 'disabled' | 'not-locked' | 'unsupported' | 'unavailable' | 'cancelled' | 'failed' };

export const DEFAULT_APP_LOCK_SYSTEM_UNLOCK_STATUS: AppLockSystemUnlockStatus = {
  supported: false,
  available: false,
  enabled: false,
  platform: 'unsupported',
  label: null,
  reason: null,
};

export function normalizeAppLockSystemUnlockStatus(input: unknown): AppLockSystemUnlockStatus {
  const record = input && typeof input === 'object' ? input as Record<string, unknown> : null;
  const platform = record?.platform === 'darwin' || record?.platform === 'win32'
    ? record.platform
    : 'unsupported';
  const label = record?.label === 'Touch ID' || record?.label === 'Windows Hello'
    ? record.label
    : null;
  return {
    supported: record?.supported === true,
    available: record?.available === true,
    enabled: record?.enabled === true,
    platform,
    label,
    reason: typeof record?.reason === 'string' && record.reason ? record.reason : null,
  };
}

export function normalizeAppLockSystemUnlockResult(input: unknown): AppLockSystemUnlockResult {
  const record = input && typeof input === 'object' ? input as Record<string, unknown> : null;
  if (record?.ok === true) return { ok: true };
  const error = record?.error;
  if (
    error === 'disabled' ||
    error === 'not-locked' ||
    error === 'unsupported' ||
    error === 'unavailable' ||
    error === 'cancelled' ||
    error === 'failed'
  ) {
    return { ok: false, error };
  }
  return { ok: false, error: 'failed' };
}

export function resolveAppLockRuntimeWhenOwnerMissing(
  runtime: RuntimeAppLockState,
  hasRuntimeOwner: boolean,
): RuntimeAppLockState {
  if (hasRuntimeOwner || runtime.initialized) return runtime;
  return {
    ...runtime,
    initialized: true,
    locked: false,
    reason: null,
  };
}

export function shouldLockOnStartup(settings: AppLockSettings): boolean {
  const normalized = normalizeAppLockSettings(settings);
  return normalized.enabled && normalized.passwordVerifier !== null;
}

export function shouldLockAfterIdle(
  settings: AppLockSettings,
  lastActivityAt: number,
  now: number,
): boolean {
  const normalized = normalizeAppLockSettings(settings);
  if (!normalized.enabled || !normalized.passwordVerifier) return false;
  if (normalized.timeoutMinutes <= 0) return false;
  return now - lastActivityAt >= normalized.timeoutMinutes * 60_000;
}

export function getIdleLockDelayMs(
  settings: AppLockSettings,
  lastActivityAt: number,
  now: number,
): number | null {
  const normalized = normalizeAppLockSettings(settings);
  if (!normalized.enabled || !normalized.passwordVerifier) return null;
  if (normalized.timeoutMinutes <= 0) return null;
  const timeoutMs = normalized.timeoutMinutes * 60_000;
  return Math.max(0, timeoutMs - (now - lastActivityAt));
}

export interface IdleLockDecision {
  /** True when the merged activity clock says the idle timeout elapsed. */
  lock: boolean;
  /** The freshest activity timestamp across local input and the Go clock. */
  activityAt: number;
  /** Remaining idle budget in ms; `null` when idle locking is disabled. */
  delayMs: number | null;
}

/**
 * Merges the local renderer activity clock with the Go-side cross-window
 * clock (the real idle source: every window's ReportActivity lands there)
 * and decides whether the app should lock after idle right now.
 */
export function resolveIdleLockDecision(input: {
  settings: AppLockSettings;
  localLastActivityAt: number;
  crossWindowLastActivityAt: number | null;
  now: number;
}): IdleLockDecision {
  const activityAt = Math.max(input.localLastActivityAt, input.crossWindowLastActivityAt ?? 0);
  return {
    lock: shouldLockAfterIdle(input.settings, activityAt, input.now),
    activityAt,
    delayMs: getIdleLockDelayMs(input.settings, activityAt, input.now),
  };
}

export interface IdleLockWatchdogOptions {
  settings: AppLockSettings;
  bridge: Pick<LemonSSHBridge, 'getAppLockRuntimeState' | 'setAppLockRuntimeLocked'> | null | undefined;
  lastActivityRef: { current: number };
  scheduleTimeout: (callback: () => void, delayMs: number) => unknown;
  cancelTimeout: (handle: unknown) => void;
  now: () => number;
}

/**
 * Arms and evaluates the renderer-side idle auto-lock schedule; returns a
 * disposer. Go owns the authoritative idle timer (it sees every window's
 * ReportActivity), so this watchdog never locks on its own clock: before
 * requesting the "idle" lock it re-checks shouldLockAfterIdle against Go's
 * cross-window activity clock, and Go re-verifies the request server-side.
 */
export function createIdleLockWatchdog(options: IdleLockWatchdogOptions): () => void {
  const { settings, bridge, lastActivityRef, scheduleTimeout, cancelTimeout, now } = options;
  let cancelled = false;
  let timer: unknown;
  const arm = () => {
    if (cancelled) return;
    if (timer !== undefined) cancelTimeout(timer);
    const decision = resolveIdleLockDecision({
      settings,
      localLastActivityAt: lastActivityRef.current,
      crossWindowLastActivityAt: null,
      now: now(),
    });
    if (decision.delayMs === null) return;
    timer = scheduleTimeout(() => { void evaluate(); }, decision.delayMs);
  };
  const evaluate = async () => {
    if (cancelled) return;
    timer = undefined;
    let crossWindowLastActivityAt: number | null = null;
    try {
      const state = await bridge?.getAppLockRuntimeState?.();
      if (state && typeof state.lastActivityAt === 'number') {
        crossWindowLastActivityAt = state.lastActivityAt;
      }
    } catch {
      // Keep the local anchor when the runtime state cannot be read.
    }
    if (cancelled) return;
    const decision = resolveIdleLockDecision({
      settings,
      localLastActivityAt: lastActivityRef.current,
      crossWindowLastActivityAt,
      now: now(),
    });
    lastActivityRef.current = Math.max(lastActivityRef.current, decision.activityAt);
    if (!decision.lock) {
      arm();
      return;
    }
    void bridge?.setAppLockRuntimeLocked?.('idle');
  };
  arm();
  return () => {
    cancelled = true;
    if (timer !== undefined) cancelTimeout(timer);
  };
}

export async function resolveUnlockAttempt(password: string): Promise<AppLockUnlockResult> {
  if (!password) return { ok: false, error: 'empty' };
  try {
    return await lemonsshBridge.get()?.requestAppLockUnlock?.(password) ?? { ok: false, error: 'incorrect' };
  } catch {
    return { ok: false, error: 'incorrect' };
  }
}

export function createOptimisticUnlockedRuntimeState(
  input: RuntimeAppLockState,
  now: number,
): RuntimeAppLockState {
  const current = normalizeRuntimeAppLockState(input);
  if (current.initialized && !current.locked && current.reason === null) {
    return current;
  }

  return {
    ...current,
    initialized: true,
    locked: false,
    reason: null,
    // Keep the observed main-process version. Bumping here can outrank a real
    // concurrent re-lock and leave the renderer unlocked after refresh (Codex P2).
    version: current.version,
    lastUnlockedAt: now,
    lastActivityAt: now,
  };
}

export function useAppLockState(settings: AppLockSettings) {
  const normalizedSettings = useMemo(() => normalizeAppLockSettings(settings), [settings]);
  const systemUnlockRefreshKey = `${normalizedSettings.enabled}:${normalizedSettings.systemUnlockEnabled}:${Boolean(normalizedSettings.passwordVerifier)}`;
  const bridge = lemonsshBridge.get();
  const { runtimeState, refreshRuntimeState, setRuntimeState } = useAppLockRuntime(bridge);
  const [systemUnlockStatus, setSystemUnlockStatus] = useState<AppLockSystemUnlockStatus>(
    DEFAULT_APP_LOCK_SYSTEM_UNLOCK_STATUS,
  );
  // Local idle clock. Renderer input only covers this window, so the idle
  // watchdog below always re-checks against the Go-side cross-window clock
  // (every window's reportAppLockActivity lands there) before locking.
  const lastActivityRef = useRef<number>(Date.now());
  const normalizedRuntimeState = useMemo(
    () => normalizeRuntimeAppLockState(runtimeState),
    [runtimeState],
  );
  const effectiveRuntimeState = useMemo(() => {
    const resolved = resolveAppLockRuntimeWhenOwnerMissing(
      normalizedRuntimeState,
      Boolean(bridge && 'getAppLockRuntimeState' in bridge),
    );
    if (resolved.initialized) return resolved;
    if (shouldLockOnStartup(normalizedSettings)) {
      return {
        ...resolved,
        locked: true,
        reason: 'startup' as const,
      };
    }
    return resolved;
  }, [bridge, normalizedRuntimeState, normalizedSettings]);

  const lockNow = useCallback((reason: AppLockReason = 'manual') => {
    if (!shouldLockOnStartup(normalizedSettings) || !reason) return;
    void bridge?.setAppLockRuntimeLocked?.(reason);
  }, [bridge, normalizedSettings]);

  // Merge the Go-side activity clock into the local one whenever a fresher
  // runtime state arrives (pull or push). Declared before the watchdog so a
  // state update lands before the next idle schedule is computed.
  useEffect(() => {
    const crossWindowActivityAt = normalizedRuntimeState.lastActivityAt;
    if (typeof crossWindowActivityAt === 'number') {
      lastActivityRef.current = Math.max(lastActivityRef.current, crossWindowActivityAt);
    }
  }, [normalizedRuntimeState]);

  const recordActivity = useCallback(() => {
    lastActivityRef.current = Date.now();
    if (effectiveRuntimeState.locked) return;
    void bridge?.reportAppLockActivity?.();
  }, [bridge, effectiveRuntimeState.locked]);

  const unlock = useCallback(async (password: string): Promise<AppLockUnlockResult> => {
    const result = await resolveUnlockAttempt(password);
    if (result.ok) {
      const unlockedAt = Date.now();
      // Unlocking proves presence: restart the local idle clock like Go does.
      lastActivityRef.current = Math.max(lastActivityRef.current, unlockedAt);
      setRuntimeState((current) => createOptimisticUnlockedRuntimeState(current, unlockedAt));
      await refreshRuntimeState().catch(() => {});
    }
    return result;
  }, [refreshRuntimeState, setRuntimeState]);

  const refreshSystemUnlockStatus = useCallback(async () => {
    try {
      const nextStatus = await bridge?.getAppLockSystemUnlockStatus?.();
      const normalized = normalizeAppLockSystemUnlockStatus(nextStatus);
      setSystemUnlockStatus(normalized);
      return normalized;
    } catch {
      setSystemUnlockStatus(DEFAULT_APP_LOCK_SYSTEM_UNLOCK_STATUS);
      return DEFAULT_APP_LOCK_SYSTEM_UNLOCK_STATUS;
    }
  }, [bridge]);

  const unlockWithSystemAuth = useCallback(async (): Promise<AppLockSystemUnlockResult> => {
    const result = normalizeAppLockSystemUnlockResult(await bridge?.requestAppLockSystemUnlock?.());
    if (result.ok) {
      const unlockedAt = Date.now();
      lastActivityRef.current = Math.max(lastActivityRef.current, unlockedAt);
      setRuntimeState((current) => createOptimisticUnlockedRuntimeState(current, unlockedAt));
      await refreshRuntimeState().catch(() => {});
    }
    await refreshSystemUnlockStatus().catch(() => {});
    return result;
  }, [bridge, refreshRuntimeState, refreshSystemUnlockStatus, setRuntimeState]);

  const reset = useCallback(async (currentPassword: string) => {
    if (typeof bridge?.requestAppLockReset !== 'function') {
      throw new Error('App Lock reset bridge is unavailable');
    }
    const result = await bridge.requestAppLockReset(currentPassword);
    if (result && typeof result === 'object' && 'ok' in result && result.ok === false) {
      throw new Error(result.error);
    }
    const unlockedAt = Date.now();
    lastActivityRef.current = Math.max(lastActivityRef.current, unlockedAt);
    setRuntimeState((current) => createOptimisticUnlockedRuntimeState(current, unlockedAt));
    await refreshRuntimeState().catch(() => {});
  }, [bridge, refreshRuntimeState, setRuntimeState]);

  useEffect(() => {
    if (!shouldLockOnStartup(normalizedSettings) && effectiveRuntimeState.locked) {
      const unlockedAt = Date.now();
      void bridge?.requestAppLockUnlock?.('')
        ?.then((result) => {
          if (result?.ok !== true) return;
          setRuntimeState((current) => createOptimisticUnlockedRuntimeState(current, unlockedAt));
          return refreshRuntimeState();
        })
        .catch(() => {});
    }
  }, [bridge, effectiveRuntimeState.locked, normalizedSettings, refreshRuntimeState, setRuntimeState]);

  useEffect(() => {
    if (!shouldLockOnStartup(normalizedSettings)) return undefined;

    const events: Array<keyof WindowEventMap> = ['pointerdown', 'keydown', 'wheel', 'touchstart', 'focus'];
    for (const eventName of events) {
      window.addEventListener(eventName, recordActivity, { passive: true });
    }

    return () => {
      for (const eventName of events) {
        window.removeEventListener(eventName, recordActivity);
      }
    };
  }, [normalizedSettings, recordActivity]);

  useEffect(() => {
    if (!shouldLockOnStartup(normalizedSettings)) return undefined;
    void bridge?.reportAppLockActivity?.();
    return undefined;
  }, [bridge, normalizedSettings]);

  // Idle auto-lock watchdog (see createIdleLockWatchdog): schedules the
  // configured timeout and requests the Go-verified "idle" lock when the
  // merged activity clock says the app went idle.
  useEffect(() => {
    if (!shouldLockOnStartup(normalizedSettings)) return undefined;
    if (effectiveRuntimeState.locked) return undefined;
    if (typeof window === 'undefined') return undefined;
    return createIdleLockWatchdog({
      settings: normalizedSettings,
      bridge,
      lastActivityRef,
      scheduleTimeout: (callback, delayMs) => window.setTimeout(callback, delayMs),
      cancelTimeout: (handle) => window.clearTimeout(handle as number),
      now: () => Date.now(),
    });
  }, [bridge, normalizedSettings, effectiveRuntimeState.locked]);

  useEffect(() => {
    void refreshSystemUnlockStatus();
  }, [refreshSystemUnlockStatus, systemUnlockRefreshKey]);

  return {
    initialized: effectiveRuntimeState.initialized,
    locked: effectiveRuntimeState.locked,
    lockReason: effectiveRuntimeState.reason,
    lockNow,
    unlock,
    unlockWithSystemAuth,
    systemUnlockStatus,
    refreshSystemUnlockStatus,
    reset,
    recordActivity,
    resync: refreshRuntimeState,
  };
}
