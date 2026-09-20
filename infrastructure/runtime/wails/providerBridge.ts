import { PROVIDER_PRESETS, type AIProviderId } from '../../ai/types';

interface FetchRequest {
  URL: string; Method: string; Headers: Record<string, string>; Body: string;
  ProviderID: string; SkipHostCheck: boolean; FollowRedirects: boolean; SkipTLSVerify: boolean;
}
interface FetchResult { OK: boolean; Status: number; Data: string; Error: string }
interface StreamResult { OK: boolean; StatusCode: number; StatusText: string; Error: string; Aborted: boolean }

export interface NativeProviderBindings {
  AllowlistAddHost: (url: string) => Promise<{ OK: boolean; Error: string }>;
  Fetch: (request: FetchRequest) => Promise<FetchResult>;
  ChatStream: (id: string, request: FetchRequest, idleTimeoutMs: number) => Promise<StreamResult>;
  ChatCancel: (id: string) => Promise<boolean>;
  SyncProviders: (providers: Array<{ ID: string; ProviderID: string; BaseURL: string; Enabled: boolean; APIKey: string; SkipTLSVerify: boolean; CustomHeaders: Record<string, string> }>) => Promise<{ OK: boolean; Error: string }>;
  SyncWebSearch: (apiHost: string, apiKey: string) => Promise<{ OK: boolean; Error: string }>;
}

type StreamEvent = { requestId: string; data?: string; event?: string; error?: string };

export function createProviderBridge(
  native: NativeProviderBindings,
  on: (name: string, callback: (event: { data?: unknown }) => void) => () => void,
): Partial<NetcattyBridge> {
  const subscribe = (name: string, id: string, callback: (event: StreamEvent) => void) => (
    on(name, event => {
      const payload = (Array.isArray(event.data) ? event.data[0] : event.data) as StreamEvent | undefined;
      if (payload?.requestId === id) callback(payload);
    })
  );
  return {
    aiAllowlistAddHost: async url => {
      const result = await native.AllowlistAddHost(url);
      return { ok: result.OK, error: result.Error || undefined };
    },
    aiFetch: async (url, method, headers, body, providerId, skipHostCheck, followRedirects, skipTLSVerify) => {
      const result = await native.Fetch({ URL: url, Method: method ?? '', Headers: headers ?? {}, Body: body ?? '', ProviderID: providerId ?? '', SkipHostCheck: skipHostCheck ?? false, FollowRedirects: followRedirects ?? false, SkipTLSVerify: skipTLSVerify ?? false });
      return { ok: result.OK, status: result.Status || 502, data: result.Data || (result.Error ? JSON.stringify({ error: { message: result.Error } }) : ''), error: result.Error || undefined };
    },
    aiChatStream: async (id, url, headers, body, providerId, idleTimeoutMs) => {
      const result = await native.ChatStream(id, { URL: url, Method: 'POST', Headers: headers ?? {}, Body: body ?? '', ProviderID: providerId ?? '', SkipHostCheck: false, FollowRedirects: false, SkipTLSVerify: false }, idleTimeoutMs ?? 0);
      return { ok: result.OK, statusCode: result.StatusCode || undefined, statusText: result.StatusText || undefined, error: result.Error || undefined, aborted: result.Aborted };
    },
    aiChatCancel: id => native.ChatCancel(id),
    onAiStreamData: (id, callback) => subscribe('ai:stream-data', id, event => callback(event.data ?? '', event.event)),
    onAiStreamEnd: (id, callback) => subscribe('ai:stream-end', id, () => callback()),
    onAiStreamError: (id, callback) => subscribe('ai:stream-error', id, event => callback(event.error ?? 'AI stream failed')),
    aiSyncProviders: async providers => {
      const result = await native.SyncProviders(providers.map(provider => {
        const options = provider as typeof provider & { skipTLSVerify?: boolean; customHeaders?: Record<string, string> };
        return {
          ID: provider.id, ProviderID: provider.providerId,
          BaseURL: provider.baseURL || PROVIDER_PRESETS[provider.providerId as AIProviderId]?.defaultBaseURL || 'https://api.openai.com/v1',
          Enabled: provider.enabled, APIKey: provider.apiKey ?? '', SkipTLSVerify: options.skipTLSVerify ?? false,
          CustomHeaders: options.customHeaders ?? {},
        };
      }));
      return { ok: result.OK };
    },
    aiSyncWebSearch: async (apiHost, apiKey) => {
      const result = await native.SyncWebSearch(apiHost ?? '', apiKey ?? '');
      return { ok: result.OK, error: result.Error || undefined };
    },
  };
}
