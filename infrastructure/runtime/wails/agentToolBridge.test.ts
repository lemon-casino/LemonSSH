import assert from 'node:assert/strict';
import test from 'node:test';
import { createAgentToolBridge, type NativeAgentToolBindings } from './agentToolBridge';

const on = () => () => {};

test('external MCP settings reach the independent native access controller', async () => {
  const calls: unknown[] = [];
  const bridge = createAgentToolBridge({
    AgentExternalStatus: async () => ({ ok: true, enabled: true, launcherPath: 'LemonSSH-mcp.exe' }),
    AgentExternalSetEnabled: async enabled => { calls.push(enabled); return { ok: true }; },
    AgentExternalSetConfig: async config => { calls.push(config); return { ok: true }; },
  }, on, id => id);
  assert.equal((await bridge.externalMcpGetStatus!()).launcherPath, 'LemonSSH-mcp.exe');
  await bridge.externalMcpSetEnabled!(false);
  await bridge.externalMcpSetConfig!({ mode: 'persistent', idleTimeoutMinutes: 12 });
  assert.deepEqual(calls, [false, { mode: 'persistent', idleTimeoutMinutes: 12 }]);
});

test('native tool calls retain UI scope and map session metadata to the Go transport', async () => {
  const calls: unknown[][] = [];
  const bindings: NativeAgentToolBindings = {
    AgentCapability: async (...args) => { calls.push(args); return { ok: true, output: 'real output', exitCode: 7, exitCodeKnown: true }; },
    AgentUpdateSessions: async (...args) => { calls.push(args); },
  };
  const bridge = createAgentToolBridge(bindings, on, id => `go-${id}`);
  const sessions = [{ sessionId: 'ui', hostname: 'server', label: 'prod', connected: true, hostChain: [{ hostId: 'jump' }] }];
  await bridge.aiMcpUpdateSessions!(sessions, 'chat');
  assert.deepEqual(calls[0], ['chat', [{ ...sessions[0], nativeSessionId: 'go-ui' }], false]);
  const result = await bridge.aiExec!('ui', 'pwd', 'chat');
  assert.deepEqual(calls[1], ['netcatty/exec', { sessionId: 'ui', command: 'pwd' }, 'chat']);
  assert.equal(result.stdout, 'real output');
  assert.equal(result.exitCode, 7);
  await bridge.aiCapability!('vault/notes/create', { title: 'note' }, 'chat');
  assert.deepEqual(calls[2], ['vault/notes/create', { title: 'note' }, 'chat']);
});

test('missing bindings and native failures never report successful execution', async () => {
  const missing = createAgentToolBridge(undefined, on, id => id);
  assert.equal((await missing.aiExec!('ui', 'pwd', 'chat')).ok, false);
  await assert.rejects(missing.aiCapability!('vault/notes/list', {}, 'chat'), /unavailable/);
  const bridge = createAgentToolBridge({ AgentCapability: async () => ({ ok: true, output: 'partial', exitCode: 0, exitCodeKnown: false }) }, on, id => id);
  assert.equal((await bridge.aiExec!('ui', 'pwd', 'chat')).exitCode, null);
});

test('scope merge, cancellation, command policy and grants reach the host', async () => {
  const calls: unknown[][] = [];
  const record = async (...args: unknown[]) => { calls.push(args); };
  const bridge = createAgentToolBridge({ AgentUpdateSessions: record, AgentSetCancelled: record, AgentSetPermissionMode: record, AgentSetCommandPolicy: record, AgentSyncPermissionGrants: record }, on, id => id);
  await bridge.aiMcpMergeSessions!([], 'chat');
  await bridge.aiSetChatSessionCancelled!('chat', true);
  await bridge.aiSetChatSessionCancelled!('chat', false);
  await bridge.aiMcpSetPermissionMode!('observer');
  await bridge.aiMcpSetCommandBlocklist!([String.raw`rm\s+-rf`]);
  await bridge.aiMcpSetCommandTimeout!(90);
  await bridge.aiMcpSyncPermissionGrants!([{ id: 'grant' }]);
  assert.deepEqual(calls, [['chat', [], true], ['chat', true], ['chat', false], ['observer'], [[String.raw`rm\s+-rf`], 0], [null, 90], [[{ id: 'grant' }]]]);
});

test('attachments retain inline content and vault events keep correlation IDs', async () => {
  const calls: unknown[][] = [];
  let listener: (event: { data?: unknown }) => void = () => {};
  let disposed = false;
  const bridge = createAgentToolBridge({
    AgentRegisterChatAttachments: async (...args) => { calls.push(args); },
    AgentRespondVault: async (...args) => { calls.push(args); },
    AgentVaultRequestPending: async id => id === 'pending',
  }, (name, callback) => { assert.equal(name, 'agent:vault-request'); listener = callback; return () => { disposed = true; }; }, id => id);
  await bridge.aiMcpUpdateAttachments!([{ filename: 'a.txt', base64Data: 'aGk=' }], 'chat');
  assert.equal((calls[0][1] as Array<{ base64Data: string }>)[0].base64Data, 'aGk=');
  const seen: unknown[] = [];
  const dispose = bridge.onVaultAgentRequest!(payload => seen.push(payload));
  const request = { requestId: 'pending', op: 'note.create', params: { title: 'note' } };
  listener({ data: request });
  assert.deepEqual(seen, [request]);
  listener({ data: [request] });
  assert.deepEqual(seen, [request, request]);
  assert.equal(await bridge.isVaultAgentRequestPending!('expired'), false);
  await bridge.respondVaultAgent!('pending', { ok: true });
  assert.deepEqual(calls[1], ['pending', { ok: true }]);
  dispose();
  assert.equal(disposed, true);
});
