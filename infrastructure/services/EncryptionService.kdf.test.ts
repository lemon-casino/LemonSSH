import assert from 'node:assert/strict';
import test from 'node:test';

import { EncryptionService } from './EncryptionService.ts';
import type { SyncedFile, SyncPayload } from '../../domain/sync.ts';

const PAYLOAD: SyncPayload = {
  hosts: [],
  keys: [],
  snippets: [],
  customGroups: [],
  syncedAt: 1_700_000_000_000,
};

// A kdf:'Argon2id' envelope (e.g. written by a future version or an external
// tool) must fail with an explicit unsupported-KDF error instead of silently
// deriving a PBKDF2 key and reporting a misleading GCM authentication failure.
test('decryptPayload rejects Argon2id metadata with an explicit unsupported-KDF error', async () => {
  const file = await EncryptionService.encryptPayload(
    PAYLOAD,
    'correct horse battery staple',
    'device-a',
    'Device A',
    '1.0.0',
  );
  const tampered: SyncedFile = {
    meta: { ...file.meta, kdf: 'Argon2id' },
    payload: file.payload,
  };
  await assert.rejects(
    EncryptionService.decryptPayload(tampered, 'correct horse battery staple'),
    /Unsupported sync encryption KDF: Argon2id/,
  );
});

test('decrypt rejects Argon2id input before any key material is used', async () => {
  await assert.rejects(
    EncryptionService.decrypt(
      {
        ciphertext: new Uint8Array(16),
        iv: new Uint8Array(12),
        salt: new Uint8Array(32),
        kdf: 'Argon2id',
      },
      await EncryptionService.deriveKey('pass', new Uint8Array(32)),
    ),
    /Unsupported sync encryption KDF: Argon2id/,
  );
});

// Files written before the kdf field existed are always PBKDF2 and must keep
// decrypting.
test('legacy files without a kdf field remain decryptable', async () => {
  const file = await EncryptionService.encryptPayload(
    PAYLOAD,
    'correct horse battery staple',
    'device-a',
    'Device A',
    '1.0.0',
  );
  const legacy: SyncedFile = {
    meta: { ...file.meta },
    payload: file.payload,
  };
  delete (legacy.meta as { kdf?: string }).kdf;
  const decrypted = await EncryptionService.decryptPayload(
    legacy,
    'correct horse battery staple',
  );
  assert.equal(decrypted.syncedAt, PAYLOAD.syncedAt);
});
