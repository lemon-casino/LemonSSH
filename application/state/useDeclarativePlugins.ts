import { useEffect, useState } from 'react';
import { getPluginV2Client, type PluginV2Client } from '../../infrastructure/runtime/pluginClient';

type Permission = {kind:string;resource:string;mode:string};

export async function loadDeclarativePlugins(client: PluginV2Client) {
 const records = await client.list();
 if (!Array.isArray(records)) throw new Error('Invalid plugin inventory');
 return Promise.all(records.filter(record => {
  if (!record || typeof record !== 'object' || !('pluginId' in record) || typeof record.pluginId !== 'string' || !('state' in record)) throw new Error('Invalid plugin inventory identity');
  return record.state === 'enabled';
 }).map(async record => {
  const pluginId = (record as {pluginId:string}).pluginId;
  const [schema,values] = await Promise.all([client.uiSchema(pluginId),client.settings(pluginId)]);
  if (!schema || typeof schema !== 'object' || !values || typeof values !== 'object') throw new Error('Invalid plugin UI response');
  const rawManifest=(record as {manifest?:unknown}).manifest;
  const manifest=typeof rawManifest === 'string' ? JSON.parse(rawManifest) : rawManifest;
  const permissions:Permission[] = manifest && typeof manifest === 'object' && 'permissions' in manifest && Array.isArray(manifest.permissions) ? manifest.permissions.filter((p:unknown):p is Permission=>!!p && typeof p==='object' && 'kind' in p && typeof p.kind==='string' && 'resource' in p && typeof p.resource==='string' && 'mode' in p && (p.mode==='read'||p.mode==='write')) : [];
  return {pluginId,schema,values,permissions,onGrantPermission:async(permission:Permission,lifetime:'once'|'session')=>{await client.grantPermission(pluginId,permission.kind,permission.resource,permission.mode,lifetime);},data:values as Record<string,unknown>,onSettingChange:async(id:string,value:string|number|boolean)=>{await client.setSetting(pluginId,id,JSON.stringify(value));}};
 }));
}

export function useDeclarativePlugins() {
 const client=getPluginV2Client();
 const [plugins,setPlugins]=useState<Awaited<ReturnType<typeof loadDeclarativePlugins>>>([]);
 const [error,setError]=useState<unknown>(null);
 useEffect(()=>{
  let active=true;
  if(client) void loadDeclarativePlugins(client).then(result=>{if(active)setPlugins(result);},cause=>{if(active)setError(cause);});
  return ()=>{active=false;};
 },[client]);
 return {available:!!client,plugins,error};
}
