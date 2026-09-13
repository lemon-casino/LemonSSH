import assert from 'node:assert/strict';
import test from 'node:test';
import { hasProviderConnectionData, isCloudProviderConnectDisabled, isProviderReadyForSync } from './sync';

test('hasProviderConnectionData treats falsy scalar configs as present', () => {
  assert.equal(hasProviderConnectionData({ config: false }), true);
  assert.equal(hasProviderConnectionData({ config: 0 }), true);
  assert.equal(hasProviderConnectionData({ config: '' }), true);
  assert.equal(hasProviderConnectionData({ config: null as never }), true);
  assert.equal(hasProviderConnectionData({}), false);
  assert.equal(hasProviderConnectionData({ tokens: undefined }), false);
});

test('isProviderReadyForSync keeps error status ready when only scalar config remains', () => {
  assert.equal(
    isProviderReadyForSync({ status: 'error', config: false }),
    true,
  );
  assert.equal(
    isProviderReadyForSync({ status: 'error', config: 0 }),
    true,
  );
  assert.equal(
    isProviderReadyForSync({ status: 'error', config: '' }),
    true,
  );
  assert.equal(
    isProviderReadyForSync({ status: 'error' }),
    false,
  );
  assert.equal(
    isProviderReadyForSync({ status: 'disconnected', config: false }),
    false,
  );
});

test('a ready provider does not grey out Connect on the other cloud services', () => {
  assert.equal(
    isCloudProviderConnectDisabled({
      provider: 'google',
      connection: { status: 'disconnected' },
      pendingConnectProvider: null,
      hasConnectingProvider: false,
    }),
    false,
  );
  assert.equal(
    isCloudProviderConnectDisabled({
      provider: 'onedrive',
      connection: { status: 'disconnected' },
      pendingConnectProvider: null,
      hasConnectingProvider: false,
    }),
    false,
  );
  assert.equal(
    isCloudProviderConnectDisabled({
      provider: 'google',
      connection: { status: 'disconnected' },
      pendingConnectProvider: 'github',
      hasConnectingProvider: true,
    }),
    true,
  );
  assert.equal(
    isCloudProviderConnectDisabled({
      provider: 'github',
      connection: { status: 'connecting' },
      pendingConnectProvider: 'github',
      hasConnectingProvider: true,
    }),
    true,
  );
});
