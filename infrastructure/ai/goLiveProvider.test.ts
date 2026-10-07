import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildGoLiveProviderPayload,
  buildGoLiveProviderPayloadFor,
  openAIChatEndpoint,
  resolveCattyModelId,
  resolveCattyProvider,
  type GoLiveProviderInputs,
} from './goLiveProvider';
import type { ProviderConfig } from './types';

const provider = (overrides: Partial<ProviderConfig>): ProviderConfig => ({
  id: 'p1',
  providerId: 'openai',
  name: 'Test',
  enabled: true,
  ...overrides,
});

const inputs = (overrides: Partial<GoLiveProviderInputs> = {}): GoLiveProviderInputs => ({
  providers: [provider({ apiKey: 'sk-test' })],
  activeProviderId: 'p1',
  activeModelId: 'gpt-active',
  ...overrides,
});

test('buildGoLiveProviderPayloadFor resolves the preset endpoint, model and key', () => {
  const payload = buildGoLiveProviderPayloadFor(provider({ apiKey: 'enc:v1:sealed' }), 'gpt-5', 20);
  assert.deepEqual(payload, {
    family: 'openai',
    endpoint: 'https://api.openai.com/v1/chat/completions',
    apiKeyHeader: 'Authorization',
    apiKeyValue: 'enc:v1:sealed',
    model: 'gpt-5',
    maxIterations: 20,
  });
});

test('buildGoLiveProviderPayloadFor appends /v1 to bare origins and honors custom paths', () => {
  assert.equal(
    openAIChatEndpoint(provider({ providerId: 'custom', baseURL: 'https://gw.example.com' })),
    'https://gw.example.com/v1/chat/completions',
  );
  assert.equal(
    openAIChatEndpoint(provider({ providerId: 'custom', baseURL: 'https://gw.example.com/api/v4/' })),
    'https://gw.example.com/api/v4/chat/completions',
  );
  // Ollama Cloud origin gets its /v1 prefix like the renderer chain.
  assert.equal(
    openAIChatEndpoint(provider({ providerId: 'ollama', baseURL: 'https://ollama.com' })),
    'https://ollama.com/v1/chat/completions',
  );
  assert.equal(openAIChatEndpoint(provider({ providerId: 'custom', baseURL: '' })), '');
});

test('buildGoLiveProviderPayloadFor returns null for native wire families and incomplete configs', () => {
  // The Go ToolLoop speaks OpenAI Chat wire only: native families must stay
  // on the renderer chain (null clears any installed driver).
  assert.equal(buildGoLiveProviderPayloadFor(provider({ providerId: 'anthropic' }), 'claude-x', 20), null);
  assert.equal(buildGoLiveProviderPayloadFor(provider({ providerId: 'google' }), 'gemini-x'), null);
  assert.equal(buildGoLiveProviderPayloadFor(provider({ enabled: false }), 'm'), null);
  assert.equal(buildGoLiveProviderPayloadFor(provider({ apiKey: '' }), 'm'), null);
  assert.equal(buildGoLiveProviderPayloadFor(null, 'm'), null);
});

test('the Catty resolution mirrors the panel: per-agent override then global fallback', () => {
  const overrides = provider({ id: 'deepseek', providerId: 'deepseek', defaultModel: 'deepseek-chat', apiKey: 'k' });
  const scoped: GoLiveProviderInputs = inputs({
    providers: [overrides, provider({ defaultModel: 'gpt-preset', apiKey: 'sk' })],
    activeProviderId: 'p1',
    activeModelId: 'gpt-active',
    cattyProviderId: 'deepseek',
    cattyModelId: 'deepseek-reasoner',
  });
  const resolved = resolveCattyProvider(scoped);
  assert.equal(resolved?.id, 'deepseek');
  assert.equal(resolveCattyModelId(scoped, resolved), 'deepseek-reasoner');

  // Override provider without a stored model keeps the provider default.
  assert.equal(resolveCattyModelId({ ...scoped, cattyModelId: '' }, resolved), 'deepseek-chat');

  // Without an override: the provider's defaultModel wins over the global
  // active model; without a defaultModel the global active model applies.
  const global = resolveCattyProvider(inputs());
  assert.equal(global?.id, 'p1');
  assert.equal(resolveCattyModelId(inputs(), global), 'gpt-active');
  assert.equal(
    resolveCattyModelId(
      inputs({ providers: [provider({ defaultModel: 'gpt-preset', apiKey: 'sk' })] }),
      provider({ defaultModel: 'gpt-preset', apiKey: 'sk' }),
    ),
    'gpt-preset',
  );

  // A dangling override id falls back to the global active provider.
  assert.equal(resolveCattyProvider({ ...scoped, cattyProviderId: 'gone' })?.id, 'p1');
});

test('buildGoLiveProviderPayload turns the raw Settings→AI state into the Go payload', () => {
  const payload = buildGoLiveProviderPayload(inputs({
    providers: [provider({ defaultModel: 'gpt-preset', apiKey: 'sk-test' })],
    maxIterations: 20,
  }));
  assert.equal(payload?.endpoint, 'https://api.openai.com/v1/chat/completions');
  assert.equal(payload?.model, 'gpt-preset');
  assert.equal(payload?.maxIterations, 20);
  assert.equal(buildGoLiveProviderPayload(inputs({ providers: [] })), null);
});
