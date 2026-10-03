import assert from 'node:assert/strict';
import test from 'node:test';

import { WebDAVAdapter } from './WebDAVAdapter.ts';
import {
  getActiveRuntimeClient,
  setActiveRuntimeClient,
  type RuntimeClient,
} from '../../runtime/runtimeClient';
import type { WebDAVConfig } from '../../../domain/sync';

const CONFIG: WebDAVConfig = {
  endpoint: 'https://dav.example.com/dav/',
  authType: 'token',
  token: 'tok-123',
  username: 'lemon',
  password: 'unused-for-token',
};

// #2223 regression fixture: a complete synced file envelope.
const SYNCED_FILE = {
  meta: { version: 1, updatedAt: 1_700_000_000_000, kdf: 'PBKDF2' },
  payload: 'eA==',
  etag: 'etag-1',
};

// WebDAVAdapter must drive the runtime client's sync port under Wails (the Go
// CloudSyncWebdav* transport), never the aggregate bridge (which lacks those
// methods) and never the CORS-limited renderer webdav client.
test('WebDAV adapter drives the native sync port under Wails', async () => {
  const previousWindow = globalThis.window;
  const previousClient = getActiveRuntimeClient();
  const calls: string[] = [];
  const seenConfigs: unknown[] = [];

  Object.assign(globalThis, { window: { _wails: {} } });
  setActiveRuntimeClient({
    sync: {
      cloudSyncWebdavInitialize: async (config: unknown) => {
        calls.push('initialize');
        seenConfigs.push(config);
        return { resourceId: 'etag-init' };
      },
      cloudSyncWebdavUpload: async (config: unknown, file: unknown) => {
        calls.push('upload');
        seenConfigs.push(config);
        assert.deepEqual(file, SYNCED_FILE);
        return { resourceId: 'etag-put' };
      },
      cloudSyncWebdavDownload: async (config: unknown) => {
        calls.push('download');
        seenConfigs.push(config);
        return { syncedFile: SYNCED_FILE };
      },
      cloudSyncWebdavDelete: async (config: unknown) => {
        calls.push('delete');
        seenConfigs.push(config);
        return { ok: true as const };
      },
    },
    transitionBridge: {},
  } as unknown as RuntimeClient);

  try {
    const adapter = new WebDAVAdapter({ ...CONFIG });
    assert.equal(await adapter.initializeSync(), 'etag-init');
    assert.equal(await adapter.upload({ ...SYNCED_FILE }), 'etag-put');
    assert.deepEqual(await adapter.download(), SYNCED_FILE);
    await adapter.deleteSync();
    assert.deepEqual(calls, ['initialize', 'upload', 'download', 'delete']);
    // The full renderer config (auth type, token, credentials) reaches the Go
    // transport verbatim — including optional fields digest/token auth need.
    for (const seen of seenConfigs) {
      assert.deepEqual(seen, { ...CONFIG });
    }
  } finally {
    Object.assign(globalThis, { window: previousWindow });
    setActiveRuntimeClient(previousClient);
  }
});

// Wails has no renderer-fetch fallback: a missing sync port method must fail
// loudly instead of silently switching to the CORS-limited webdav client.
test('WebDAV adapter refuses to fall back when the native method is missing', async () => {
  const previousWindow = globalThis.window;
  const previousClient = getActiveRuntimeClient();
  Object.assign(globalThis, { window: { _wails: {} } });
  setActiveRuntimeClient({ sync: {}, transitionBridge: {} } as unknown as RuntimeClient);
  try {
    const adapter = new WebDAVAdapter({ ...CONFIG });
    await assert.rejects(adapter.initializeSync(), /Native cloud sync method unavailable/);
    await assert.rejects(adapter.upload({ ...SYNCED_FILE }), /Native cloud sync method unavailable/);
    await assert.rejects(adapter.download(), /Native cloud sync method unavailable/);
  } finally {
    Object.assign(globalThis, { window: previousWindow });
    setActiveRuntimeClient(previousClient);
  }
});

// Outside Wails the bridge resolves to the aggregate bridge, which lacks the
// native methods, so the adapter falls back to its renderer client.
test('WebDAV adapter keeps its renderer fallback outside Wails', async () => {
  const previousWindow = globalThis.window;
  Reflect.deleteProperty(globalThis, 'window');
  try {
    const adapter = new WebDAVAdapter({
      endpoint: 'https://localhost:9/dav',
      authType: 'token',
      token: 'tok',
    });
    // No server is reachable in tests; the point is that the failure comes
    // from the webdav client (network), not from a bridge availability error.
    await assert.rejects(
      adapter.initializeSync(),
      (error: unknown) => !/Native cloud sync method unavailable/.test(String(error)),
    );
  } finally {
    Object.assign(globalThis, { window: previousWindow });
  }
});
