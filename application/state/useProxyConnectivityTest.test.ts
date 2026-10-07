import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

test('proxy connectivity test is wired through the Wails terminal probe', () => {
  const hookSource = readFileSync(new URL('./useProxyConnectivityTest.ts', import.meta.url), 'utf8');
  const clientSource = readFileSync(new URL('../../infrastructure/runtime/wails/wailsRuntimeClient.ts', import.meta.url), 'utf8');
  const panelSource = readFileSync(new URL('../../components/host-details/ProxyPanel.tsx', import.meta.url), 'utf8');
  const managerSource = readFileSync(new URL('../../components/ProxyProfilesManager.tsx', import.meta.url), 'utf8');
  assert.match(hookSource, /testProxy\?\.\(/);
  assert.match(clientSource, /const testProxy = /);
  assert.match(clientSource, /bindings\.terminal\.TestProxy/);
  assert.match(panelSource, /hostDetails\.proxyPanel\.test/);
  assert.match(managerSource, /hostDetails\.proxyPanel\.test/);
});
