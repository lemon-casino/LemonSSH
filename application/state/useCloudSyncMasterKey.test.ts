import assert from 'node:assert/strict';
import test from 'node:test';
import { updateCloudSyncMasterKey } from './useCloudSyncMasterKey';
import type { CloudSyncHook } from './useCloudSync';
import type { SyncPayload } from '../../domain/sync';

const payload: SyncPayload = {hosts:[],keys:[],snippets:[],customGroups:[],syncedAt:123};

test('key update action rejects failed upload and missing results instead of returning success', async () => {
  const calls: string[] = [];
  const sync = {
    providers:{github:{provider:'github',status:'connected',tokens:{accessToken:'fixture'}}},
    changeMasterKey:async () => {calls.push('local');return true;},
    propagateMasterKeyRotation:async () => {calls.push('propagate');},
    syncNow:async () => {calls.push('sync');return new Map([['github',{provider:'github',success:false,action:'none',error:'upload failed'}]]);},
  } as unknown as CloudSyncHook;
  await assert.rejects(updateCloudSyncMasterKey(sync,'old','new',payload,undefined),/upload failed/);
  assert.deepEqual(calls,['local','propagate','sync']);
  sync.syncNow = async () => new Map();
  await assert.rejects(updateCloudSyncMasterKey(sync,'old','new',payload,undefined),/did not complete/);
  sync.syncNow = async () => new Map([['github',{provider:'github',success:true,action:'upload'}]]);
  assert.equal(await updateCloudSyncMasterKey(sync,'old','new',payload,undefined),true);
});

test('failed local key check or commit never starts remote propagation', async () => {
  let remote = false;
  const sync = {
    providers:{}, changeMasterKey:async () => false,
    propagateMasterKeyRotation:async () => {remote=true;}, syncNow:async () => {remote=true;return new Map();},
  } as unknown as CloudSyncHook;
  assert.equal(await updateCloudSyncMasterKey(sync,'old','new',payload,undefined),false);
  sync.changeMasterKey = async () => {throw new Error('disk failed');};
  await assert.rejects(updateCloudSyncMasterKey(sync,'old','new',payload,undefined),/disk failed/);
  assert.equal(remote,false);
});
