import assert from 'node:assert/strict';
import test from 'node:test';
import { createAgentCliBridge } from './agentCliBridge';

test('selected directory path is passed to native discovery and normalized', async () => {
  const calls: unknown[][] = [];
  const bridge = createAgentCliBridge({
    Resolve: async (...args) => {
      calls.push(args);
      return { Path: 'C:\\tools\\codex.exe', BinPath: 'C:\\tools\\codex.exe', Version: 'codex 1.2.3', Available: true, Installed: true };
    },
    Discover: async () => [],
    Prewarm: async () => ({ OK: true }),
  });
  const result = await bridge.aiResolveCli!({ command: 'codex', customPath: 'C:\\tools', refreshShellEnv: true });
  assert.deepEqual(calls, [['codex', 'C:\\tools', true, false]]);
  assert.equal(result.path, 'C:\\tools\\codex.exe');
  assert.equal(result.available, true);
});

test('native agent discovery adds renderer metadata', async () => {
  const bridge = createAgentCliBridge({
    Resolve: async () => ({}),
    Discover: async () => [{ Command: 'claude', Path: '/tools/claude', Version: '2.0.0', Available: true, Installed: true }],
    Prewarm: async () => ({ OK: true }),
  });
  const result = await bridge.aiDiscoverAgents!();
  assert.equal(result[0].command, 'claude');
  assert.equal(result[0].name, 'Claude Code');
  assert.equal(result[0].binPath, null);
});
