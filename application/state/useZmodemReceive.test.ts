import assert from 'node:assert/strict';
import { test } from 'node:test';
import { receiveZmodemIntoDirectory } from './useZmodemReceive';

test('explicit receive selects destination before starting Go and cancellation starts nothing', async () => {
  const calls: string[] = [];
  const bridge = {
    selectDirectory: async () => '/chosen',
    receiveZmodem: async (session: string, dir: string) => { calls.push(`${session}:${dir}`); return { success: true }; },
  };
  await receiveZmodemIntoDirectory(bridge, 's', 'Choose', () => calls.push('ready'));
  assert.deepEqual(calls, ['ready', 's:/chosen']);
  await receiveZmodemIntoDirectory({ ...bridge, selectDirectory: async () => null }, 's', 'Choose', () => calls.push('unexpected'));
  assert.deepEqual(calls, ['ready', 's:/chosen']);
});

test('explicit receive surfaces rejected transfers', async () => {
  await assert.rejects(receiveZmodemIntoDirectory({
    selectDirectory: async () => '/chosen', receiveZmodem: async () => ({ success: false, error: 'occupied' }),
  }, 's', 'Choose', () => undefined), /occupied/);
});
