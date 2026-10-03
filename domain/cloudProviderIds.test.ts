import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import {
  BUILTIN_CLOUD_PROVIDERS,
  assertCloudProviderId,
  isBuiltinCloudProvider,
  isPluginCloudProviderId,
  legacyProviderConnectionStorageKey,
  providerConnectionStorageKey,
} from './cloudProviderIds';

describe('cloudProviderIds', () => {
  it('recognizes built-in providers', () => {
    for (const id of BUILTIN_CLOUD_PROVIDERS) {
      assert.equal(isBuiltinCloudProvider(id), true);
      assert.equal(isPluginCloudProviderId(id), false);
      // Writes target the renamed key...
      assert.equal(providerConnectionStorageKey(id), `lemonssh_provider_${id}_v1`);
      // ...while reads keep the pre-rename key as a fallback.
      assert.equal(legacyProviderConnectionStorageKey(id), `netcatty_provider_${id}_v1`);
    }
  });

  it('accepts namespaced plugin provider IDs without coercing them to built-ins', () => {
    const id = 'com.example.backup.sync';
    assert.equal(isPluginCloudProviderId(id), true);
    assert.equal(isBuiltinCloudProvider(id), false);
    assert.equal(providerConnectionStorageKey(id), `lemonssh_provider_plugin_v1:${id}`);
    assert.equal(legacyProviderConnectionStorageKey(id), `netcatty_provider_plugin_v1:${id}`);
    assert.equal(assertCloudProviderId(id), id);
  });

  it('rejects empty or NUL-containing provider IDs', () => {
    assert.throws(() => assertCloudProviderId(''), /invalid/i);
    assert.throws(() => assertCloudProviderId('bad\0id'), /invalid/i);
  });
});
