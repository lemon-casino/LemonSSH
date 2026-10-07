/* eslint-disable @typescript-eslint/no-explicit-any */

import {
  SYNC_STORAGE_KEYS,
  type CloudProvider,
  type ConvergentProviderBaselineV2,
  type ConvergentReplicaRecordV2,
  type MasterKeyConfig,
} from '../../../domain/sync';
import {
  assertValidConvergentSyncState,
  canonicalizeConvergentSyncState,
  stripConvergentSyncEnvelope,
} from '../../../domain/convergentSync';
import {
  decryptLocalStorageValue,
  encryptLocalStorageValue,
} from './encryptedLocalStorage';
import { commitHostProfileTransaction, flushHostProfileWrites, getHostProfileRevision, hasHostProfileClient, hostStorageAdapter } from '../../persistence/hostStorageAdapter';

/** Built-ins plus any dynamic plugin providers currently in manager state. */
function listedProviders(manager: any): CloudProvider[] {
  const fromState = Object.keys(manager?.state?.providers ?? {}) as CloudProvider[];
  if (fromState.length > 0) return fromState.sort();
  return ['github', 'google', 'onedrive', 'webdav', 's3'];
}

export function convergentProviderBaselineKeyImpl(this: any, provider: CloudProvider): string {
  return `${SYNC_STORAGE_KEYS.CONVERGENT_PROVIDER_BASELINE}_${provider}`;
}

function requireLocalEncryptionKey(manager: any): CryptoKey {
  const key = manager.state.unlockedKey?.derivedKey;
  if (!key) throw new Error('Convergent sync encryption key is unavailable');
  return key;
}

export async function saveConvergentReplicaImpl(
  this: any,
  record: ConvergentReplicaRecordV2,
): Promise<void> {
  if (record.schemaVersion !== 2) throw new Error('Unsupported convergent replica schema');
  assertValidConvergentSyncState(record.state);
  const normalized: ConvergentReplicaRecordV2 = {
    schemaVersion: 2,
    state: canonicalizeConvergentSyncState(record.state),
    updatedAt: record.updatedAt,
  };
  const key = requireLocalEncryptionKey(this);
  const encoded = await encryptLocalStorageValue(normalized, key);
  if (requireLocalEncryptionKey(this) !== key) throw new Error('Master key changed before replica persistence');
  if (this.saveToStorage(
    SYNC_STORAGE_KEYS.CONVERGENT_REPLICA,
    encoded,
  ) === false) throw new Error('Unable to persist convergent sync replica');
  await flushHostProfileWrites();
}

export async function loadConvergentReplicaImpl(this: any): Promise<ConvergentReplicaRecordV2 | null> {
  const encoded = this.loadFromStorage(SYNC_STORAGE_KEYS.CONVERGENT_REPLICA) as unknown;
  if (!encoded) return null;
  if (typeof encoded !== 'string') throw new Error('Convergent replica record is invalid');
  const record = await decryptLocalStorageValue<ConvergentReplicaRecordV2>(
    encoded,
    requireLocalEncryptionKey(this),
  );
  if (record?.schemaVersion !== 2 || !Number.isFinite(record.updatedAt)) {
    throw new Error('Convergent replica record has an unsupported schema');
  }
  assertValidConvergentSyncState(record.state);
  return {
    schemaVersion: 2,
    state: canonicalizeConvergentSyncState(record.state),
    updatedAt: record.updatedAt,
  };
}

export async function saveConvergentProviderBaselineImpl(
  this: any,
  baseline: ConvergentProviderBaselineV2,
): Promise<void> {
  if (baseline.schemaVersion !== 2) throw new Error('Unsupported convergent baseline schema');
  assertValidConvergentSyncState(baseline.state);
  const normalized: ConvergentProviderBaselineV2 = {
    ...baseline,
    materializedPayload: stripConvergentSyncEnvelope(baseline.materializedPayload),
    state: canonicalizeConvergentSyncState(baseline.state),
  };
  const key = requireLocalEncryptionKey(this);
  const encoded = await encryptLocalStorageValue(normalized, key);
  if (requireLocalEncryptionKey(this) !== key) throw new Error('Master key changed before baseline persistence');
  if (this.saveToStorage(
    this.convergentProviderBaselineKey(baseline.provider),
    encoded,
  ) === false) throw new Error(`Unable to persist convergent baseline for ${baseline.provider}`);
  await flushHostProfileWrites();
}

export async function loadConvergentProviderBaselineImpl(
  this: any,
  provider: CloudProvider,
): Promise<ConvergentProviderBaselineV2 | null> {
  const encoded = this.loadFromStorage(this.convergentProviderBaselineKey(provider)) as unknown;
  if (!encoded) return null;
  if (typeof encoded !== 'string') throw new Error(`Convergent baseline for ${provider} is invalid`);
  const baseline = await decryptLocalStorageValue<ConvergentProviderBaselineV2>(
    encoded,
    requireLocalEncryptionKey(this),
  );
  if (baseline?.schemaVersion !== 2 || baseline.provider !== provider) {
    throw new Error(`Convergent baseline for ${provider} has an unsupported schema`);
  }
  assertValidConvergentSyncState(baseline.state);
  return {
    ...baseline,
    materializedPayload: stripConvergentSyncEnvelope(baseline.materializedPayload),
    state: canonicalizeConvergentSyncState(baseline.state),
  };
}

export function clearConvergentSyncStorageImpl(this: any, confirmed: boolean): void {
  if (!confirmed) throw new Error('Explicit confirmation is required to remove convergent sync state');
  this.removeFromStorage(SYNC_STORAGE_KEYS.CONVERGENT_REPLICA);
  for (const provider of listedProviders(this)) {
    this.removeFromStorage(this.convergentProviderBaselineKey(provider));
  }
}

function encryptedSyncStorageKeys(manager: any): string[] {
  const keys = new Set<string>([
    manager.syncBaseKey(),
    manager.syncSnapshotsKey(),
    SYNC_STORAGE_KEYS.CONVERGENT_REPLICA,
  ]);
  for (const provider of listedProviders(manager)) {
    keys.add(manager.syncBaseKey(provider));
    keys.add(manager.syncSnapshotsKey(provider));
    keys.add(manager.convergentProviderBaselineKey(provider));
  }
  // Retain dormant provider baselines too, including unavailable plugins.
  const persistedKeys = hasHostProfileClient() || typeof globalThis.localStorage !== 'undefined' ? hostStorageAdapter.keys() : [];
  for (const key of persistedKeys) {
    if (key.startsWith(`${manager.syncBaseKey()}_`) || key.startsWith(`${manager.syncSnapshotsKey()}_`) || key.startsWith(`${SYNC_STORAGE_KEYS.CONVERGENT_PROVIDER_BASELINE}_`)) keys.add(key);
  }
  return [...keys];
}

/**
 * Re-encrypt every derived-key local record before committing the new master
 * configuration. Values are prepared first, concurrent changes abort the
 * transaction, and any write failure rolls all keys back to their exact prior
 * ciphertext.
 */
export async function reencryptSyncStorageImpl(
  this: any,
  oldKey: CryptoKey,
  newKey: CryptoKey,
  newConfig: MasterKeyConfig,
): Promise<void> {
  await flushHostProfileWrites();
  const keys = encryptedSyncStorageKeys(this);
  const expectedRevision = getHostProfileRevision();
  const securityGeneration = this.getSyncSecurityGeneration?.();
  const previousConfig = (
    this.loadFromStorage(SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG)
    ?? this.state.masterKeyConfig
  ) as MasterKeyConfig | null;
  const originals = new Map<string, string | null>();
  const replacements = new Map<string, string>();
  for (const key of keys) {
    const encoded = this.loadFromStorage(key) as unknown;
    if (encoded == null) {
      originals.set(key, null);
      continue;
    }
    if (typeof encoded !== 'string') throw new Error(`Encrypted sync record ${key} is invalid`);
    originals.set(key, encoded);
    const value = await decryptLocalStorageValue<unknown>(encoded, oldKey);
    replacements.set(key, await encryptLocalStorageValue(value, newKey));
  }
  for (const [key, original] of originals) {
    const current = this.loadFromStorage(key) as unknown;
    if ((current ?? null) !== original) {
      throw new Error('Sync data changed while the master key was being rotated');
    }
  }
  const currentConfig = this.loadFromStorage(
    SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG,
  ) as MasterKeyConfig | null;
  if (JSON.stringify(currentConfig) !== JSON.stringify(previousConfig)) {
    throw new Error('Master key configuration changed while it was being rotated');
  }
  const expected = new Map([...originals].map(([key, value]) => [key, value === null ? null : JSON.stringify(value)]));
  expected.set(SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG, previousConfig === null ? null : JSON.stringify(previousConfig));
  const next = new Map([...replacements].map(([key, value]) => [key, JSON.stringify(value)]));
  next.set(SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG, JSON.stringify(newConfig));
  if (this.getSyncSecurityGeneration?.() !== securityGeneration) throw new Error('Master key rotation was interrupted before commit');
  if (await commitHostProfileTransaction(expected, next, expectedRevision)) return;

  // Electron's synchronous storage path retains exact-ciphertext rollback.
  const committed: string[] = [];
  try {
    for (const [key, replacement] of replacements) {
      if (this.saveToStorage(key, replacement) === false) {
        throw new Error(`Unable to persist re-encrypted sync record: ${key}`);
      }
      committed.push(key);
    }
    if (this.saveToStorage(SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG, newConfig) === false) {
      throw new Error('Unable to persist the new master key configuration');
    }
  } catch (error) {
    const rollbackErrors: unknown[] = [];
    for (const key of committed.reverse()) {
      const original = originals.get(key);
      try {
        if (original !== undefined && this.saveToStorage(key, original) === false) throw new Error(`Unable to roll back ${key}`);
      } catch (rollbackError) { rollbackErrors.push(rollbackError); }
    }
    try {
      if (previousConfig) {
        if (this.saveToStorage(SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG, previousConfig) === false) throw new Error('Unable to roll back master key configuration');
      } else this.removeFromStorage(SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG);
    } catch (rollbackError) { rollbackErrors.push(rollbackError); }
    if (rollbackErrors.length) throw new AggregateError([error, ...rollbackErrors], 'Master key rotation rollback failed; sync must remain locked');
    throw error;
  }
}
