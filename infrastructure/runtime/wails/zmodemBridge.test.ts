import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createZmodemBridge } from './zmodemBridge';
import { createWailsRuntimeClient, type WailsBindingDeps } from './wailsRuntimeClient';

test('runtime routes ZMODEM send receive cancel and event through injected terminal bindings', async () => {
  const calls: unknown[] = [];
  const runtime = createWailsRuntimeClient({
    terminal: {
      SendZmodem: async (...args: unknown[]) => { calls.push(['send', ...args]); },
      ReceiveZmodem: async (...args: unknown[]) => { calls.push(['receive', ...args]); },
      CancelZmodem: async (...args: unknown[]) => { calls.push(['cancel', ...args]); },
    }, sftp: {}, events: { On: (name: string) => { calls.push(['event', name]); return () => undefined; } },
  } as unknown as WailsBindingDeps).transitionBridge;
  assert.deepEqual(await runtime.startZmodemDragDropUpload!('s', [{ name: 'file', remoteName: 'remote', path: '/file' }]), { success: true });
  assert.deepEqual(await runtime.receiveZmodem!('s', '/dest'), { success: true });
  await runtime.cancelZmodem!('s');
  runtime.onZmodemEvent!('s', () => undefined)();
  assert.deepEqual(calls, [['send', 's', '/file', 'remote', 'rz'], ['receive', 's', '/dest'], ['cancel', 's'], ['event', 'terminal:zmodem']]);
});

test('ZMODEM stages binary memory data, preserves remote names, passes command to Go and cleans only staged files', async () => {
  const calls: unknown[] = [];
  const bridge = createZmodemBridge({
    SendZmodem: async (...args) => { calls.push(['send', ...args]); },
  }, {
    StageBegin: async name => { calls.push(['begin', name]); return '/private/staged'; },
    StageAppend: async (path, offset, data) => { calls.push(['append', path, offset, [...Buffer.from(data, 'base64')]]); },
    StageDiscard: async path => { calls.push(['discard', path]); },
  });
  assert.deepEqual(await bridge.startZmodemDragDropUpload('s', [
    { name: 'memory', remoteName: 'remote.bin', data: Uint8Array.of(0, 255, 13).buffer },
    { name: 'local', remoteName: 'other.bin', path: '/original/file' },
  ], 'custom rz\r'), { success: true });
  assert.deepEqual(calls, [
    ['begin', 'memory'], ['append', '/private/staged', 0, [0, 255, 13]],
    ['send', 's', '/private/staged', 'remote.bin', 'custom rz'], ['discard', '/private/staged'],
    ['send', 's', '/original/file', 'other.bin', 'custom rz'],
  ]);
});

test('ZMODEM staging and send failures clean temporary files and stop later sends', async () => {
  for (const failAt of ['append', 'send']) {
    const discarded: string[] = [];
    let sends = 0;
    const bridge = createZmodemBridge({ SendZmodem: async () => { sends++; throw new Error('send failed'); } }, {
      StageBegin: async () => '/staged',
      StageAppend: async () => { if (failAt === 'append') throw new Error('append failed'); },
      StageDiscard: async path => { discarded.push(path); },
    });
    const result = await bridge.startZmodemDragDropUpload('s', [
      { name: 'memory', remoteName: 'remote', data: new ArrayBuffer(1) },
      { name: 'later', remoteName: 'later', path: '/later' },
    ]);
    assert.equal(result.success, false);
    assert.match(result.error!, /failed/);
    assert.deepEqual(discarded, ['/staged']);
    assert.equal(sends, failAt === 'append' ? 0 : 1);
  }
});

test('ZMODEM rejects unavailable bindings and empty input; defaults command to rz', async () => {
  assert.equal((await createZmodemBridge({}).startZmodemDragDropUpload('s', [{ name: 'f', remoteName: 'f', path: '/f' }])).success, false);
  const commands: string[] = [];
  const bridge = createZmodemBridge({ SendZmodem: async (_id, _path, _name, command) => { commands.push(command); } });
  assert.equal((await bridge.startZmodemDragDropUpload('s', [])).success, false);
  assert.equal((await bridge.startZmodemDragDropUpload('s', [{ name: 'f', remoteName: 'f' }])).success, false);
  assert.equal((await bridge.startZmodemDragDropUpload('s', [{ name: 'f', remoteName: 'f', path: '/f' }])).success, true);
  assert.deepEqual(commands, ['rz']);
});

test('automatic download detect prompts once across subscribers and filename events never prompt', async () => {
  const listeners: Array<(event: unknown) => void> = [];
  let finish!: (path: string | null) => void;
  let prompts = 0;
  const received: string[] = [];
  const bridge = createZmodemBridge({
    ReceiveZmodem: async (session, path) => { received.push(`${session}:${path}`); },
    CancelZmodem: async session => { received.push(`cancel:${session}`); },
  }, undefined, (_name, callback) => { listeners.push(callback); return () => undefined; }, () => {
    prompts++; return new Promise(resolve => { finish = resolve; });
  });
  bridge.onZmodemEvent('s', () => undefined); bridge.onZmodemEvent('s', () => undefined);
  const detect = { data: { type: 'detect', sessionId: 's', transferType: 'download' } };
  for (const listener of listeners) listener(detect);
  assert.equal(prompts, 1);
  finish('/downloads'); await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(received, ['s:/downloads']);
  for (const listener of listeners) listener({ data: { type: 'detect', sessionId: 's', transferType: 'download', filename: 'file' } });
  assert.equal(prompts, 1);
  for (const listener of listeners) listener(detect);
  assert.equal(prompts, 2);
  finish(null); await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(received, ['s:/downloads', 'cancel:s']);
});

test('ZMODEM event mapping filters session identity and returns event unsubscribe', () => {
  let handler!: (event: unknown) => void;
  let unsubscribed = false;
  const received: unknown[] = [];
  const bridge = createZmodemBridge({}, undefined, (name, callback) => {
    assert.equal(name, 'terminal:zmodem'); handler = callback;
    return () => { unsubscribed = true; };
  });
  const off = bridge.onZmodemEvent('s', event => received.push(event));
  handler({ data: { type: 'progress', sessionId: 'other', transferred: 12 } });
  handler({ data: { type: 'progress', sessionId: 's', transferred: 12, total: 20 } });
  handler({ data: { type: 'unexpected', sessionId: 's' } });
  assert.deepEqual(received, [{ type: 'progress', sessionId: 's', transferred: 12, total: 20 }]);
  off(); assert.equal(unsubscribed, true);
});
