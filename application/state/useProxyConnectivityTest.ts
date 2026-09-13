import { useCallback, useState } from 'react';

import { isProxyCommandConfig, resolveProxyConfigAuth } from '../../domain/proxyProfiles';
import type { Identity, ProxyConfig } from '../../types';
import { netcattyBridge } from '../../infrastructure/services/netcattyBridge';

export type ProxyConnectivityStatus = 'idle' | 'testing' | 'ok' | 'error';

export interface ProxyConnectivityState {
  status: ProxyConnectivityStatus;
  message?: string;
  latencyMs?: number;
}

export function useProxyConnectivityTest(identities: Identity[] = []) {
  const [state, setState] = useState<ProxyConnectivityState>({ status: 'idle' });

  const testConfig = useCallback(async (config: ProxyConfig | undefined, targetHost?: string, targetPort?: number) => {
    if (!config) {
      setState({ status: 'error', message: 'missing' });
      return;
    }
    const resolved = resolveProxyConfigAuth(config, identities);
    const kind = isProxyCommandConfig(resolved) ? 'command' : resolved.type;
    setState({ status: 'testing' });
    try {
      const result = await netcattyBridge.require().testProxy?.({
        kind,
        host: resolved.host,
        port: resolved.port,
        username: resolved.username,
        password: resolved.password,
        command: resolved.command,
        targetHost,
        targetPort,
      });
      if (!result) {
        setState({ status: 'error', message: 'unavailable' });
        return;
      }
      if (result.ok) {
        setState({ status: 'ok', latencyMs: result.latencyMs });
        return;
      }
      setState({ status: 'error', message: result.error || 'failed', latencyMs: result.latencyMs });
    } catch (error) {
      setState({
        status: 'error',
        message: error instanceof Error ? error.message : String(error),
      });
    }
  }, [identities]);

  return { state, testConfig };
}
