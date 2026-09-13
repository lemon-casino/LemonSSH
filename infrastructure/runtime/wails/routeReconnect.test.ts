import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createWailsRuntimeClient, type WailsBindingDeps } from './wailsRuntimeClient';
import type { OpenDataPlaneSessionOptions } from './dataPlaneSession';
const tick = () => new Promise<void>(resolve => setImmediate(resolve));

test('transport loss rotates route without restarting native process and fences stale callbacks', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const opened: OpenDataPlaneSessionOptions[] = [];
  let starts = 0, rotations = 0, closes = 0, exits = 0;
  const data: string[] = [];
  const route = (generation: number) => ({ SessionID: 's', Generation: generation, DataToken: `d${generation}`, UrgentToken: `u${generation}`, WindowBytes: 1024 });
  const bridge = createWailsRuntimeClient({
    terminal: { StartLocal: async () => { starts++; return 's'; }, Bootstrap: async () => route(1),
      Reconnect: async () => { rotations++; return route(rotations + 1); }, ListenAddr: () => '127.0.0.1:9', Close: async () => { closes++; } },
    sftp: {}, openDataPlane: (options: OpenDataPlaneSessionOptions) => { opened.push(options); return { dispose: () => undefined }; },
  } as unknown as WailsBindingDeps).transitionBridge;
  bridge.onSessionExit('s', () => { exits++; });
  bridge.onSessionData('s', chunk => data.push(chunk));
  await bridge.startLocalSession!({});
  opened[0].onDisconnect?.();
  t.mock.timers.tick(1000); await tick();
  assert.equal(opened.length, 2);
  assert.equal(opened[1].bootstrap.Generation, 2);
  assert.equal(starts, 1); assert.equal(rotations, 1); assert.equal(closes, 0); assert.equal(exits, 0);
  opened[0].onData('stale'); opened[0].onComplete?.(); opened[0].onDisconnect?.();
  opened[1].onData('current');
  assert.deepEqual(data, ['current']); assert.equal(exits, 0);
  await bridge.closeSession('s');
  opened[1].onDisconnect?.();
  t.mock.timers.tick(60000); await tick();
  assert.equal(rotations, 1); assert.equal(closes, 1);
});

test('reconnect failures use bounded retries and surface exhaustion without restarting native session', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  t.mock.method(console, 'error', () => undefined);
  let opened!: OpenDataPlaneSessionOptions;
  let attempts = 0, exits = 0;
  const messages: string[] = [];
  const bridge = createWailsRuntimeClient({
    terminal: { StartLocal: async () => 's', Bootstrap: async () => ({ SessionID: 's', Generation: 1, DataToken: 'd', UrgentToken: 'u', WindowBytes: 1024 }),
      Reconnect: async () => { attempts++; throw new Error('offline'); }, ListenAddr: () => '127.0.0.1:9', Close: async () => undefined },
    sftp: {}, openDataPlane: (options: OpenDataPlaneSessionOptions) => { opened = options; return { dispose: () => undefined }; },
  } as unknown as WailsBindingDeps).transitionBridge;
  bridge.onSessionExit('s', () => { exits++; }); bridge.onSessionData('s', chunk => messages.push(chunk));
  await bridge.startLocalSession!({}); opened.onDisconnect?.();
  for (const delay of [1000, 2000, 4000, 8000, 16000, 30000]) { t.mock.timers.tick(delay); await tick(); }
  assert.equal(attempts, 6); assert.equal(exits, 1); assert.match(messages.join(''), /could not be restored/);
  t.mock.timers.tick(60000); await tick(); assert.equal(attempts, 6);
  await bridge.closeSession('s');
});

test('closing during pending route reconnect prevents a late socket from opening', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const opened: OpenDataPlaneSessionOptions[] = [];
  let finish!: (value: unknown) => void;
  const route = { SessionID: 's', Generation: 1, DataToken: 'd', UrgentToken: 'u', WindowBytes: 1024 };
  const bridge = createWailsRuntimeClient({
    terminal: { StartLocal: async () => 's', Bootstrap: async () => route, Reconnect: () => new Promise(resolve => { finish = resolve; }), ListenAddr: () => '127.0.0.1:9', Close: async () => undefined },
    sftp: {}, openDataPlane: (options: OpenDataPlaneSessionOptions) => { opened.push(options); return { dispose: () => undefined }; },
  } as unknown as WailsBindingDeps).transitionBridge;
  await bridge.startLocalSession!({}); opened[0].onDisconnect?.();
  t.mock.timers.tick(1000); await tick();
  await bridge.closeSession('s'); finish({ ...route, Generation: 2 }); await tick();
  assert.equal(opened.length, 1);
});
