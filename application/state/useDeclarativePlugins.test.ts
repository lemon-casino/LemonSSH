import assert from 'node:assert/strict';
import test from 'node:test';
import { loadDeclarativePlugins } from './useDeclarativePlugins';

test('loads enabled plugin schemas and saves only through the runtime service', async () => {
 const values: Record<string,string|number|boolean> = {mode:'dark'};
 const runtime = {list:async()=>[{pluginId:'example',state:'enabled'},{pluginId:'disabled',state:'disabled'}],uiSchema:async(id:string)=>{assert.equal(id,'example');return {settings:[{id:'mode',type:'text' as const,label:'Mode'}]};},settings:async()=>({...values}),setSetting:async(id:string,key:string,json:string)=>{assert.equal(id,'example');values[key]=JSON.parse(json);},grantPermission:async()=>{throw Error('must not autoapprove');}};
 const plugins=await loadDeclarativePlugins(runtime);
 assert.equal(plugins.length,1);
 await plugins[0].onSettingChange('mode','light');
 assert.equal(values.mode,'light');
 assert.equal((await loadDeclarativePlugins(runtime))[0].values.mode,'light');
});

test('declared permission approval is explicit and session scoped', async () => {
 const grants:string[]=[];
 const client={list:async()=>[{pluginId:'example',state:'enabled',manifest:{permissions:[{kind:'terminal',resource:'session',mode:'read'}]}}],uiSchema:async()=>({}),settings:async()=>({}),setSetting:async()=>{},grantPermission:async(...args:string[])=>{grants.push(args.join('|'));}};
 const plugins=await loadDeclarativePlugins(client);
 assert.deepEqual(grants,[]);
 await plugins[0].onGrantPermission(plugins[0].permissions[0],'once');
 assert.deepEqual(grants,['example|terminal|session|read|once']);
});

test('invalid runtime inventory fails visibly instead of binding the wrong plugin', async () => {
 await assert.rejects(loadDeclarativePlugins({list:async()=>[{id:'wrong',state:'enabled'}]} as never),/inventory/);
});
