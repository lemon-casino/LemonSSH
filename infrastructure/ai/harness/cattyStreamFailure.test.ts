import assert from 'node:assert/strict';
import test from 'node:test';
import { createModelFromConfig } from '../sdk/providers';
import { processCattyStream } from './turnDrivers/cattyStreamProcessor';
import type { ChatMessage } from '../types';

test('a provider rejection reports its cause once without a second NoOutputGenerated failure', async t => {
  const host = globalThis as unknown as { window?: unknown };
  const previous = host.window;
  t.after(() => { host.window = previous; });
  host.window = { netcatty: {
    aiChatStream: async () => ({ ok: true, statusCode: 401, statusText: 'fixture key rejected' }),
    aiChatCancel: async () => true,
    onAiStreamData: () => () => {},
    onAiStreamEnd: () => () => {},
    onAiStreamError: () => () => {},
  } };
  const messages: ChatMessage[] = [];
  const model = createModelFromConfig({ id: 'p', providerId: 'openai', name: 'fixture', defaultModel: 'fixture', apiKey: 'fixture', enabled: true });
  const result = await processCattyStream({
    model, streamSessionId: 'chat', currentAssistantMsgId: 'message', systemPrompt: 'test',
    sdkMessages: [{ role: 'user', content: 'hello' }], toolsBundle: { tools: {}, toolsContext: {} },
    signal: new AbortController().signal, maxIterations: 1,
    runtimeContext: { chatSessionId: 'chat', turnId: 'turn', agentKind: 'sidebar', permissionMode: 'auto', scopeType: 'terminal' },
    ui: { addMessageToSession: (_id, message) => { messages.push(message); }, updateMessageById: () => {} },
  });
  assert.deepEqual(result, {});
  assert.equal(messages.length, 1);
  assert.equal(messages[0].errorInfo?.type, 'auth');
  assert.match(messages[0].errorInfo?.message ?? '', /fixture key rejected/);
});
