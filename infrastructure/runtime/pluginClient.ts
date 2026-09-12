import type { DeclarativePluginUI } from '@netcatty/plugin-contract';
import { getActiveRuntimeClient } from './runtimeClient';

export interface PluginV2Client {
 list(): Promise<unknown[]>;
 uiSchema(id: string): Promise<DeclarativePluginUI>;
 settings(id: string): Promise<Record<string, string | number | boolean>>;
 setSetting(id: string, key: string, valueJSON: string): Promise<void>;
 grantPermission(id: string, kind: string, resource: string, mode: string, lifetime: string): Promise<void>;
}

export function getPluginV2Client(): PluginV2Client | undefined {
 return getActiveRuntimeClient()?.transitionBridge.pluginV2;
}
