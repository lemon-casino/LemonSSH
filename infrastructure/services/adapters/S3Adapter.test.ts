import assert from 'node:assert/strict';
import test from 'node:test';

import { S3Adapter } from './S3Adapter.ts';
import {
  getActiveRuntimeClient,
  setActiveRuntimeClient,
  type RuntimeClient,
} from '../../runtime/runtimeClient';
import type { S3Config } from '../../../domain/sync';

// forcePathStyle/allowInsecure/prefix ride through the bridge verbatim; the
// Go transport must own their interpretation.
const CONFIG: S3Config = {
  endpoint: 'https://s3.example.com',
  region: 'us-east-1',
  bucket: 'vault',
  accessKeyId: 'AKIAEXAMPLE',
  secretAccessKey: 'renderer-secret',
  prefix: 'backups/lemon/',
  forcePathStyle: false,
  allowInsecure: true,
};

const SYNCED_FILE = {
  meta: { version: 1, updatedAt: 1_700_000_000_000, kdf: 'PBKDF2' },
  payload: 'eA==',
  etag: 'etag-1',
};

// S3Adapter must drive the runtime client's sync port under Wails (the Go
// CloudSyncS3* transport). This also covers the allowInsecure case where the
// renderer AWS SDK client cannot be constructed at all.
test('S3 adapter drives the native sync port under Wails, including insecure configs', async () => {
  const previousWindow = globalThis.window;
  const previousClient = getActiveRuntimeClient();
  const calls: string[] = [];
  const seenConfigs: unknown[] = [];

  Object.assign(globalThis, { window: { _wails: {} } });
  setActiveRuntimeClient({
    sync: {
      cloudSyncS3Initialize: async (config: unknown) => {
        calls.push('initialize');
        seenConfigs.push(config);
        return { resourceId: 'etag-init' };
      },
      cloudSyncS3Upload: async (config: unknown, file: unknown) => {
        calls.push('upload');
        seenConfigs.push(config);
        assert.deepEqual(file, SYNCED_FILE);
        return { resourceId: 'etag-put' };
      },
      cloudSyncS3Download: async (config: unknown) => {
        calls.push('download');
        seenConfigs.push(config);
        return { syncedFile: SYNCED_FILE };
      },
      cloudSyncS3Delete: async (config: unknown) => {
        calls.push('delete');
        seenConfigs.push(config);
        return { ok: true as const };
      },
    },
    transitionBridge: {},
  } as unknown as RuntimeClient);

  try {
    const adapter = new S3Adapter({ ...CONFIG });
    // With allowInsecure the renderer SDK client is null; only the bridge
    // path can serve requests.
    assert.equal(await adapter.initializeSync(), 'etag-init');
    assert.equal(await adapter.upload({ ...SYNCED_FILE }), 'etag-put');
    assert.deepEqual(await adapter.download(), SYNCED_FILE);
    await adapter.deleteSync();
    assert.deepEqual(calls, ['initialize', 'upload', 'download', 'delete']);
    for (const seen of seenConfigs) {
      assert.deepEqual(seen, { ...CONFIG, endpoint: 'https://s3.example.com' });
    }
  } finally {
    Object.assign(globalThis, { window: previousWindow });
    setActiveRuntimeClient(previousClient);
  }
});

// Wails has no renderer AWS-SDK fallback: a missing sync port method must
// fail loudly instead of attempting a CORS-blocked browser request.
test('S3 adapter refuses to fall back when the native method is missing', async () => {
  const previousWindow = globalThis.window;
  const previousClient = getActiveRuntimeClient();
  Object.assign(globalThis, { window: { _wails: {} } });
  setActiveRuntimeClient({ sync: {}, transitionBridge: {} } as unknown as RuntimeClient);
  try {
    const adapter = new S3Adapter({ ...CONFIG });
    await assert.rejects(adapter.initializeSync(), /Native cloud sync method unavailable/);
    await assert.rejects(adapter.upload({ ...SYNCED_FILE }), /Native cloud sync method unavailable/);
    await assert.rejects(adapter.download(), /Native cloud sync method unavailable/);
  } finally {
    Object.assign(globalThis, { window: previousWindow });
    setActiveRuntimeClient(previousClient);
  }
});
