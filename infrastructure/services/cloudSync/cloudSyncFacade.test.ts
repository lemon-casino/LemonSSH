import assert from 'node:assert/strict';
import test from 'node:test';
import { createCloudOAuthFacade, type CloudOAuthBindings } from './cloudSyncFacade';
import { getActiveRuntimeClient, setActiveRuntimeClient, type RuntimeClient } from '../../runtime/runtimeClient';
import * as github from '../adapters/GitHubAdapter';
import * as google from '../adapters/GoogleDriveAdapter';
import * as onedrive from '../adapters/OneDriveAdapter';
import { saveProviderConnectionImpl, initProviderDecryptionImpl } from './stateAndSecurityMethods';

test('native facade preserves callback arguments, refresh token rotation, JSON null and errors', async () => {
  const calls: unknown[] = [];
  const bindings = {
    PrepareOAuthCallback: async () => ({ sessionId: 's', port: 1234, redirectUri: 'http://127.0.0.1:1234/callback' }),
    AwaitOAuthCallback: async (...args: string[]) => { calls.push(args); return { code: 'c', state: 'state' }; },
    CancelOAuthCallback: async (id: string) => { calls.push(id); },
    GoogleRefreshAccessToken: async () => ({ accessToken: 'new', tokenType: 'Bearer' }),
    OnedriveRefreshAccessToken: async () => ({ accessToken: 'new', refreshToken: 'rotated', tokenType: 'Bearer' }),
    GoogleDriveCreateSyncFile: async () => ({ fileId: null }),
    GithubDeleteSyncFile: async () => ({ ok: false }),
    GithubDownloadSyncFile: async () => ({ syncedFile: null }),
  } as unknown as CloudOAuthBindings;
  const facade = createCloudOAuthFacade(bindings);
  assert.equal((await facade.prepareOAuthCallback()).port, 1234);
  await facade.awaitOAuthCallback('state', 's'); await facade.cancelOAuthCallback('s');
  assert.deepEqual(calls, [['state','s'],'s']);
  assert.equal((await facade.googleRefreshAccessToken({ clientId: 'c', refreshToken: 'old' })).refreshToken, 'old');
  assert.equal((await facade.onedriveRefreshAccessToken({ clientId: 'c', refreshToken: 'old' })).refreshToken, 'rotated');
  assert.deepEqual(await facade.githubDownloadSyncFile({ accessToken: 't' }), { syncedFile: null });
  await assert.rejects(facade.googleDriveCreateSyncFile({ accessToken:'t', syncedFile:{} }), /file ID/);
  await assert.rejects(facade.githubDeleteSyncFile({ accessToken:'t', fileId:'f' }), /did not complete/);
});

test('Wails provider calls use sync port, including GitHub user and gist REST', async () => {
  const previousWindow = globalThis.window;
  const previousClient = getActiveRuntimeClient();
  const previousFetch = globalThis.fetch;
  let fetches = 0;
  const calls: string[] = [];
  const file = { meta: { version: 1 }, payload: 'eA==', etag: 'etag' };
  Object.assign(globalThis, { window: { _wails: {} } });
  globalThis.fetch = async () => { fetches++; throw new Error('renderer fetch forbidden'); };
  setActiveRuntimeClient({ sync: {
    githubGetUserInfo: async () => { calls.push('user'); return { id:'u', name:'name', email:'e' }; },
    githubFindSyncFile: async () => { calls.push('find'); return { fileId:'g' }; },
    githubDownloadSyncFile: async () => { calls.push('download'); return { syncedFile:file }; },
    googleDriveDownloadSyncFile: async () => ({ syncedFile:file }),
    onedriveDownloadSyncFile: async () => ({ syncedFile:file }),
  }, transitionBridge: {} } as unknown as RuntimeClient);
  try {
    assert.equal((await github.getUserInfo('t')).id,'u');
    assert.equal(await github.findSyncGist('t'),'g');
    assert.deepEqual(await github.downloadSyncGist('t','g'),file);
    assert.deepEqual(await google.downloadSyncFile('t','f'),file);
    assert.deepEqual(await onedrive.downloadSyncFile('t','f'),file);
    assert.deepEqual(calls,['user','find','download']);
    setActiveRuntimeClient({ sync: {}, transitionBridge: {} } as unknown as RuntimeClient);
    await assert.rejects(github.startDeviceFlow(), /Native cloud sync method unavailable/);
    await assert.rejects(github.getUserInfo('t'), /Native cloud sync method unavailable/);
    await assert.rejects(google.getUserInfo('t'), /Native cloud sync method unavailable/);
    await assert.rejects(onedrive.exchangeCodeForTokens('c','v','u'), /Native cloud sync method unavailable/);
    assert.equal(fetches,0);
  } finally {
    Object.assign(globalThis,{window:previousWindow}); setActiveRuntimeClient(previousClient); globalThis.fetch = previousFetch;
  }
});

test('Wails refuses plaintext credential persistence and leaves unopened tokens unavailable', async () => {
  const previousWindow = globalThis.window;
  const previousClient = getActiveRuntimeClient();
  Object.assign(globalThis,{window:{_wails:{}}});
  setActiveRuntimeClient({sync:{},transitionBridge:{credentialsAvailable:async () => false}} as unknown as RuntimeClient);
  let wrote = false;
  const connection = {provider:'github',status:'connected',tokens:{accessToken:'fixture-plaintext',tokenType:'Bearer'}};
  const subject = {
    state:{providers:{github:connection,google:{},onedrive:{},webdav:{},s3:{}}},
    providerDecryptSeq:{github:0}, providerWriteSeq:{github:0},providerDecrypted:{github:false},
    saveToStorage:() => {wrote=true;},loadFromStorage:() => null,notifyStateChange:() => {},
  };
  try {
    await assert.rejects(saveProviderConnectionImpl.call(subject,'github',connection),/Secure credential storage is unavailable/);
    await initProviderDecryptionImpl.call(subject);
    assert.equal(wrote,false);
    assert.equal(subject.providerDecrypted.github,false);
  } finally {Object.assign(globalThis,{window:previousWindow});setActiveRuntimeClient(previousClient);}
});
