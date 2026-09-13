import assert from 'node:assert/strict';
import { test } from 'node:test';
import { subscribePopupConfig } from './popupConfigSubscription';
import { createWailsRuntimeClient, type WailsBindingDeps } from './wailsRuntimeClient';

const tick = () => new Promise<void>(resolve => setImmediate(resolve));

test('runtime popup subscription uses URL config RPC without registering a global config listener', async () => {
  const original = Object.getOwnPropertyDescriptor(globalThis, 'window');
  Object.defineProperty(globalThis, 'window', { configurable: true, value: { location: { search: '?popupId=own&popupToken=capability' } } });
  const delivered: unknown[] = [];
  const identities: string[][] = [];
  const client = createWailsRuntimeClient({
    terminal: {}, sftp: {},
    events: { On: () => { throw new Error('global config subscription forbidden'); } },
    popup: {
      Open: async () => ({ success: true }),
      GetConfig: async (id: string, token: string) => { identities.push([id, token]); return { hostId: 'own-host' }; },
      Heartbeat: async (id: string, token: string) => { identities.push([id, token]); },
    },
  } as unknown as WailsBindingDeps);
  const dispose = client.transitionBridge.onTerminalPopupConfig!(config => delivered.push(config));
  try {
    await tick();
    assert.deepEqual(identities, [['own', 'capability'], ['own', 'capability']]);
    assert.deepEqual(delivered, [{ hostId: 'own-host' }]);
  } finally {
    dispose?.();
    if (original) Object.defineProperty(globalThis, 'window', original);
    else Reflect.deleteProperty(globalThis, 'window');
  }
});

test('popup config requires both URL identity fields and never subscribes to global config', async () => {
  for (const search of ['', '?popupId=id', '?popupToken=token']) {
    let calls = 0;
    const dispose = subscribePopupConfig(search, {
      GetConfig: async () => { calls++; return {}; }, Heartbeat: async () => { calls++; },
    }, () => { throw new Error('unexpected config'); });
    await tick();
    dispose();
    assert.equal(calls, 0);
  }
});

test('wrong popup identity fails closed and clears heartbeat timer', async t => {
  t.mock.timers.enable({ apis: ['setInterval'] });
  let beats = 0;
  let delivered = 0;
  const errors: unknown[] = [];
  const dispose = subscribePopupConfig('?popupId=wrong&popupToken=wrong', {
    GetConfig: async (id, token) => { assert.equal(id, 'wrong'); assert.equal(token, 'wrong'); throw new Error('invalid identity'); },
    Heartbeat: async () => { beats++; },
  }, () => { delivered++; }, error => errors.push(error));
  await tick();
  t.mock.timers.tick(20000);
  await tick();
  assert.equal(delivered, 0);
  assert.equal(beats, 1);
  assert.equal(errors.length, 1);
  dispose();
});

test('heartbeat failure clears timer and suppresses pending configuration', async t => {
  t.mock.timers.enable({ apis: ['setInterval'] });
  let finish!: (config: unknown) => void;
  let beats = 0;
  const received: unknown[] = [];
  const dispose = subscribePopupConfig('?popupId=id&popupToken=token', {
    GetConfig: () => new Promise(resolve => { finish = resolve; }),
    Heartbeat: async () => { beats++; throw new Error('lease expired'); },
  }, config => received.push(config), () => undefined);
  await tick();
  finish({ hostId: 'private' });
  await tick();
  t.mock.timers.tick(20000);
  assert.equal(beats, 1);
  assert.deepEqual(received, []);
  dispose();
});

test('unsubscribe suppresses pending config and releases timer; valid identity receives only its response', async t => {
  t.mock.timers.enable({ apis: ['setInterval'] });
  let finish!: (config: unknown) => void;
  let beats = 0;
  const received: unknown[] = [];
  const bindings = {
    GetConfig: (id: string, token: string) => {
      assert.equal(id, 'id'); assert.equal(token, 'token');
      return new Promise(resolve => { finish = resolve; });
    },
    Heartbeat: async () => { beats++; },
  };
  const dispose = subscribePopupConfig('?popupId=id&popupToken=token', bindings, config => received.push(config));
  dispose();
  finish({ secret: 'late config' });
  await tick();
  t.mock.timers.tick(20000);
  assert.deepEqual(received, []);
  assert.equal(beats, 1);
  const second = subscribePopupConfig('?popupId=id&popupToken=token', bindings, config => received.push(config));
  finish({ hostId: 'own-host' });
  await tick();
  assert.deepEqual(received, [{ hostId: 'own-host' }]);
  second();
});
