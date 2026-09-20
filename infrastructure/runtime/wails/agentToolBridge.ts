type Session = Parameters<NonNullable<NetcattyBridge['aiMcpUpdateSessions']>>[0][number];
type Attachment = { filename: string; mediaType: string; base64Data: string; filePath: string; sizeBytes: number };

function eventPayload<T>(event: { data?: unknown }): T {
  const data = event.data;
  return (Array.isArray(data) && data.length === 1 ? data[0] : data) as T;
}

export interface NativeAgentToolBindings {
  AgentExternalStatus?: () => ReturnType<NonNullable<NetcattyBridge['externalMcpGetStatus']>>;
  AgentExternalSetEnabled?: (enabled: boolean) => Promise<Record<string, unknown>>;
  AgentExternalSetConfig?: (config: { mode?: string; idleTimeoutMinutes?: number; sessionIdleTimeoutMinutes?: number }) => Promise<Record<string, unknown>>;
  AgentCapability?: (method: string, params: Record<string, unknown>, chat: string) => Promise<unknown>;
  AgentUpdateSessions?: (chat: string, sessions: Array<Session & { nativeSessionId: string }>, merge: boolean) => Promise<void>;
  AgentSetCancelled?: (chat: string, cancelled: boolean) => Promise<void>;
  AgentSetPermissionMode?: (mode: string) => Promise<void>;
  AgentSetCommandPolicy?: (patterns: string[] | null, seconds: number) => Promise<void>;
  AgentSyncPermissionGrants?: (grants: Array<Record<string, unknown>>) => Promise<void>;
  AgentRegisterChatAttachments?: (chat: string, attachments: Attachment[]) => Promise<void>;
  AgentRespondVault?: (id: string, result: Record<string, unknown>) => Promise<void>;
  AgentVaultRequestPending?: (id: string) => Promise<boolean>;
}

export function createAgentToolBridge(
  bindings: NativeAgentToolBindings | undefined,
  on: (name: string, callback: (event: { data?: unknown }) => void) => () => void,
  nativeSessionId: (id: string) => string,
): Partial<NetcattyBridge> {
  function method<K extends keyof NativeAgentToolBindings>(name: K): NonNullable<NativeAgentToolBindings[K]> {
    const fn = bindings?.[name];
    if (!fn) throw new Error(`Native agent method ${name} is unavailable`);
    return fn as NonNullable<NativeAgentToolBindings[K]>;
  }
  const update = async (sessions: Session[], chat = '', merge = false) => {
    await method('AgentUpdateSessions')(chat, sessions.map(session => ({ ...session, nativeSessionId: nativeSessionId(session.sessionId) })), merge);
    return { ok: true, count: sessions.length };
  };
  const cancel = async (chat: string, cancelled = true) => {
    await method('AgentSetCancelled')(chat, cancelled);
    return { ok: true };
  };
  const capability = async (name: string, params: Record<string, unknown>, chat = '') => (
    method('AgentCapability')(name, params, chat)
  );
  return {
    aiToolApprovalOwner: 'host',
    externalMcpGetStatus: () => method('AgentExternalStatus')(),
    externalMcpSetEnabled: enabled => method('AgentExternalSetEnabled')(enabled),
    externalMcpSetConfig: config => method('AgentExternalSetConfig')(config),
    onAgentInteractionCleared: callback => on('agent:interaction-cleared', event => callback(eventPayload(event))),
    aiCapability: capability,
    aiExec: async (id, command, chat) => {
      try {
        const result = await capability('netcatty/exec', { sessionId: id, command }, chat) as {
          ok: boolean; stdout?: string; stderr?: string; output?: string; exitCode?: number; exitCodeKnown?: boolean;
        };
        return { ...result, stdout: result.stdout ?? result.output ?? '', stderr: result.stderr ?? '', exitCode: result.exitCodeKnown === false ? null : result.exitCode };
      } catch (error) { return { ok: false, error: error instanceof Error ? error.message : String(error) }; }
    },
    aiMcpUpdateSessions: (sessions, chat) => update(sessions, chat),
    aiMcpUpdateLiveSessions: sessions => update(sessions),
    aiMcpMergeSessions: (sessions, chat) => update(sessions, chat, true),
    aiSetChatSessionCancelled: cancel,
    aiCattyCancelExec: chat => cancel(chat),
    aiMcpSetPermissionMode: async mode => { await method('AgentSetPermissionMode')(mode); return { ok: true }; },
    aiMcpSetCommandBlocklist: async patterns => { await method('AgentSetCommandPolicy')(patterns, 0); return { ok: true }; },
    aiMcpSetCommandTimeout: async seconds => { await method('AgentSetCommandPolicy')(null, seconds); return { ok: true }; },
    aiMcpSyncPermissionGrants: async grants => { await method('AgentSyncPermissionGrants')(grants); return { ok: true, count: grants.length }; },
    aiMcpUpdateAttachments: async (attachments, chat = '') => {
      await method('AgentRegisterChatAttachments')(chat, attachments.map(file => ({
        filename: file.filename ?? 'attachment', mediaType: file.mediaType ?? 'application/octet-stream',
        base64Data: file.base64Data ?? '', filePath: file.filePath ?? '', sizeBytes: 0,
      })));
      return { ok: true };
    },
    onVaultAgentRequest: callback => on('agent:vault-request', event => callback(eventPayload(event))),
    isVaultAgentRequestPending: id => method('AgentVaultRequestPending')(id),
    respondVaultAgent: async (id, result) => { await method('AgentRespondVault')(id, result); return { ok: true }; },
  };
}
