import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createMonitoringBridge } from './monitoringBridge';

test('monitoring resolves independent session aliases and Docker options', async () => {
  const aliases: Record<string, string> = { a: 'ssh-a', b: 'ssh-b' };
  const bridge = createMonitoringBridge({
    GetServerStats: async (id) => ({ success: false, error: id }),
    GetDockerStats: async ({ sessionId, ids }) => ({ success: false, error: `${sessionId}:${ids?.join(',')}` }),
  }, id => aliases[id] ?? id);
  assert.deepEqual(await bridge.getServerStats('a'), { success: false, error: 'ssh-a' });
  assert.deepEqual(await bridge.getServerStats('b'), { success: false, error: 'ssh-b' });
  assert.deepEqual(await bridge.getDockerStats({ sessionId: 'b', ids: ['$(bad)'] }), { success: false, error: 'ssh-b:$(bad)' });
});
test('monitoring exposes missing bindings and rejected calls as failures', async () => {
  const bridge = createMonitoringBridge({ ListTmuxSessions: async () => { throw new Error('permission denied'); } }, id => id);
  assert.deepEqual(await bridge.listTmuxSessions('x'), { success: false, error: 'permission denied' });
  const result = await bridge.listDockerImages('x');
  assert.equal(result.success, false);
  assert.match(result.error!, /unavailable/);
});
