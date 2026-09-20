import assert from 'node:assert/strict';
import test from 'node:test';
import { tool } from 'ai';
import { z } from 'zod';
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

test('a provider failure after a successful tool call returns the collected output without a retry error', async t => {
  const host = globalThis as unknown as { window?: unknown };
  const previous = host.window;
  t.after(() => { host.window = previous; });
  const animationHost = globalThis as unknown as {
    requestAnimationFrame?: (callback: (time: number) => void) => number;
    cancelAnimationFrame?: (id: number) => void;
  };
  const previousRequestAnimationFrame = animationHost.requestAnimationFrame;
  const previousCancelAnimationFrame = animationHost.cancelAnimationFrame;
  let animationFrameId = 0;
  animationHost.requestAnimationFrame = callback => {
    const id = ++animationFrameId;
    queueMicrotask(() => callback(Date.now()));
    return id;
  };
  animationHost.cancelAnimationFrame = () => {};
  t.after(() => {
    if (previousRequestAnimationFrame) {
      animationHost.requestAnimationFrame = previousRequestAnimationFrame;
    } else {
      delete animationHost.requestAnimationFrame;
    }
    if (previousCancelAnimationFrame) {
      animationHost.cancelAnimationFrame = previousCancelAnimationFrame;
    } else {
      delete animationHost.cancelAnimationFrame;
    }
  });

  const dataHandlers = new Map<string, (data: string) => void>();
  const endHandlers = new Map<string, () => void>();
  let requestCount = 0;
  let allowContinuation = false;
  const emitChunk = (emit: (data: string) => void, delta: Record<string, unknown>, finishReason?: string) => {
    emit(JSON.stringify({
      id: 'chatcmpl-tool-fallback',
      object: 'chat.completion.chunk',
      created: 1,
      model: 'fixture',
      choices: [{ index: 0, delta, finish_reason: finishReason ?? null }],
    }));
  };

  host.window = { netcatty: {
    aiChatStream: async (requestId: string) => {
      requestCount += 1;
      if (allowContinuation) {
        setTimeout(() => {
          const emit = dataHandlers.get(requestId);
          assert.ok(emit);
          emitChunk(emit, { content: 'Conversation continued.' });
          emitChunk(emit, {}, 'stop');
          endHandlers.get(requestId)?.();
        }, 0);
        return { ok: true, statusCode: 200, statusText: 'OK' };
      }
      if (requestCount === 1) {
        setTimeout(() => {
          const emit = dataHandlers.get(requestId);
          assert.ok(emit);
          emitChunk(emit, {
            tool_calls: [{
              index: 0,
              id: 'call-system-usage',
              type: 'function',
              function: { name: 'terminal_exec', arguments: '{}' },
            }],
          });
          emitChunk(emit, {}, 'tool_calls');
          endHandlers.get(requestId)?.();
        }, 0);
        return { ok: true, statusCode: 200, statusText: 'OK' };
      }
      return { ok: true, statusCode: 503, statusText: 'Service temporarily unavailable' };
    },
    aiChatCancel: async () => true,
    onAiStreamData: (requestId: string, callback: (data: string) => void) => {
      dataHandlers.set(requestId, callback);
      return () => dataHandlers.delete(requestId);
    },
    onAiStreamEnd: (requestId: string, callback: () => void) => {
      endHandlers.set(requestId, callback);
      return () => endHandlers.delete(requestId);
    },
    onAiStreamError: () => () => {},
  } };

  const messages: ChatMessage[] = [{
    id: 'message',
    role: 'assistant',
    content: '',
    timestamp: Date.now(),
  }];
  const model = createModelFromConfig({
    id: 'p', providerId: 'custom', name: 'fixture', defaultModel: 'fixture',
    apiKey: 'fixture', baseURL: 'https://fixture.test/v1', enabled: true,
  });
  const collectedOutput = 'CPU: 3.7%\nMemory: 38%\nDisk: 18%';
  const result = await processCattyStream({
    model, streamSessionId: 'chat', currentAssistantMsgId: 'message', systemPrompt: 'test',
    sdkMessages: [{ role: 'user', content: 'inspect system usage' }],
    toolsBundle: {
      tools: {
        terminal_exec: tool({
          inputSchema: z.object({}),
          execute: async () => ({
            stdout: `${collectedOutput}\n`,
            stderr: '',
            exitCode: 0,
            command: 'inspect-system',
          }),
        }),
      } as never,
      toolsContext: {},
    },
    signal: new AbortController().signal, maxIterations: 2,
    runtimeContext: { chatSessionId: 'chat', turnId: 'turn', agentKind: 'sidebar', permissionMode: 'auto', scopeType: 'terminal' },
    ui: {
      addMessageToSession: (_id, message) => { messages.push(message); },
      updateMessageById: (_id, messageId, updater) => {
        const index = messages.findIndex(message => message.id === messageId);
        assert.notEqual(index, -1);
        messages[index] = updater(messages[index]);
      },
    },
  });

  assert.deepEqual(result, {});
  const fallback = messages.find(message => message.role === 'assistant' && message.content === collectedOutput);
  assert.ok(fallback, 'collected tool output should be returned as assistant content');
  assert.equal(fallback.executionStatus, 'completed');
  assert.equal(fallback.errorInfo, undefined);

  allowContinuation = true;
  messages.push({
    id: 'continuation-message',
    role: 'assistant',
    content: '',
    timestamp: Date.now(),
  });
  await processCattyStream({
    model,
    streamSessionId: 'chat',
    currentAssistantMsgId: 'continuation-message',
    systemPrompt: 'test',
    sdkMessages: [{ role: 'user', content: 'continue' }],
    toolsBundle: { tools: {}, toolsContext: {} },
    signal: new AbortController().signal,
    maxIterations: 1,
    runtimeContext: { chatSessionId: 'chat', turnId: 'turn-2', agentKind: 'sidebar', permissionMode: 'auto', scopeType: 'terminal' },
    ui: {
      addMessageToSession: (_id, message) => { messages.push(message); },
      updateMessageById: (_id, messageId, updater) => {
        const index = messages.findIndex(message => message.id === messageId);
        assert.notEqual(index, -1);
        messages[index] = updater(messages[index]);
      },
    },
  });
  const continuation = messages.find(message => message.id === 'continuation-message');
  assert.equal(continuation?.content, 'Conversation continued.');
  assert.equal(continuation?.errorInfo, undefined);
});
