type EventSubscription = (name: string, callback: (event: { data?: unknown }) => void) => () => void;

interface NativeExternalAgentBindings {
  Stream(request: Record<string, unknown>): Promise<{ ok: boolean; error?: string }>;
  Cancel(requestId: string, chatSessionId: string): Promise<{ ok: boolean; error?: string }>;
  Cleanup(chatSessionId: string): Promise<{ ok: boolean; error?: string }>;
  Steer(requestId: string, chatSessionId: string, prompt: string, images: Array<Record<string, unknown>>, clientUserMessageId: string): Promise<{ status: string; message?: string; turnKind?: string }>;
  ListModels(sdkBackend: string, cwd: string, providerId: string, chatSessionId: string, agentEnv: Record<string, string>, agentCommand: string, codexRuntime: string): Promise<{ ok: boolean; models?: Array<Record<string, unknown>>; currentModelId?: string; warning?: string; error?: string }>;
  CodexAppServerStatus(agentCommand: string, agentEnv: Record<string, string>): Promise<{ ok: boolean; error?: string }>;
  AccountInfo(agentEnv: Record<string, string>, agentCommand: string): Promise<Record<string, unknown>>;
}

function payloadFrom(event: { data?: unknown } | unknown): Record<string, unknown> {
  const raw = (event as { data?: unknown })?.data ?? event;
  const payload = Array.isArray(raw) ? raw[0] : raw;
  return payload && typeof payload === 'object' ? payload as Record<string, unknown> : {};
}

export function createExternalAgentBridge(
  bindings: NativeExternalAgentBindings | undefined,
  on: EventSubscription,
): Partial<NetcattyBridge> {
  if (!bindings) return {};
  const subscribe = (name: string, requestId: string, callback: (payload: Record<string, unknown>) => void) => on(name, event => {
    const payload = payloadFrom(event);
    if (payload.requestId === requestId) callback(payload);
  });
  return {
    aiSdkAgentStream: async (
      requestId, chatSessionId, sdkBackend, prompt, cwd, providerId, model,
      existingSessionId, historyMessages, images, toolIntegrationMode,
      defaultTargetSession, userSkillsContext, agentEnv, agentCommand,
      codexRuntime, permissionMode, codebuddyOptions,
    ) => bindings.Stream({
      requestId, chatSessionId, sdkBackend, prompt,
      cwd: cwd ?? '', providerId: providerId ?? '', model: model ?? '',
      existingSessionId: existingSessionId ?? '', historyMessages: historyMessages ?? [],
      images: images ?? [], toolIntegrationMode: toolIntegrationMode ?? 'mcp',
      defaultTargetSession: defaultTargetSession ?? null,
      userSkillsContext: userSkillsContext ?? '', agentEnv: agentEnv ?? {},
      agentCommand: agentCommand ?? '', codexRuntime: codexRuntime ?? 'sdk',
      permissionMode: permissionMode ?? 'confirm', codebuddyOptions: codebuddyOptions ?? {},
    }),
    aiSdkAgentCancel: (requestId, chatSessionId) => bindings.Cancel(requestId, chatSessionId ?? ''),
    aiSdkAgentCleanup: async chatSessionId => {
      const result = await bindings.Cleanup(chatSessionId);
      return { ok: result.ok };
    },
    aiSdkAgentSteer: async (requestId, chatSessionId, prompt, images, clientUserMessageId) => {
      const result = await bindings.Steer(requestId, chatSessionId, prompt, images ?? [], clientUserMessageId);
      return result as Awaited<ReturnType<NonNullable<NetcattyBridge['aiSdkAgentSteer']>>>;
    },
    aiSdkAgentListModels: async (sdkBackend, cwd, providerId, chatSessionId, agentEnv, agentCommand, codexRuntime) => {
      const result = await bindings.ListModels(sdkBackend, cwd ?? '', providerId ?? '', chatSessionId ?? '', agentEnv ?? {}, agentCommand ?? '', codexRuntime ?? 'sdk');
      return result as Awaited<ReturnType<NonNullable<NetcattyBridge['aiSdkAgentListModels']>>>;
    },
    codexAppServerGetStatus: async (agentCommand, agentEnv) => {
      const result = await bindings.CodexAppServerStatus(agentCommand ?? '', agentEnv ?? {});
      return { ok: result.ok, available: result.ok, error: result.error };
    },
    aiSdkAgentAccountInfo: async (agentEnv, agentCommand) => {
      const result = await bindings.AccountInfo(agentEnv ?? {}, agentCommand ?? '');
      return result as Awaited<ReturnType<NonNullable<NetcattyBridge['aiSdkAgentAccountInfo']>>>;
    },
    onAiSdkAgentEvent: (requestId, callback) => subscribe('ai:sdk-agent:event', requestId, payload => {
      const event = payload.event;
      if (event && typeof event === 'object') callback(event as Record<string, unknown>);
    }),
    onAiSdkAgentDone: (requestId, callback) => subscribe('ai:sdk-agent:done', requestId, () => callback()),
    onAiSdkAgentError: (requestId, callback) => subscribe('ai:sdk-agent:error', requestId, payload => callback(String(payload.error ?? 'External agent failed'))),
  };
}
