import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createLocalShellBridge } from './localShellBridge';

test('native local launch preserves argv and terminal environment and attaches route', async () => {
 const calls: unknown[] = [];
 const bridge = createLocalShellBridge({
  StartLocalWithOptions: async request => { calls.push(request); return 'native-1'; },
  GetDefaultShell: async () => '/bin/sh', DiscoverShells: async () => [],
  ValidatePath: async () => ({ exists: true, isFile: true, isDirectory: false, isExecutable: true }),
 }, async (alias,id) => { calls.push([alias,id]); });
 assert.equal(await bridge.startLocalSession({sessionId:'ui-1',shell:'/bin/sh',shellArgs:['-c','printf "%s" "a b"'],env:{TERM:'vt100'},cwd:'/home/me'}),'native-1');
 assert.deepEqual(calls,[{shell:'/bin/sh',shellArgs:['-c','printf "%s" "a b"'],env:{TERM:'vt100'},cwd:'/home/me',cols:80,rows:24},['ui-1','native-1']]);
 assert.equal(await bridge.getDefaultShell(),'/bin/sh');
 assert.equal((await bridge.validatePath('/bin/sh','file')).isExecutable,true);
});
