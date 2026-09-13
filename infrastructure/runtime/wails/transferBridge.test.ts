import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createTransferBridge } from './transferBridge';

test('missing backend task returns structured resume failure for reconnect fallback', async () => {
  const bridge = createTransferBridge({ Start: async () => snapshot('running'), Progress: async () => snapshot('running'), Pause: async () => { throw new Error('transfer task not found'); }, Resume: async () => { throw new Error('transfer task not found'); }, Cancel: async () => {} });
  assert.deepEqual(await bridge.resumeTransfer('missing'), { success: false, reason: 'transfer task not found' });
});

test('subscription reattaches to backend transfer and emits authoritative epochs through pause resume completion', async () => {
  let state = { ...snapshot('paused', 3), lifecycleEpoch: 8, sourcePath: '/src', targetPath: '/dst', targetSessionId: 'remote', direction: 'upload' as const };
  const events: Array<{ type: string; lifecycleEpoch?: number }> = [];
  const bridge = createTransferBridge({
    List: async () => [state],
    Start: async () => { throw new Error('must reattach without Start'); }, Progress: async () => state,
    Pause: async () => { state = { ...state, state: 'paused', lifecycleEpoch: 10 }; },
    Resume: async () => { state = { ...state, state: 'running', lifecycleEpoch: 9 }; },
    Cancel: async () => {},
  }, 1);
  const unsubscribe = bridge.onGlobalSftpTransferEvent(event => events.push(event));
  try {
    await new Promise(resolve => setTimeout(resolve, 10));
    assert.ok(events.some(event => event.lifecycleEpoch === 8));
    assert.deepEqual(await bridge.resumeTransfer('t1'), { success: true, lifecycleEpoch: 9 });
    assert.deepEqual(await bridge.pauseTransfer('t1'), { success: true, checkpointBytes: 3, lifecycleEpoch: 10 });
    state = { ...state, state: 'completed', doneBytes: 10 };
    await new Promise(resolve => setTimeout(resolve, 10));
    assert.ok(events.some(event => event.type === 'completed' && event.lifecycleEpoch === 10));
  } finally { unsubscribe(); }
});

test('pause leaves the stream promise pending until a terminal backend state', async () => {
  let state = snapshot('running', 3);
  const bridge = createTransferBridge({ Start: async () => state, Progress: async () => state,
    Pause: async () => { state = snapshot('paused', 3); }, Resume: async () => {}, Cancel: async () => { state = snapshot('cancelled', 3); } }, 1);
  let settled = false;
  const pending = bridge.startStreamTransfer(options).finally(() => { settled = true; });
  await bridge.pauseTransfer('t1');
  await new Promise(resolve => setTimeout(resolve, 10));
  assert.equal(settled, false);
  await bridge.cancelTransfer('t1');
  await pending;
});

test('compressed upload publishes compression and scheduled upload phases and routes controls by id', async () => {
  let state={...snapshot('running'),phase:'compressing' as 'compressing'|'uploading',controlKind:'compressed-upload' as const,sourcePath:'/folder',targetPath:'/dst/archive.zip',lifecycleEpoch:0};
  const phases:string[]=[];
  const byteEvents:Array<{phase?:string;transferred?:number;totalBytes?:number}>=[];
  const bridge=createTransferBridge({Start:async()=>{throw new Error('wrong start')},StartCompressed:async()=>state,Progress:async()=>state,
    Pause:async id=>{assert.equal(id,'t1');state={...state,state:'paused',lifecycleEpoch:1}},Resume:async()=>{state={...state,state:'running',phase:'uploading',lifecycleEpoch:2,doneBytes:5,totalBytes:10}},Cancel:async()=>{state={...state,state:'cancelled'}}},1);
  state={...state,doneBytes:1000,totalBytes:1000};
  bridge.onGlobalSftpTransferEvent(event=>{byteEvents.push(event);if(event.phase)phases.push(event.phase)});
  const result=bridge.startCompressedUpload({compressionId:'t1',sftpId:'remote',folderPath:'/folder',targetPath:'/dst',folderName:'archive',totalBytes:10});
  await new Promise(resolve=>setTimeout(resolve,5));
  await bridge.pauseTransfer('t1'); await bridge.resumeTransfer('t1'); await bridge.cancelTransfer('t1');
  assert.equal((await result).success,false);
  assert.ok(phases.includes('compressing')); assert.ok(phases.includes('uploading'));
  assert.ok(byteEvents.filter(event=>event.phase==='compressing').every(event=>event.transferred===0 && event.totalBytes===0));
  assert.ok(byteEvents.some(event=>event.phase==='uploading' && event.transferred===5 && event.totalBytes===10));
});

const options = { transferId: 't1', sourcePath: '/src', targetPath: '/dst', sourceType: 'local', targetType: 'sftp', targetSftpId: 'remote' } as const;
const snapshot = (state: string, doneBytes = 0) => ({ taskId: 't1', state, doneBytes, totalBytes: 10 });

test('stream completion waits for Go completion and emits real byte progress', async () => {
  const events: Array<{ type: string; transferred?: number }> = [];
  const states = [snapshot('running', 4), snapshot('completed', 10)];
  const bridge = createTransferBridge({
    Start: async request => { assert.equal(request.targetSessionId, 'remote'); assert.equal(request.sourceSessionId, ''); return snapshot('running'); },
    Progress: async () => states.shift()!,
    Pause: async () => {}, Resume: async () => {}, Cancel: async () => {},
  }, 1);
  bridge.onGlobalSftpTransferEvent(event => events.push(event));
  const result = await bridge.startStreamTransfer(options);
  assert.equal(result.totalBytes, 10);
  assert.deepEqual(events.map(event => event.type), ['started', 'progress', 'completed']);
  assert.equal(events[1].transferred, 4);
});

test('pause and resume expose checkpoints and cancellation settles pending transfer', async () => {
  let state = snapshot('running', 3);
  const bridge = createTransferBridge({
    Start: async () => state, Progress: async () => state,
    Pause: async () => { state = snapshot('paused', 3); },
    Resume: async () => { state = snapshot('running', 3); },
    Cancel: async () => { state = snapshot('cancelled', 3); },
  }, 1);
  const pending = bridge.startStreamTransfer(options);
  await new Promise(resolve => setTimeout(resolve, 5));
  assert.deepEqual(await bridge.pauseTransfer('t1'), { success: true, checkpointBytes: 3 });
  assert.deepEqual(await bridge.resumeTransfer('t1'), { success: true });
  await bridge.cancelTransfer('t1');
  assert.equal((await pending).cancelled, true);
});

test('failed backend state and missing binding cannot report success', async () => {
  const bridge = createTransferBridge({
    Start: async () => ({ ...snapshot('failed'), error: 'disk full' }),
    Progress: async () => snapshot('failed'), Pause: async () => {}, Resume: async () => {}, Cancel: async () => {},
  }, 1);
  await assert.rejects(bridge.startStreamTransfer(options), /disk full/);
  await assert.rejects(createTransferBridge(undefined).startStreamTransfer(options), /not available/);
});
