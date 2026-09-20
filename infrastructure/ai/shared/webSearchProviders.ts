/**
 * Web search provider implementations.
 *
 * Each provider function normalises its API response into a common
 * `{ results: Array<{ title, url, content }> }` shape so callers don't need
 * to know about provider-specific quirks.
 *
 * All HTTP requests go through `bridge.aiFetch()` to avoid CORS issues in the
 * renderer process.
 */

import type { NetcattyBridge } from '../cattyAgent/executor';
import type { WebSearchConfig } from '../types';
import { WEB_SEARCH_PROVIDER_PRESETS } from '../types';
import { decryptField } from '../../persistence/secureFieldAdapter';

export interface WebSearchResult {
  title: string;
  url: string;
  content: string;
}

interface BridgeFetchResponse {
  ok: boolean;
  status?: number;
  data?: string;
  error?: string;
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

function resolveApiHost(config: WebSearchConfig): string {
  return config.apiHost || WEB_SEARCH_PROVIDER_PRESETS[config.providerId].defaultApiHost;
}

async function fetchJson(
  bridge: NetcattyBridge,
  url: string,
  method: string,
  headers: Record<string, string>,
  body?: string,
): Promise<unknown> {
  const aiFetch = (bridge as unknown as Record<string, (...args: unknown[]) => Promise<unknown>>).aiFetch;
  if (!aiFetch) throw new Error('aiFetch is not available on the bridge');
  // Search API hosts are added to the allowlist via aiSyncWebSearch, no skipHostCheck needed
  const resp = await aiFetch(url, method, headers, body) as BridgeFetchResponse;
  if (!resp.ok) throw new Error(resp.error || `HTTP ${resp.status}`);
  return JSON.parse(resp.data || '{}');
}

// ---------------------------------------------------------------------------
// Tavily
// ---------------------------------------------------------------------------

async function searchTavily(
  bridge: NetcattyBridge,
  config: WebSearchConfig,
  query: string,
  maxResults: number,
): Promise<WebSearchResult[]> {
  const host = resolveApiHost(config);
  const data = await fetchJson(bridge, `${host}/search`, 'POST', {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${config.apiKey}`,
  }, JSON.stringify({
    query,
    max_results: maxResults,
    search_depth: 'basic',
  })) as { results?: Array<{ title?: string; url?: string; content?: string }> };

  return (data.results || []).map(r => ({
    title: r.title || '',
    url: r.url || '',
    content: r.content || '',
  }));
}

// ---------------------------------------------------------------------------
// Exa
// ---------------------------------------------------------------------------

async function searchExa(
  bridge: NetcattyBridge,
  config: WebSearchConfig,
  query: string,
  maxResults: number,
): Promise<WebSearchResult[]> {
  const host = resolveApiHost(config);
  const data = await fetchJson(bridge, `${host}/search`, 'POST', {
    'Content-Type': 'application/json',
    'x-api-key': config.apiKey || '',
  }, JSON.stringify({
    query,
    numResults: maxResults,
    contents: { text: true },
  })) as { results?: Array<{ title?: string; url?: string; text?: string }> };

  return (data.results || []).map(r => ({
    title: r.title || '',
    url: r.url || '',
    content: r.text || '',
  }));
}

// ---------------------------------------------------------------------------
// Bocha
// ---------------------------------------------------------------------------

async function searchBocha(
  bridge: NetcattyBridge,
  config: WebSearchConfig,
  query: string,
  maxResults: number,
): Promise<WebSearchResult[]> {
  const host = resolveApiHost(config);
  const data = await fetchJson(bridge, `${host}/v1/web-search`, 'POST', {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${config.apiKey}`,
  }, JSON.stringify({
    query,
    count: maxResults,
    summary: true,
  })) as { webPages?: { value?: Array<{ name?: string; url?: string; snippet?: string; summary?: string }> } };

  return (data.webPages?.value || []).map(r => ({
    title: r.name || '',
    url: r.url || '',
    content: r.summary || r.snippet || '',
  }));
}

// ---------------------------------------------------------------------------
// Zhipu
// ---------------------------------------------------------------------------

async function searchZhipu(
  bridge: NetcattyBridge,
  config: WebSearchConfig,
  query: string,
  _maxResults: number,
): Promise<WebSearchResult[]> {
  const host = resolveApiHost(config);
  const data = await fetchJson(bridge, `${host}/web_search`, 'POST', {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${config.apiKey}`,
  }, JSON.stringify({
    search_query: query,
    search_engine: 'search_std',
  })) as { search_result?: Array<{ title?: string; link?: string; content?: string }> };

  return (data.search_result || []).map(r => ({
    title: r.title || '',
    url: r.link || '',
    content: r.content || '',
  }));
}

// ---------------------------------------------------------------------------
// SearXNG
// ---------------------------------------------------------------------------

async function searchSearxng(
  bridge: NetcattyBridge,
  config: WebSearchConfig,
  query: string,
  _maxResults: number,
): Promise<WebSearchResult[]> {
  const host = resolveApiHost(config);
  if (!host) throw new Error('SearXNG requires an API Host to be configured');
  const url = `${host}/search?q=${encodeURIComponent(query)}&format=json`;
  const data = await fetchJson(bridge, url, 'GET', {}) as {
    results?: Array<{ title?: string; url?: string; content?: string }>;
  };

  return (data.results || []).map(r => ({
    title: r.title || '',
    url: r.url || '',
    content: r.content || '',
  }));
}

// ---------------------------------------------------------------------------
// Dispatcher
// ---------------------------------------------------------------------------

const PROVIDER_SEARCH_FNS: Record<string, typeof searchTavily> = {
  tavily: searchTavily,
  exa: searchExa,
  bocha: searchBocha,
  zhipu: searchZhipu,
  searxng: searchSearxng,
};

/**
 * Placeholder token for the web search API key.
 * The renderer sends this in HTTP headers; the main process replaces it
 * with the real decrypted key before the request is sent, so plaintext
 * keys never enter the renderer.
 */
const WEB_SEARCH_KEY_PLACEHOLDER = '__WEB_SEARCH_KEY__';

/**
 * Renderer-side decrypt hook for the stored (enc:v1) API key.
 * Defaults to the real `decryptField` so shells without a main-process
 * key injection step (Wails) can resolve the plaintext key themselves;
 * tests inject fakes. Returning null/undefined, throwing, or returning
 * the stored value unchanged (a passthrough, not a decryption) keeps the
 * placeholder in place.
 */
export type WebSearchKeyDecrypt = (
  value: string | undefined,
) => Promise<string | undefined | null>;

export async function executeWebSearchProvider(
  bridge: NetcattyBridge,
  config: WebSearchConfig,
  query: string,
  maxResults: number,
  decrypt: WebSearchKeyDecrypt = decryptField,
): Promise<WebSearchResult[]> {
  const fn = PROVIDER_SEARCH_FNS[config.providerId];
  if (!fn) throw new Error(`Unsupported web search provider: ${config.providerId}`);

  // Resolve the real key renderer-side for shells without main-process
  // injection (Wails). A result equal to the stored value is a passthrough
  // (shell without a credentialsDecrypt bridge), never a decryption, so it
  // keeps the placeholder — ciphertext must not go out as the API key. On
  // decrypt failure or an empty result the placeholder is kept too, so the
  // Electron main process injection path still works.
  let apiKey = WEB_SEARCH_KEY_PLACEHOLDER;
  try {
    const realKey = await decrypt(config.apiKey);
    if (typeof realKey === 'string' && realKey.length > 0 && realKey !== config.apiKey) {
      apiKey = realKey;
    }
  } catch {
    // Decrypt failed - keep the placeholder.
  }

  // Best-effort allowlist self-service for user-configured API hosts before
  // the fetch (process-lifetime entries). netpolicy still adjudicates every
  // request fail-closed, so this call is advisory: ignore result and errors.
  if (typeof config.apiHost === 'string' && config.apiHost.length > 0) {
    const allowlistBridge = bridge as NetcattyBridge & {
      aiAllowlistAddHost?: (baseURL: string) => Promise<unknown>;
    };
    try {
      await allowlistBridge.aiAllowlistAddHost?.(config.apiHost);
    } catch {
      // Advisory only - never block the search on allowlist seeding.
    }
  }

  return fn(bridge, { ...config, apiKey }, query, maxResults);
}
