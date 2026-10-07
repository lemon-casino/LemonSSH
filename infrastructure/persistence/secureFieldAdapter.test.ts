import assert from "node:assert/strict";
import { test } from "node:test";

import { decryptProviderSecrets, encryptProviderSecrets } from "./secureFieldAdapter";
import type { ProviderConnection } from "../../domain/sync";

// The bridge is unavailable under node:test, so encryptField/decryptField
// degrade to pass-through. That still exercises every envelope-shape rule:
// marker detection, single-key invariant, and the write-current/read-legacy
// behavior — only the ciphertext layer is a no-op.

const NEW_CONFIG_KEY = "__lemonssh_plugin_config_v1";
const LEGACY_NETCATTY_CONFIG_KEY = "__netcatty_plugin_config_v1";
const OLDEST_CONFIG_KEY = "__encryptedPluginConfig";
const NEW_CREDENTIAL_KEY = "__lemonssh_plugin_credential_v1";
const LEGACY_NETCATTY_CREDENTIAL_KEY = "__netcatty_plugin_credential_v1";

const pluginConnection = (overrides: Record<string, unknown>): ProviderConnection =>
  ({ id: "conn-1", provider: "my-plugin", ...overrides }) as unknown as ProviderConnection;

test("decrypt accepts current, netcatty-era and original config envelope markers", async () => {
  const opaque = JSON.stringify({ endpoint: "https://p.example", apiKey: "k" });

  const current = await decryptProviderSecrets(pluginConnection({ config: { [NEW_CONFIG_KEY]: opaque } }));
  assert.deepEqual(current.config, { endpoint: "https://p.example", apiKey: "k" });

  const netcattyEra = await decryptProviderSecrets(
    pluginConnection({ config: { [LEGACY_NETCATTY_CONFIG_KEY]: opaque } }),
  );
  assert.deepEqual(netcattyEra.config, { endpoint: "https://p.example", apiKey: "k" });

  const oldest = await decryptProviderSecrets(pluginConnection({ config: { [OLDEST_CONFIG_KEY]: opaque } }));
  assert.deepEqual(oldest.config, { endpoint: "https://p.example", apiKey: "k" });
});

test("decrypt accepts current and netcatty-era credential envelope markers", async () => {
  const durableRef = JSON.stringify({ kind: "secret", id: "secret-1", key: "k" });

  const current = await decryptProviderSecrets(
    pluginConnection({ credential: { [NEW_CREDENTIAL_KEY]: durableRef } }),
  );
  assert.deepEqual(current.credential, { kind: "secret", id: "secret-1", key: "k" });

  const netcattyEra = await decryptProviderSecrets(
    pluginConnection({ credential: { [LEGACY_NETCATTY_CREDENTIAL_KEY]: durableRef } }),
  );
  assert.deepEqual(netcattyEra.credential, { kind: "secret", id: "secret-1", key: "k" });
});

test("encrypt always writes the new config envelope marker", async () => {
  const opaque = { endpoint: "https://p.example", apiKey: "k" };

  const sealed = await encryptProviderSecrets(pluginConnection({ config: opaque }));

  assert.deepEqual(Object.keys(sealed.config as object), [NEW_CONFIG_KEY]);
  assert.deepEqual(
    JSON.parse((sealed.config as Record<string, string>)[NEW_CONFIG_KEY]),
    opaque,
  );
});

test("encrypt re-seals a netcatty-era config envelope under the new marker", async () => {
  const opaque = { endpoint: "https://p.example", apiKey: "k" };

  const resealed = await encryptProviderSecrets(
    pluginConnection({ config: { [LEGACY_NETCATTY_CONFIG_KEY]: JSON.stringify(opaque) } }),
  );

  assert.deepEqual(Object.keys(resealed.config as object), [NEW_CONFIG_KEY]);
  assert.deepEqual(
    JSON.parse((resealed.config as Record<string, string>)[NEW_CONFIG_KEY]),
    opaque,
  );
});

test("encrypt re-seals the original encryptedPluginConfig envelope under the new marker", async () => {
  const opaque = { endpoint: "https://p.example" };

  const resealed = await encryptProviderSecrets(
    pluginConnection({ config: { [OLDEST_CONFIG_KEY]: JSON.stringify(opaque) } }),
  );

  assert.deepEqual(Object.keys(resealed.config as object), [NEW_CONFIG_KEY]);
  assert.deepEqual(
    JSON.parse((resealed.config as Record<string, string>)[NEW_CONFIG_KEY]),
    opaque,
  );
});

test("encrypt seals a re-encrypted netcatty-era credential envelope under the new marker", async () => {
  const durableRef = { kind: "secret", id: "secret-1", key: "k" };

  const resealed = await encryptProviderSecrets(
    pluginConnection({ credential: { [LEGACY_NETCATTY_CREDENTIAL_KEY]: JSON.stringify(durableRef) } }),
  );

  assert.deepEqual(Object.keys(resealed.credential as object), [NEW_CREDENTIAL_KEY]);
  assert.deepEqual(
    JSON.parse((resealed.credential as Record<string, string>)[NEW_CREDENTIAL_KEY]),
    durableRef,
  );
});

test("marker-colliding plugin JSON is never treated as an envelope", async () => {
  // A plugin-owned object that merely contains a similar key (plus extra
  // properties) must be sealed as opaque JSON, not unwrapped.
  const colliding = { [NEW_CONFIG_KEY]: "not-a-seal", extra: 1 };

  const sealed = await encryptProviderSecrets(pluginConnection({ config: colliding }));

  assert.deepEqual(Object.keys(sealed.config as object), [NEW_CONFIG_KEY]);
  assert.deepEqual(
    JSON.parse((sealed.config as Record<string, string>)[NEW_CONFIG_KEY]),
    colliding,
  );

  // Decrypt leaves it sealed (single-key invariant broken by `extra`).
  const leftSealed = await decryptProviderSecrets(pluginConnection({ config: colliding }));
  assert.deepEqual(leftSealed.config, colliding);
});

test("encrypt keeps plain JSON values decryptable without losing the single-key invariant", async () => {
  const plain = { authType: "token", token: "t" };

  const sealed = await encryptProviderSecrets(pluginConnection({ config: plain }));
  const roundTrip = await decryptProviderSecrets(sealed);

  assert.deepEqual(roundTrip.config, plain);
});
