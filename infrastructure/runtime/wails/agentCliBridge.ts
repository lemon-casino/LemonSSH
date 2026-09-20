import type { DiscoveredAgent } from '../../ai/types';

type ResolvedAgentCLI = Awaited<ReturnType<NonNullable<NetcattyBridge['aiResolveCli']>>>;

export interface NativeAgentCLIPathInfo {
  Path?: string;
  path?: string;
  BinPath?: string;
  binPath?: string;
  Version?: string | null;
  version?: string | null;
  Available?: boolean;
  available?: boolean;
  Installed?: boolean;
  installed?: boolean;
  Authenticated?: boolean;
  authenticated?: boolean;
  AuthSource?: string | null;
  authSource?: string | null;
  CLIEmail?: string;
  cliEmail?: string;
  CLIBinPath?: string;
  cliBinPath?: string;
  CLILoginOK?: boolean;
  cliLoginOk?: boolean;
  APIKeyOK?: boolean;
  apiKeyOk?: boolean;
  SDKInstalled?: boolean;
  sdkInstalled?: boolean;
  Command?: string;
  command?: string;
}

export interface NativeCodexIntegrationOptions {
  refreshShellEnv?: boolean;
  validateChatGptAuth?: boolean;
  codexPath?: string;
}

export interface NativeCodexLoginSession {
  sessionId: string;
  state: 'running' | 'success' | 'error' | 'cancelled';
  url?: string;
  output: string;
  error?: string;
  exitCode?: number | null;
  codexPath?: string;
}

export interface NativeAgentCLIBindings {
  CodexGetIntegration: (options: NativeCodexIntegrationOptions) => Promise<{
    state: string;
    isConnected: boolean;
    rawOutput: string;
    exitCode?: number | null;
    customConfig?: Record<string, unknown>;
  }>;
  CodexStartLogin: (options: NativeCodexIntegrationOptions) => Promise<{ ok: boolean; session?: NativeCodexLoginSession; error?: string }>;
  CodexGetLoginSession: (sessionID: string) => Promise<{ ok: boolean; found?: boolean; session?: NativeCodexLoginSession; error?: string }>;
  CodexCancelLogin: (sessionID: string) => Promise<{ ok: boolean; found?: boolean; session?: NativeCodexLoginSession; error?: string }>;
  CodexLogout: (options: NativeCodexIntegrationOptions) => Promise<{
    ok: boolean;
    state?: string;
    isConnected?: boolean;
    rawOutput?: string;
    logoutOutput?: string;
    error?: string;
  }>;
  Resolve: (command: string, customPath: string, refreshShellEnv: boolean, apiKeyPresent: boolean) => Promise<NativeAgentCLIPathInfo>;
  Discover: (refreshShellEnv: boolean, apiKeyPresent: boolean) => Promise<NativeAgentCLIPathInfo[]>;
  Prewarm: () => Promise<{ OK?: boolean; ok?: boolean }>;
}

const metadata: Record<string, Pick<DiscoveredAgent, 'name' | 'icon' | 'description' | 'args' | 'sdkBackend'>> = {
  codex: { name: 'Codex CLI', icon: 'openai', description: 'OpenAI Codex CLI', args: ['exec', '--full-auto', '--json', '{prompt}'], sdkBackend: 'codex' },
  claude: { name: 'Claude Code', icon: 'claude', description: 'Anthropic Claude Code', args: ['-p', '--output-format', 'text', '{prompt}'], sdkBackend: 'claude' },
  copilot: { name: 'GitHub Copilot CLI', icon: 'copilot', description: 'GitHub Copilot CLI', args: ['-p', '{prompt}'], sdkBackend: 'copilot' },
  cursor: { name: 'Cursor', icon: 'cursor', description: 'Cursor Agent CLI', args: ['{prompt}'], sdkBackend: 'cursor' },
  codebuddy: { name: 'CodeBuddy Code', icon: 'codebuddy', description: 'CodeBuddy Code CLI', args: ['-p', '{prompt}'], sdkBackend: 'codebuddy' },
  opencode: { name: 'OpenCode', icon: 'opencode', description: 'OpenCode CLI', args: ['run', '{prompt}'], sdkBackend: 'opencode' },
  grok: { name: 'Grok Build', icon: 'grok', description: 'Grok Build CLI', args: ['{prompt}'], sdkBackend: 'grok' },
};

function normalize(result: NativeAgentCLIPathInfo | null | undefined): ResolvedAgentCLI {
  return {
    path: result?.Path ?? result?.path ?? null,
    binPath: result?.BinPath ?? result?.binPath ?? null,
    version: result?.Version ?? result?.version ?? null,
    available: result?.Available ?? result?.available ?? false,
    installed: result?.Installed ?? result?.installed ?? false,
    authenticated: result?.Authenticated ?? result?.authenticated ?? false,
    authSource: result?.AuthSource ?? result?.authSource ?? null,
    cliEmail: result?.CLIEmail ?? result?.cliEmail ?? null,
    cliBinPath: result?.CLIBinPath ?? result?.cliBinPath ?? null,
    cliLoginOk: result?.CLILoginOK ?? result?.cliLoginOk ?? false,
    apiKeyOk: result?.APIKeyOK ?? result?.apiKeyOk ?? false,
    sdkInstalled: result?.SDKInstalled ?? result?.sdkInstalled ?? false,
  };
}

export function createAgentCliBridge(bindings: NativeAgentCLIBindings | undefined): Partial<NetcattyBridge> {
  const required = () => {
    if (!bindings) throw new Error('Native agent CLI discovery is unavailable');
    return bindings;
  };
  const options = (value?: NativeCodexIntegrationOptions): NativeCodexIntegrationOptions => ({
    refreshShellEnv: value?.refreshShellEnv ?? false,
    validateChatGptAuth: value?.validateChatGptAuth ?? false,
    codexPath: value?.codexPath ?? '',
  });
  return {
    aiCodexGetIntegration: async value => {
      const result = await required().CodexGetIntegration(options(value));
      return {
        state: result.state as Awaited<ReturnType<NonNullable<NetcattyBridge['aiCodexGetIntegration']>>>['state'],
        isConnected: result.isConnected,
        rawOutput: result.rawOutput,
        exitCode: result.exitCode ?? null,
        customConfig: result.customConfig as Awaited<ReturnType<NonNullable<NetcattyBridge['aiCodexGetIntegration']>>>['customConfig'],
      };
    },
    aiCodexStartLogin: async value => required().CodexStartLogin(options(value)) as ReturnType<NonNullable<NetcattyBridge['aiCodexStartLogin']>>,
    aiCodexGetLoginSession: async sessionID => required().CodexGetLoginSession(sessionID) as ReturnType<NonNullable<NetcattyBridge['aiCodexGetLoginSession']>>,
    aiCodexCancelLogin: async sessionID => required().CodexCancelLogin(sessionID) as ReturnType<NonNullable<NetcattyBridge['aiCodexCancelLogin']>>,
    aiCodexLogout: async value => required().CodexLogout(options(value)) as ReturnType<NonNullable<NetcattyBridge['aiCodexLogout']>>,
    aiResolveCli: async params => normalize(await required().Resolve(
      params.command,
      params.customPath ?? '',
      params.refreshShellEnv ?? false,
      params.apiKeyPresent ?? false,
    )),
    aiDiscoverAgents: async options => {
      const found = await required().Discover(options?.refreshShellEnv ?? false, options?.apiKeyPresent ?? false);
      return found.flatMap(result => {
        const command = result.Command ?? result.command ?? '';
        const info = normalize(result);
        const details = metadata[command];
        if (!details || !info.available || !info.path) return [];
        return [{ command, ...details, ...info, path: info.path, version: info.version ?? '' }];
      });
    },
    aiPrewarmShellEnv: async () => {
      const result = await required().Prewarm();
      return { ok: result.OK ?? result.ok ?? false };
    },
  };
}
