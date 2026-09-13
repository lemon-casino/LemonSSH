import assert from 'node:assert/strict';
import test from 'node:test';
import { propagateMasterKeyRotationImpl, assertMasterKeySyncSucceeded } from './masterKeyPropagation';
import { EncryptionService } from '../EncryptionService';
import type { CloudProvider, SyncedFile, SyncPayload, SyncResult } from '../../../domain/sync';
import { createConvergentSyncStateFromPayload, withConvergentSyncEnvelope } from '../../../domain/convergentSync';

test('remote rotation preserves v2 lineage; partial upload fails visibly and retry finishes', async () => {
  const base: SyncPayload = { hosts:[], keys:[], snippets:[], customGroups:[], syncedAt:123 };
  const state = createConvergentSyncStateFromPayload(base,'device',123);
  const payload: SyncPayload = JSON.parse(JSON.stringify(withConvergentSyncEnvelope(state,{syncedAt:base.syncedAt})));
  const old = await EncryptionService.encryptPayload(payload,'old-password','device','Device','1.0',4);
  const remotes = new Map<string, SyncedFile>([['github',old],['onedrive',old]]);
  let failUpload = true;
  let uploaded = 0;
  const anchors: string[] = [];
  const subject = {
    state: { providers: {
      github: {provider:'github',status:'connected',tokens:{accessToken:'t'}},
      onedrive: {provider:'onedrive',status:'connected',tokens:{accessToken:'t'}},
    }, syncState:'IDLE', pendingLocalSync:false, lastError:null as string | null },
    verifyPassword: async (password: string) => password === 'new-password',
    getSyncSecurityGeneration: () => 1,
    assertSyncSecurityGeneration: () => {},
    notifyStateChange: () => {},
    saveSyncAnchor: async (provider: string) => { anchors.push(provider); },
    getConnectedAdapter: async (provider: string) => ({
      resourceId:provider,
      download:async () => remotes.get(provider)!,
      upload:async (file: SyncedFile) => {
        if (provider === 'onedrive' && failUpload) throw new Error('upload failed');
        uploaded++; remotes.set(provider,file); return provider;
      },
    }),
  };
  await assert.rejects(propagateMasterKeyRotationImpl.call(subject,'old-password','new-password'), /Master key changed locally.*upload failed/);
  assert.equal(subject.state.syncState,'ERROR');
  assert.equal(subject.state.pendingLocalSync,true);
  assert.deepEqual(await EncryptionService.decryptPayload(remotes.get('github')!,'new-password'),payload);
  assert.deepEqual(await EncryptionService.decryptPayload(remotes.get('onedrive')!,'old-password'),payload);
  failUpload = false;
  await propagateMasterKeyRotationImpl.call(subject,'old-password','new-password');
  assert.equal(uploaded,2);
  assert.deepEqual(anchors,['github','onedrive']);
  for (const remote of remotes.values()) {
    assert.deepEqual(await EncryptionService.decryptPayload(remote,'new-password'),payload);
    assert.equal(remote.meta.version,old.meta.version);
    assert.equal(remote.meta.updatedAt,old.meta.updatedAt);
  }
});

test('key update cannot report success for failed, partial, or missing sync results', () => {
  const success: SyncResult = { provider:'github',success:true,action:'upload' };
  const results = new Map<CloudProvider,SyncResult>([['github',success]]);
  assert.doesNotThrow(() => assertMasterKeySyncSucceeded(results,['github']));
  assert.throws(() => assertMasterKeySyncSucceeded(results,['github','onedrive']),/onedrive/);
  results.set('onedrive',{provider:'onedrive',success:false,action:'none',error:'HTTP 500'});
  assert.throws(() => assertMasterKeySyncSucceeded(results,['github','onedrive']),/HTTP 500/);
  assert.throws(() => assertMasterKeySyncSucceeded(new Map(),['github']),/did not complete/);
});
