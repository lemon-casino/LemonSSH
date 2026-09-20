type Method<K extends keyof NetcattyBridge> = NonNullable<NetcattyBridge[K]>;
export interface MonitoringBindings {
  GetServerStats?: Method<'getServerStats'>;
  ProbeSystemCapabilities?: Method<'probeSystemCapabilities'>;
  ListSystemProcesses?: Method<'listSystemProcesses'>;
  SignalSystemProcess?: Method<'signalSystemProcess'>;
  SetupOsc7Tracking?: (sessionId: string, command: string) => Promise<{ success: boolean; output?: string; error?: string }>;
  ListTmuxSessions?: Method<'listTmuxSessions'>;
  CreateTmuxSession?: Method<'createTmuxSession'>;
  ListTmuxWindows?: Method<'listTmuxWindows'>;
  ListTmuxPanes?: Method<'listTmuxPanes'>;
  ListTmuxClients?: Method<'listTmuxClients'>;
  TmuxAction?: Method<'tmuxAction'>;
  ListDockerContainers?: Method<'listDockerContainers'>;
  ListDockerImages?: Method<'listDockerImages'>;
  GetDockerStats?: Method<'getDockerStats'>;
  ListAccelerators?: Method<'listAccelerators'>;
  ListListeningPorts?: Method<'listListeningPorts'>;
  ListSystemServices?: Method<'listSystemServices'>;
  SystemServiceAction?: Method<'systemServiceAction'>;
  DockerInspect?: Method<'dockerInspect'>;
  DockerImageInspect?: Method<'dockerImageInspect'>;
  DockerAction?: Method<'dockerAction'>;
  DockerImageAction?: Method<'dockerImageAction'>;
}

export function createMonitoringBridge(bindings: MonitoringBindings, nativeSessionId: (id: string) => string) {
  async function invoke<T>(name: string, call: (() => Promise<T>) | undefined): Promise<T | { success: false; error: string }> {
    if (!call) return { success: false, error: `${name} unavailable in Wails bindings` };
    try { return await call(); }
    catch (error) { return { success: false, error: error instanceof Error ? error.message : String(error) }; }
  }
  const withSession = <T extends { sessionId: string }>(options: T): T => ({
    ...options,
    sessionId: nativeSessionId(options.sessionId),
  });
  return {
    getServerStats: (id: string) => invoke('GetServerStats', bindings.GetServerStats && (() => bindings.GetServerStats!(nativeSessionId(id)))),
    probeSystemCapabilities: (id: string) => invoke('ProbeSystemCapabilities', bindings.ProbeSystemCapabilities && (() => bindings.ProbeSystemCapabilities!(nativeSessionId(id)))),
    listSystemProcesses: (id: string) => invoke('ListSystemProcesses', bindings.ListSystemProcesses && (() => bindings.ListSystemProcesses!(nativeSessionId(id)))),
    signalSystemProcess: (options: Parameters<Method<'signalSystemProcess'>>[0]) => invoke('SignalSystemProcess', bindings.SignalSystemProcess && (() => bindings.SignalSystemProcess!(withSession(options)))),
    setupOsc7Tracking: async (id: string, command: string) => {
      const result = await invoke('SetupOsc7Tracking', bindings.SetupOsc7Tracking && (() => bindings.SetupOsc7Tracking!(nativeSessionId(id), command)));
      if ('success' in result && result.success) return { ...result, stdout: 'output' in result ? result.output : undefined, code: 0 };
      return result;
    },
    listTmuxSessions: (id: string) => invoke('ListTmuxSessions', bindings.ListTmuxSessions && (() => bindings.ListTmuxSessions!(nativeSessionId(id)))),
    createTmuxSession: (options: Parameters<Method<'createTmuxSession'>>[0]) => invoke('CreateTmuxSession', bindings.CreateTmuxSession && (() => bindings.CreateTmuxSession!(withSession(options)))),
    listTmuxWindows: (options: Parameters<Method<'listTmuxWindows'>>[0]) => invoke('ListTmuxWindows', bindings.ListTmuxWindows && (() => bindings.ListTmuxWindows!(withSession(options)))),
    listTmuxPanes: (options: Parameters<Method<'listTmuxPanes'>>[0]) => invoke('ListTmuxPanes', bindings.ListTmuxPanes && (() => bindings.ListTmuxPanes!(withSession(options)))),
    listTmuxClients: (options: Parameters<Method<'listTmuxClients'>>[0]) => invoke('ListTmuxClients', bindings.ListTmuxClients && (() => bindings.ListTmuxClients!(withSession(options)))),
    tmuxAction: (options: Parameters<Method<'tmuxAction'>>[0]) => invoke('TmuxAction', bindings.TmuxAction && (() => bindings.TmuxAction!(withSession(options)))),
    listDockerContainers: (id: string) => invoke('ListDockerContainers', bindings.ListDockerContainers && (() => bindings.ListDockerContainers!(nativeSessionId(id)))),
    listDockerImages: (id: string) => invoke('ListDockerImages', bindings.ListDockerImages && (() => bindings.ListDockerImages!(nativeSessionId(id)))),
    getDockerStats: (options: Parameters<Method<'getDockerStats'>>[0]) => invoke('GetDockerStats', bindings.GetDockerStats && (() => bindings.GetDockerStats!(withSession(options)))),
    listAccelerators: (id: string) => invoke('ListAccelerators', bindings.ListAccelerators && (() => bindings.ListAccelerators!(nativeSessionId(id)))),
    listListeningPorts: (id: string) => invoke('ListListeningPorts', bindings.ListListeningPorts && (() => bindings.ListListeningPorts!(nativeSessionId(id)))),
    listSystemServices: (id: string) => invoke('ListSystemServices', bindings.ListSystemServices && (() => bindings.ListSystemServices!(nativeSessionId(id)))),
    systemServiceAction: (options: Parameters<Method<'systemServiceAction'>>[0]) => invoke('SystemServiceAction', bindings.SystemServiceAction && (() => bindings.SystemServiceAction!(withSession(options)))),
    dockerInspect: (options: Parameters<Method<'dockerInspect'>>[0]) => invoke('DockerInspect', bindings.DockerInspect && (() => bindings.DockerInspect!(withSession(options)))),
    dockerImageInspect: (options: Parameters<Method<'dockerImageInspect'>>[0]) => invoke('DockerImageInspect', bindings.DockerImageInspect && (() => bindings.DockerImageInspect!(withSession(options)))),
    dockerAction: (options: Parameters<Method<'dockerAction'>>[0]) => invoke('DockerAction', bindings.DockerAction && (() => bindings.DockerAction!(withSession(options)))),
    dockerImageAction: (options: Parameters<Method<'dockerImageAction'>>[0]) => invoke('DockerImageAction', bindings.DockerImageAction && (() => bindings.DockerImageAction!(withSession(options)))),
  };
}
