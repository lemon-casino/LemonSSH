import assert from 'node:assert/strict';
import test from 'node:test';
import { SYNC_STORAGE_KEYS, type MasterKeyConfig } from '../../../domain/sync';
import { configureHostProfileClient, hydrateHostProfile, hostStorageAdapter, flushHostProfileWrites } from '../../persistence/hostStorageAdapter';
import type { ProfileClient } from '../../runtime/profile/profileClient';
import { changeMasterKeyImpl } from './stateAndSecurityMethods';
import { reencryptSyncStorageImpl } from './convergentSyncStorageMethods';
import { encryptLocalStorageValue, decryptLocalStorageValue } from './encryptedLocalStorage';
import { EncryptionService } from '../EncryptionService';
import { withSyncOperation } from './syncOperationLock';

test('canonical rotation failure preserves every ciphertext; successful CAS precedes active key publication', async (t) => {
  const previousStorage = Object.getOwnPropertyDescriptor(globalThis,'localStorage');
  const local = new Map<string,string>();
  Object.defineProperty(globalThis,'localStorage',{configurable:true,value:{
    get length() { return local.size; }, key:(index: number) => [...local.keys()][index] ?? null,
    getItem:(key: string) => local.get(key) ?? null,
    setItem:(key: string,value: string) => {local.set(key,value);}, removeItem:(key: string) => {local.delete(key);},
  }});
  const data = new Map<string,string>();
  let revision = 0;
  let fail = false;
  let hold: Promise<void> | undefined;
  let began: (() => void) | undefined;
  const client: ProfileClient = {
    revision:async () => revision, domains:async () => ['settings','vault','sessions'],
    domainKeys:async domain => [...data.keys()].filter(key => key.startsWith(domain+'/')).map(key => key.slice(domain.length+1)),
    getRawBase64:async (domain,key) => data.get(domain+'/'+key),
    setRawBase64:async () => {throw new Error('nontransactional write');}, deleteRaw:async () => {throw new Error('nontransactional delete');},
    write:async (expected,mutations) => {
      began?.(); await hold;
      if (fail) throw new Error('durable write failed');
      if (expected !== revision) throw new Error('profile revision conflict');
      for (const mutation of mutations) {
        const key = mutation.domain+'/'+mutation.key;
        if (mutation.delete) data.delete(key); else data.set(key,mutation.valueBase64!);
      }
      return {revision:++revision};
    },
  };
  const oldKey = await crypto.subtle.generateKey({name:'AES-GCM',length:256},true,['encrypt','decrypt']);
  const newKey = await crypto.subtle.generateKey({name:'AES-GCM',length:256},true,['encrypt','decrypt']);
  const oldConfig: MasterKeyConfig = {verificationHash:'old',salt:'old',kdf:'PBKDF2',createdAt:123};
  const newConfig = {...oldConfig,salt:'new',verificationHash:'new'};
  const records = [SYNC_STORAGE_KEYS.CONVERGENT_REPLICA, `${SYNC_STORAGE_KEYS.CONVERGENT_PROVIDER_BASELINE}_github`, `${SYNC_STORAGE_KEYS.SYNC_BASE_PAYLOAD}_github`, 'netcatty_sync_snapshots_v1_github', `${SYNC_STORAGE_KEYS.CONVERGENT_PROVIDER_BASELINE}_dormant.plugin`];
  try {
    configureHostProfileClient(client); await hydrateHostProfile();
    hostStorageAdapter.write(SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG,oldConfig);
    for (const key of records) hostStorageAdapter.write(key,await encryptLocalStorageValue({lineage:key,secret:'fixture'},oldKey));
    await flushHostProfileWrites();
    const original = new Map(data);
    const subject = {
      state:{masterKeyConfig:oldConfig,securityState:'UNLOCKED',unlockedKey:{derivedKey:oldKey},autoSyncEnabled:false},
      masterPassword:'old-password',
      loadFromStorage:hostStorageAdapter.read,
      syncBaseKey:(provider?: string) => `${SYNC_STORAGE_KEYS.SYNC_BASE_PAYLOAD}${provider ? '_'+provider : ''}`,
      syncSnapshotsKey:(provider?: string) => `netcatty_sync_snapshots_v1${provider ? '_'+provider : ''}`,
      convergentProviderBaselineKey:(provider: string) => `${SYNC_STORAGE_KEYS.CONVERGENT_PROVIDER_BASELINE}_${provider}`,
      reencryptSyncStorage:async (old: CryptoKey,next: CryptoKey,config: MasterKeyConfig) => reencryptSyncStorageImpl.call(subject,old,next,config),
      emit:() => {},
    };
    t.mock.method(EncryptionService,'changeMasterPassword',async () => newConfig);
    t.mock.method(EncryptionService,'unlockMasterKey',async (password: string) => ({derivedKey:password === 'old-password' ? oldKey : newKey,salt:new Uint8Array(),unlockedAt:123}));
    fail = true;
    await assert.rejects(changeMasterKeyImpl.call(subject,'old-password','new-password'),/durable write failed/);
    assert.deepEqual(data,original);
    assert.equal(subject.masterPassword,'old-password');
    assert.equal(subject.state.unlockedKey.derivedKey,oldKey);
    fail = false;
    let release!: () => void;
    hold = new Promise<void>(resolve => {release=resolve;});
    const started = new Promise<void>(resolve => {began=resolve;});
    const rotation = changeMasterKeyImpl.call(subject,'old-password','new-password');
    await started;
    assert.equal(subject.masterPassword,'old-password');
    assert.deepEqual(data,original);
    release(); await rotation;
    assert.equal(subject.masterPassword,'new-password');
    for (const key of records) {
      const encoded = hostStorageAdapter.read<string>(key)!;
      assert.deepEqual(await decryptLocalStorageValue(encoded,newKey),{lineage:key,secret:'fixture'});
      await assert.rejects(decryptLocalStorageValue(encoded,oldKey));
    }
  } finally {
    configureHostProfileClient(undefined);
    if (previousStorage) Object.defineProperty(globalThis,'localStorage',previousStorage); else Reflect.deleteProperty(globalThis,'localStorage');
  }
});

test('sync operations serialize rotation behind active work and reject stale window key', async () => {
  const read = hostStorageAdapter.read;
  const config: MasterKeyConfig = {verificationHash:'old',salt:'salt',kdf:'PBKDF2',createdAt:1};
  let current = config;
  hostStorageAdapter.read = (() => current) as typeof hostStorageAdapter.read;
  const order: string[] = [];
  let release!: () => void;
  const gate = new Promise<void>(resolve => {release=resolve;});
  const owner = {
    getState:() => ({masterKeyConfig:config}), lock:() => {order.push('locked');},
    adoptStoredMasterKeyConfig:() => {order.push('locked');},
  };
  try {
    const first = withSyncOperation(owner,async () => {order.push('sync'); await gate; order.push('synced');});
    const second = withSyncOperation(owner,async () => {order.push('rotate');});
    await new Promise(resolve => setImmediate(resolve));
    assert.deepEqual(order,['sync']); release(); await Promise.all([first,second]);
    assert.deepEqual(order,['sync','synced','rotate']);
    current = {...config,salt:'new'};
    await assert.rejects(withSyncOperation(owner,async () => {order.push('unsafe');}),/another window/);
    assert.equal(order.includes('unsafe'),false); assert.equal(order.at(-1),'locked');
  } finally {hostStorageAdapter.read = read;}
});
