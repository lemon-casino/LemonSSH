import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createExternalAgentBridge } from './externalAgentBridge.ts';

type Bindings = NonNullable<Parameters<typeof createExternalAgentBridge>[0]>;

function native(overrides: Partial<Bindings> = {}): Bindings {
  return {
    Stream: async () => ({ ok: true }),
    Cancel: async () => ({ ok: true }),
    Cleanup: async () => ({ ok: true }),
    Steer: async () => ({ status: 'inactive' }),
    ListModels: async () => ({ ok: true }),
    CodexAppServerStatus: async () => ({ ok: true }),
    AccountInfo: async () => ({}),
    ...overrides,
  };
}

test('CodeBuddy responses preserve action, content and native failures', async () => {
  const calls: unknown[][] = [];
  const bridge = createExternalAgentBridge(native({
    RespondCodebuddyElicitation: async (...args) => {
      calls.push(args);
      return args[1] === 'accept' ? { ok: true } : { ok: false, error: 'no longer pending' };
    },
  }), () => () => {});
  assert.deepEqual(await bridge.aiSdkAgentElicitationResponse?.('card', 'accept', { confirm: true }), { ok: true });
  assert.deepEqual(await bridge.aiSdkAgentElicitationResponse?.('card', 'cancel'), { ok: false, error: 'no longer pending' });
  assert.deepEqual(calls, [['card', 'accept', { confirm: true }], ['card', 'cancel', {}]]);
  const stale = createExternalAgentBridge(native(), () => () => {});
  assert.equal((await stale.aiSdkAgentElicitationResponse?.('card', 'accept'))?.ok, false);
});

test('CodeBuddy interaction events remain scoped to the streaming request', () => {
  const handlers = new Map<string, (event: { data?: unknown }) => void>();
  const bridge = createExternalAgentBridge(native(), (name, callback) => {
    handlers.set(name, callback);
    return () => { handlers.delete(name); };
  });
  const events: unknown[] = [];
  const off = bridge.onAiSdkAgentEvent?.('mine', event => { events.push(event); });
  const dispatch = handlers.get('ai:sdk-agent:event')!;
  dispatch({ data: { requestId: 'other', event: { type: 'elicitation-create' } } });
  const request = { type: 'elicitation-create', elicitationId: 'scoped', request: { message: 'Continue?' } };
  dispatch({ data: [{ requestId: 'mine', event: request }] });
  assert.deepEqual(events, [request]);
  off?.();
  assert.equal(handlers.size, 0);
});

test('external agent stream preserves selected tool integration mode', async () => {
  const requests: Record<string, unknown>[] = [];
  const bridge = createExternalAgentBridge(native({ Stream: async request => { requests.push(request); return { ok: true }; } }), () => () => {});
  await bridge.aiSdkAgentStream?.('r', 'chat', 'codebuddy', 'prompt', undefined, undefined, undefined, undefined, undefined, undefined, 'skills');
  await bridge.aiSdkAgentStream?.('r2', 'chat', 'codebuddy', 'prompt');
  assert.equal(requests[0].toolIntegrationMode, 'skills');
  assert.equal(requests[1].toolIntegrationMode, 'mcp');
});
