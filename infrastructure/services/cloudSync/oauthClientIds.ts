/**
 * Runtime OAuth client IDs for the cloud sync providers.
 *
 * Client IDs are public values, but each user must register their own
 * desktop-app OAuth client. GitHub Device Flow and OneDrive PKCE only
 * need the ID. Google's installed desktop clients still send
 * client_secret on token exchange, stored separately. The IDs come
 * from runtime storage so users can configure them in Settings without
 * rebuilding; the build-time VITE_SYNC_*_CLIENT_ID constants are the
 * fallback for distribution-managed builds.
 */
import { SYNC_CONSTANTS } from '../../../domain/sync';
import { STORAGE_KEY_SYNC_OAUTH_CLIENT_IDS, STORAGE_KEY_SYNC_OAUTH_CLIENT_SECRETS } from '../../config/storageKeys';
import {
  hostStorageAdapter,
} from '../../persistence/hostStorageAdapter';
import { LOCAL_STORAGE_ADAPTER_CHANGED_EVENT } from '../../persistence/localStorageAdapter';

export type OAuthProvider = 'github' | 'google' | 'onedrive';

export type OAuthClientIds = Partial<Record<OAuthProvider, string>>;

const STORAGE_KEY = STORAGE_KEY_SYNC_OAUTH_CLIENT_IDS;
const SECRET_STORAGE_KEY = STORAGE_KEY_SYNC_OAUTH_CLIENT_SECRETS;

// useSyncExternalStore compares getSnapshot results by reference; return the
// cached object and only replace it when the state actually changes, or React
// loops into "maximum update depth exceeded" (error #185).
let snapshot: OAuthClientIds = {};
let loaded = false;
let secretSnapshot: OAuthClientIds = {};
let secretsLoaded = false;
const listeners = new Set<() => void>();

function readStorage(): OAuthClientIds {
  try {
    const raw = hostStorageAdapter.readString(STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const ids: OAuthClientIds = {};
    for (const provider of ['github', 'google', 'onedrive'] as const) {
      const value = parsed[provider];
      if (typeof value === 'string' && value.trim()) ids[provider] = value.trim();
    }
    return ids;
  } catch {
    return {};
  }
}

function current(): OAuthClientIds {
  if (!loaded) {
    snapshot = readStorage();
    loaded = true;
  }
  return snapshot;
}

function readSecrets(): OAuthClientIds {
  try {
    const raw = hostStorageAdapter.readString(SECRET_STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const secrets: OAuthClientIds = {};
    for (const provider of ['github', 'google', 'onedrive'] as const) {
      const value = parsed[provider];
      if (typeof value === 'string' && value.trim()) secrets[provider] = value.trim();
    }
    return secrets;
  } catch {
    return {};
  }
}

function currentSecrets(): OAuthClientIds {
  if (!secretsLoaded) {
    secretSnapshot = readSecrets();
    secretsLoaded = true;
  }
  return secretSnapshot;
}

function emitChange(): void {
  for (const listener of listeners) {
    try {
      listener();
    } catch {
      // listener errors must not break the store
    }
  }
}

if (typeof window !== 'undefined') {
  window.addEventListener(LOCAL_STORAGE_ADAPTER_CHANGED_EVENT, ((event: CustomEvent<{ key: string }>) => {
    if (event.detail?.key === STORAGE_KEY) {
      snapshot = readStorage();
      loaded = true;
      emitChange();
      return;
    }
    if (event.detail?.key === SECRET_STORAGE_KEY) {
      secretSnapshot = readSecrets();
      secretsLoaded = true;
      emitChange();
    }
  }) as EventListener);
}

export function getOAuthClientIds(): OAuthClientIds {
  return { ...current() };
}

/** Stable snapshot for useSyncExternalStore; same reference until a write. */
export function getOAuthClientIdsSnapshot(): OAuthClientIds {
  return current();
}

export function setOAuthClientId(provider: OAuthProvider, clientId: string): void {
  const trimmed = clientId.trim();
  const next = { ...current() };
  if (trimmed) next[provider] = trimmed;
  else delete next[provider];
  snapshot = next;
  loaded = true;
  hostStorageAdapter.writeString(STORAGE_KEY, JSON.stringify(next));
  emitChange();
}

export function setOAuthClientSecret(provider: OAuthProvider, secret: string): void {
  const trimmed = secret.trim();
  const next = { ...currentSecrets() };
  if (trimmed) next[provider] = trimmed;
  else delete next[provider];
  secretSnapshot = next;
  secretsLoaded = true;
  hostStorageAdapter.writeString(SECRET_STORAGE_KEY, JSON.stringify(next));
  emitChange();
}

export function resolveOAuthClientSecret(provider: OAuthProvider): string {
  return currentSecrets()[provider] || '';
}

/** Stable primitive snapshot for the Google secret input (useSyncExternalStore). */
export function getGoogleClientSecretSnapshot(): string {
  return currentSecrets().google || '';
}

export function subscribeOAuthClientIds(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Re-read durable storage after a same-window Go reset (no `window.storage` event). */
export function reloadOAuthClientIdsFromStorage(): void {
  snapshot = readStorage();
  loaded = true;
  secretSnapshot = readSecrets();
  secretsLoaded = true;
  emitChange();
}

/**
 * Resolve the client ID for a provider: the runtime override wins, then the
 * build-time constant. Empty string means "not configured" — callers must
 * refuse to open the provider authorize URL in that case.
 */
export function resolveOAuthClientId(provider: OAuthProvider): string {
  const override = current()[provider];
  if (override) return override;
  if (provider === 'github') return SYNC_CONSTANTS.GITHUB_CLIENT_ID || '';
  if (provider === 'google') return SYNC_CONSTANTS.GOOGLE_CLIENT_ID || '';
  return SYNC_CONSTANTS.ONEDRIVE_CLIENT_ID || '';
}

export function requireOAuthClientId(provider: OAuthProvider): string {
  const clientId = resolveOAuthClientId(provider);
  if (!clientId) {
    throw new Error(
      `The ${provider} OAuth client ID is not configured. ` +
        'Set it in Settings → Cloud Sync → OAuth application, or build with the matching VITE_SYNC_*_CLIENT_ID variable.',
    );
  }
  return clientId;
}
