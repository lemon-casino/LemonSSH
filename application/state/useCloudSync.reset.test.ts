import assert from 'node:assert/strict';
import test from 'node:test';
import { createElement } from 'react';
import { act, create } from 'react-test-renderer';
import { JSDOM } from 'jsdom';

import { STORAGE_KEY_SYNC_OAUTH_CLIENT_IDS } from '../../infrastructure/config/storageKeys';
import { SYNC_STORAGE_KEYS, type MasterKeyConfig } from '../../domain/sync';
import {
  configureHostProfileClient,
  hydrateHostProfile,
  hostStorageAdapter,
} from '../../infrastructure/persistence/hostStorageAdapter';
import type { ProfileClient } from '../../infrastructure/runtime/profile/profileClient';
import { setActiveRuntimeClient } from '../../infrastructure/runtime/runtimeClient';
import { resetCloudSyncManager } from '../../infrastructure/services/CloudSyncManager';

const MASTER_KEY: MasterKeyConfig = {
  verificationHash: 'reset-init-hash',
  salt: 'reset-init-salt',
  kdf: 'PBKDF2',
  createdAt: 1,
};

test('reset-init drops a mounted settings panel onto the NO_KEY master-key gate', async () => {
  const dom = new JSDOM('<html><body></body></html>', { url: 'http://localhost' });
  const originals = new Map<string, PropertyDescriptor | undefined>();
  const install = (key: string, value: unknown) => {
    originals.set(key, Object.getOwnPropertyDescriptor(globalThis, key));
    Object.defineProperty(globalThis, key, { configurable: true, value });
  };
  for (const key of ['window', 'document', 'localStorage', 'CustomEvent', 'navigator']) {
    install(key, (dom.window as Record<string, unknown>)[key]);
  }
  for (const key of ['addEventListener', 'removeEventListener', 'dispatchEvent']) {
    install(key, (dom.window as unknown as Record<string, (...args: unknown[]) => unknown>)[key].bind(dom.window));
  }
  install('IS_REACT_ACT_ENVIRONMENT', true);
  install('BroadcastChannel', undefined);
  setActiveRuntimeClient(undefined);
  resetCloudSyncManager();

  const data = new Map([[
    `settings/${SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG}`,
    Buffer.from(JSON.stringify(MASTER_KEY)).toString('base64'),
  ]]);
  let revision = 1;
  const client: ProfileClient = {
    revision: async () => revision,
    domains: async () => ['settings', 'vault', 'sessions'],
    domainKeys: async (domain) => [...data.keys()].filter((key) => key.startsWith(`${domain}/`)).map((key) => key.slice(domain.length + 1)),
    getRawBase64: async (domain, key) => data.get(`${domain}/${key}`),
    setRawBase64: async () => { throw new Error('non-CAS write'); },
    deleteRaw: async () => { throw new Error('non-CAS delete'); },
    write: async (expected, mutations) => {
      if (expected !== revision) throw new Error('profile revision conflict');
      for (const mutation of mutations) {
        const key = `${mutation.domain}/${mutation.key}`;
        if (mutation.delete) data.delete(key);
        else data.set(key, mutation.valueBase64!);
      }
      return { revision: ++revision };
    },
  };

  try {
    configureHostProfileClient(client);
    await hydrateHostProfile();
    assert.ok(hostStorageAdapter.read(SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG));

    const { useCloudSync } = await import('./useCloudSync.ts');
    let sync!: ReturnType<typeof useCloudSync>;
    function Probe() {
      sync = useCloudSync();
      return null;
    }
    let root!: ReturnType<typeof create>;
    await act(async () => {
      root = create(createElement(Probe));
    });
    assert.equal(sync.securityState, 'LOCKED');

    dom.window.netcatty = {
      cloudSyncResetEverything: async () => {
        data.delete(`settings/${SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG}`);
        revision += 1;
        return [SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG];
      },
    };

    await act(async () => {
      await sync.resetSyncEverything();
    });

    assert.equal(hostStorageAdapter.read(SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG), null);
    assert.equal(
      sync.securityState,
      'NO_KEY',
      'CloudSyncSettings only mounts GatekeeperScreen (设置主密钥 / 确认主密钥) on NO_KEY',
    );
    await act(async () => { root.unmount(); });
  } finally {
    resetCloudSyncManager();
    configureHostProfileClient(undefined);
    setActiveRuntimeClient(undefined);
    dom.window.close();
    for (const [key, descriptor] of originals) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else Reflect.deleteProperty(globalThis, key);
    }
  }
});

test('reset-init clears saved OAuth client IDs before the new master-key gate', async () => {
  const dom = new JSDOM('<html><body></body></html>', { url: 'http://localhost' });
  const originals = new Map<string, PropertyDescriptor | undefined>();
  const install = (key: string, value: unknown) => {
    originals.set(key, Object.getOwnPropertyDescriptor(globalThis, key));
    Object.defineProperty(globalThis, key, { configurable: true, value });
  };
  for (const key of ['window', 'document', 'localStorage', 'CustomEvent', 'navigator']) {
    install(key, (dom.window as Record<string, unknown>)[key]);
  }
  for (const key of ['addEventListener', 'removeEventListener', 'dispatchEvent']) {
    install(key, (dom.window as unknown as Record<string, (...args: unknown[]) => unknown>)[key].bind(dom.window));
  }
  install('IS_REACT_ACT_ENVIRONMENT', true);
  install('BroadcastChannel', undefined);
  setActiveRuntimeClient(undefined);
  resetCloudSyncManager();

  const oauthIds = JSON.stringify({
    github: 'Ov23licrO6aqtR2h1WBC',
    google: 'desktop.apps.googleusercontent.com',
    onedrive: '00000000-1111-2222-3333-444444444444',
  });
  const data = new Map([
    [`settings/${SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG}`, Buffer.from(JSON.stringify(MASTER_KEY)).toString('base64')],
    [`settings/${STORAGE_KEY_SYNC_OAUTH_CLIENT_IDS}`, Buffer.from(oauthIds).toString('base64')],
  ]);
  let revision = 1;
  const client: ProfileClient = {
    revision: async () => revision,
    domains: async () => ['settings', 'vault', 'sessions'],
    domainKeys: async (domain) => [...data.keys()].filter((key) => key.startsWith(`${domain}/`)).map((key) => key.slice(domain.length + 1)),
    getRawBase64: async (domain, key) => data.get(`${domain}/${key}`),
    setRawBase64: async () => { throw new Error('non-CAS write'); },
    deleteRaw: async () => { throw new Error('non-CAS delete'); },
    write: async (expected, mutations) => {
      if (expected !== revision) throw new Error('profile revision conflict');
      for (const mutation of mutations) {
        const key = `${mutation.domain}/${mutation.key}`;
        if (mutation.delete) data.delete(key);
        else data.set(key, mutation.valueBase64!);
      }
      return { revision: ++revision };
    },
  };

  try {
    configureHostProfileClient(client);
    await hydrateHostProfile();
    const { getOAuthClientIds, reloadOAuthClientIdsFromStorage } = await import('../../infrastructure/services/cloudSync/oauthClientIds.ts');
    reloadOAuthClientIdsFromStorage();
    assert.equal(getOAuthClientIds().github, 'Ov23licrO6aqtR2h1WBC');

    const { useCloudSync } = await import('./useCloudSync.ts');
    let sync!: ReturnType<typeof useCloudSync>;
    function Probe() {
      sync = useCloudSync();
      return null;
    }
    let root!: ReturnType<typeof create>;
    await act(async () => {
      root = create(createElement(Probe));
    });

    dom.window.netcatty = {
      cloudSyncResetEverything: async () => {
        data.delete(`settings/${SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG}`);
        data.delete(`settings/${STORAGE_KEY_SYNC_OAUTH_CLIENT_IDS}`);
        revision += 1;
        return [SYNC_STORAGE_KEYS.MASTER_KEY_CONFIG, STORAGE_KEY_SYNC_OAUTH_CLIENT_IDS];
      },
    };

    await act(async () => {
      await sync.resetSyncEverything();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    assert.equal(hostStorageAdapter.readString(STORAGE_KEY_SYNC_OAUTH_CLIENT_IDS), null);
    assert.deepEqual(getOAuthClientIds(), {});
    await act(async () => { root.unmount(); });
  } finally {
    resetCloudSyncManager();
    configureHostProfileClient(undefined);
    setActiveRuntimeClient(undefined);
    dom.window.close();
    for (const [key, descriptor] of originals) {
      if (descriptor) Object.defineProperty(globalThis, key, descriptor);
      else Reflect.deleteProperty(globalThis, key);
    }
  }
});
