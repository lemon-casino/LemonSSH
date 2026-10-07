/**
 * goLiveProvider — resolves the Settings→AI provider selection into the
 * wire payload the Go turn runtime installs through
 * AgentService.AgentSetLiveProvider, and decides when Catty turns can ride
 * the Go driver instead of the renderer chain.
 *
 * The Go ToolLoop (internal/agent/providers) speaks the OpenAI Chat
 * Completions wire protocol. Providers whose family resolves to native
 * `anthropic`/`google` stay on the renderer chain:
 * buildGoLiveProviderPayload returns null for them, which clears any
 * previously installed driver — never a broken turn.
 */

import { normalizeOpenAICompatSdkBaseURL } from './openaiCompatBaseUrl';
import { normalizeOllamaSdkBaseURL } from './ollamaCompatBaseUrl';
import { PROVIDER_PRESETS, resolveProviderStyle, type AIProviderId, type ProviderConfig } from './types';

/** Mirrors the Go ProviderConfig wire shape (cmd/lemonssh/providerConfig.go). */
export interface GoLiveProviderPayload {
  family: 'openai';
  endpoint: string;
  apiKeyHeader: string;
  apiKeyValue: string;
  model: string;
  maxIterations?: number;
}

const trim = (value: string | undefined | null): string => (value ?? '').trim();

/** Effective base URL: explicit override first, then the preset default. */
export function resolveProviderBaseURL(provider: ProviderConfig): string {
  return trim(provider.baseURL) || PROVIDER_PRESETS[provider.providerId as AIProviderId]?.defaultBaseURL || '';
}

/** Full chat-completions endpoint for an OpenAI-compatible provider. */
export function openAIChatEndpoint(provider: ProviderConfig): string {
  let base = resolveProviderBaseURL(provider);
  if (!base) return '';
  if (provider.providerId === 'ollama') base = normalizeOllamaSdkBaseURL(base);
  base = normalizeOpenAICompatSdkBaseURL(base);
  return `${base}/chat/completions`;
}

export interface GoLiveProviderInputs {
  providers: ProviderConfig[];
  activeProviderId: string;
  activeModelId: string;
  /** agentProviderMap['catty'] — per-agent provider override. */
  cattyProviderId?: string;
  /** agentModelMap['catty'] — per-agent model override. */
  cattyModelId?: string;
  maxIterations?: number;
}

/**
 * The Catty agent's provider: per-agent override first, then the global
 * active provider. Mirrors the AIChatSidePanel cattyAgentProvider memo.
 */
export function resolveCattyProvider(inputs: GoLiveProviderInputs): ProviderConfig | null {
  const override = trim(inputs.cattyProviderId);
  if (override) {
    const found = inputs.providers.find(provider => provider.id === override);
    if (found) return found;
  }
  return inputs.providers.find(provider => provider.id === trim(inputs.activeProviderId)) ?? null;
}

/** Mirrors the AIChatSidePanel cattyAgentModelId memo. */
export function resolveCattyModelId(inputs: GoLiveProviderInputs, provider: ProviderConfig | null): string {
  if (provider && trim(inputs.cattyProviderId) === provider.id) {
    return trim(inputs.cattyModelId) || trim(provider.defaultModel);
  }
  return trim(provider?.defaultModel) || trim(inputs.activeModelId);
}

/**
 * Builds the Go live-provider payload for one resolved provider+model pair,
 * or null when the Go runtime cannot serve it (no enabled provider, missing
 * endpoint/model/key, or a native anthropic/google wire family). null clears
 * the previously installed driver.
 */
export function buildGoLiveProviderPayloadFor(
  provider: ProviderConfig | null,
  modelId: string,
  maxIterations?: number,
): GoLiveProviderPayload | null {
  if (!provider || !provider.enabled) return null;
  if (resolveProviderStyle(provider) !== 'openai') return null;
  const endpoint = openAIChatEndpoint(provider);
  const model = trim(modelId);
  const apiKey = trim(provider.apiKey);
  if (!endpoint || !model || !apiKey) return null;
  return {
    family: 'openai',
    endpoint,
    apiKeyHeader: 'Authorization',
    apiKeyValue: apiKey,
    model,
    maxIterations,
  };
}

/**
 * Builds the Go live-provider payload for the Catty agent from the raw
 * selection state (Settings→AI storage shape).
 */
export function buildGoLiveProviderPayload(inputs: GoLiveProviderInputs): GoLiveProviderPayload | null {
  const provider = resolveCattyProvider(inputs);
  return buildGoLiveProviderPayloadFor(provider, resolveCattyModelId(inputs, provider), inputs.maxIterations);
}
