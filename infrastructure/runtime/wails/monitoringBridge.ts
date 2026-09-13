type Method<K extends keyof NetcattyBridge> = NonNullable<NetcattyBridge[K]>;
export interface MonitoringBindings {
  GetServerStats?: Method<'getServerStats'>;
  ProbeSystemCapabilities?: Method<'probeSystemCapabilities'>;
  ListSystemProcesses?: Method<'listSystemProcesses'>;
  ListTmuxSessions?: Method<'listTmuxSessions'>;
  ListDockerContainers?: Method<'listDockerContainers'>;
  ListDockerImages?: Method<'listDockerImages'>;
  GetDockerStats?: Method<'getDockerStats'>;
}
export function createMonitoringBridge(bindings: MonitoringBindings, nativeSessionId: (id: string) => string) {
  async function invoke<T>(name: string, call: (() => Promise<T>) | undefined): Promise<T | { success: false; error: string }> {
    if (!call) return { success: false, error: `${name} unavailable in Wails bindings` };
    try { return await call(); }
    catch (error) { return { success: false, error: error instanceof Error ? error.message : String(error) }; }
  }
  return {
    getServerStats: (id: string) => invoke('GetServerStats', bindings.GetServerStats && (() => bindings.GetServerStats!(nativeSessionId(id)))),
    probeSystemCapabilities: (id: string) => invoke('ProbeSystemCapabilities', bindings.ProbeSystemCapabilities && (() => bindings.ProbeSystemCapabilities!(nativeSessionId(id)))),
    listSystemProcesses: (id: string) => invoke('ListSystemProcesses', bindings.ListSystemProcesses && (() => bindings.ListSystemProcesses!(nativeSessionId(id)))),
    listTmuxSessions: (id: string) => invoke('ListTmuxSessions', bindings.ListTmuxSessions && (() => bindings.ListTmuxSessions!(nativeSessionId(id)))),
    listDockerContainers: (id: string) => invoke('ListDockerContainers', bindings.ListDockerContainers && (() => bindings.ListDockerContainers!(nativeSessionId(id)))),
    listDockerImages: (id: string) => invoke('ListDockerImages', bindings.ListDockerImages && (() => bindings.ListDockerImages!(nativeSessionId(id)))),
    getDockerStats: (options: Parameters<Method<'getDockerStats'>>[0]) => invoke('GetDockerStats', bindings.GetDockerStats && (() => bindings.GetDockerStats!({ ...options, sessionId: nativeSessionId(options.sessionId) }))),
  };
}
