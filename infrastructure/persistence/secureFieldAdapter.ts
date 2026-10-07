/**
 * Secure Field Adapter — Renderer-side helpers for field-level encryption
 *
 * Encrypts / decrypts individual sensitive fields within domain models before
 * they are written to (or after they are read from) localStorage.
 *
 * The heavy lifting is done by Electron's safeStorage via the credential
 * bridge IPC.  When the bridge is unavailable (web fallback, tests) every
 * function degrades to a no-op — values pass through unmodified.
 */

import type { GroupConfig, Host, Identity, ProxyProfile, SSHKey } from "../../domain/models";
import type { ProviderConnection, S3Config, WebDAVConfig } from "../../domain/sync";
import { lemonsshBridge } from "../services/lemonsshBridge";

// ---------------------------------------------------------------------------
// Primitive helpers
// ---------------------------------------------------------------------------

const bridge = () => lemonsshBridge.get();

export async function encryptField(value: string | undefined): Promise<string | undefined> {
  if (!value) return value;
  const b = bridge();
  if (!b?.credentialsEncrypt) return value;
  return b.credentialsEncrypt(value);
}

export async function decryptField(value: string | undefined): Promise<string | undefined> {
  if (!value) return value;
  const b = bridge();
  if (!b?.credentialsDecrypt) return value;
  return b.credentialsDecrypt(value);
}

// ---------------------------------------------------------------------------
// Host
// ---------------------------------------------------------------------------

export async function encryptHostSecrets(host: Host): Promise<Host> {
  const out = { ...host };
  out.password = await encryptField(out.password);
  out.telnetPassword = await encryptField(out.telnetPassword);
  if (out.proxyConfig?.password) {
    out.proxyConfig = { ...out.proxyConfig, password: await encryptField(out.proxyConfig.password) };
  }
  return out;
}

export async function decryptHostSecrets(host: Host): Promise<Host> {
  const out = { ...host };
  out.password = await decryptField(out.password);
  out.telnetPassword = await decryptField(out.telnetPassword);
  if (out.proxyConfig?.password) {
    out.proxyConfig = { ...out.proxyConfig, password: await decryptField(out.proxyConfig.password) };
  }
  return out;
}

// ---------------------------------------------------------------------------
// SSHKey
// ---------------------------------------------------------------------------

export async function encryptKeySecrets(key: SSHKey): Promise<SSHKey> {
  const out = { ...key };
  out.passphrase = await encryptField(out.passphrase);
  out.privateKey = (await encryptField(out.privateKey)) ?? "";
  return out;
}

export async function decryptKeySecrets(key: SSHKey): Promise<SSHKey> {
  const out = { ...key };
  out.passphrase = await decryptField(out.passphrase);
  out.privateKey = (await decryptField(out.privateKey)) ?? "";
  return out;
}

// ---------------------------------------------------------------------------
// Identity
// ---------------------------------------------------------------------------

export async function encryptIdentitySecrets(identity: Identity): Promise<Identity> {
  const out = { ...identity };
  out.password = await encryptField(out.password);
  return out;
}

export async function decryptIdentitySecrets(identity: Identity): Promise<Identity> {
  const out = { ...identity };
  out.password = await decryptField(out.password);
  return out;
}

// ---------------------------------------------------------------------------
// GroupConfig
// ---------------------------------------------------------------------------

export async function encryptGroupConfigSecrets(config: GroupConfig): Promise<GroupConfig> {
  const out = { ...config };
  out.password = await encryptField(out.password);
  out.telnetPassword = await encryptField(out.telnetPassword);
  if (out.proxyConfig?.password) {
    out.proxyConfig = { ...out.proxyConfig, password: await encryptField(out.proxyConfig.password) };
  }
  return out;
}

export async function decryptGroupConfigSecrets(config: GroupConfig): Promise<GroupConfig> {
  const out = { ...config };
  out.password = await decryptField(out.password);
  out.telnetPassword = await decryptField(out.telnetPassword);
  if (out.proxyConfig?.password) {
    out.proxyConfig = { ...out.proxyConfig, password: await decryptField(out.proxyConfig.password) };
  }
  return out;
}

export function encryptGroupConfigs(configs: GroupConfig[]): Promise<GroupConfig[]> {
  return Promise.all(configs.map(encryptGroupConfigSecrets));
}

export function decryptGroupConfigs(configs: GroupConfig[]): Promise<GroupConfig[]> {
  return Promise.all(configs.map(decryptGroupConfigSecrets));
}

// ---------------------------------------------------------------------------
// ProxyProfile
// ---------------------------------------------------------------------------

export async function encryptProxyProfileSecrets(profile: ProxyProfile): Promise<ProxyProfile> {
  const out = { ...profile, config: { ...profile.config } };
  out.config.password = await encryptField(out.config.password);
  return out;
}

export async function decryptProxyProfileSecrets(profile: ProxyProfile): Promise<ProxyProfile> {
  const out = { ...profile, config: { ...profile.config } };
  out.config.password = await decryptField(out.config.password);
  return out;
}

export function encryptProxyProfiles(profiles: ProxyProfile[]): Promise<ProxyProfile[]> {
  return Promise.all(profiles.map(encryptProxyProfileSecrets));
}

export function decryptProxyProfiles(profiles: ProxyProfile[]): Promise<ProxyProfile[]> {
  return Promise.all(profiles.map(decryptProxyProfileSecrets));
}

// ---------------------------------------------------------------------------
// Provider Connection (Cloud Sync)
// ---------------------------------------------------------------------------

/**
 * Host-owned sealed-config envelope. Must be unambiguous against plugin-owned
 * JSON: exactly one reserved key, no extra properties. Never treat a plugin
 * object that merely contains a similar key as already sealed.
 *
 * compat#10/#3: the marker was renamed `__netcatty_plugin_config_v1` →
 * `__lemonssh_plugin_config_v1` (and likewise for the credential envelope).
 * Serialization always writes the new marker; decryption still accepts the
 * legacy markers (this file's original `__encryptedPluginConfig` hop shows the
 * pattern) so persisted ProviderConnection values and cloud snapshots sealed
 * under the old names keep decrypting and upgrade on the next write-back.
 */
const PLUGIN_CONFIG_ENVELOPE_KEY = "__lemonssh_plugin_config_v1" as const;
const LEGACY_NETCATTY_PLUGIN_CONFIG_ENVELOPE_KEY = "__netcatty_plugin_config_v1" as const;
const LEGACY_PLUGIN_CONFIG_ENVELOPE_KEY = "__encryptedPluginConfig" as const;
/** At-rest envelope for ProviderConnection.credential (opaque refs only). */
const PLUGIN_CREDENTIAL_ENVELOPE_KEY = "__lemonssh_plugin_credential_v1" as const;
const LEGACY_NETCATTY_PLUGIN_CREDENTIAL_ENVELOPE_KEY = "__netcatty_plugin_credential_v1" as const;

type PluginConfigEnvelope = {
  [PLUGIN_CONFIG_ENVELOPE_KEY]: string;
};

/** Every accepted config-envelope marker, current name first. */
const PLUGIN_CONFIG_ENVELOPE_KEYS: readonly string[] = [
  PLUGIN_CONFIG_ENVELOPE_KEY,
  LEGACY_NETCATTY_PLUGIN_CONFIG_ENVELOPE_KEY,
  LEGACY_PLUGIN_CONFIG_ENVELOPE_KEY,
];

/** Every accepted credential-envelope marker, current name first. */
const PLUGIN_CREDENTIAL_ENVELOPE_KEYS: readonly string[] = [
  PLUGIN_CREDENTIAL_ENVELOPE_KEY,
  LEGACY_NETCATTY_PLUGIN_CREDENTIAL_ENVELOPE_KEY,
];

/**
 * Return the single reserved envelope marker when `value` is an exact
 * marker-shaped single-key object with a string payload, else null. The
 * single-key invariant is kept across marker renames.
 */
function sealedEnvelopeKey(value: unknown, accepted: readonly string[]): string | null {
  if (value == null || typeof value !== "object" || Array.isArray(value)) return null;
  const record = value as Record<string, unknown>;
  const keys = Object.keys(record);
  if (keys.length !== 1) return null;
  const key = keys[0];
  if (!accepted.includes(key)) return null;
  return typeof record[key] === "string" ? key : null;
}

function isPluginConfigEnvelope(value: unknown): value is PluginConfigEnvelope {
  return sealedEnvelopeKey(value, PLUGIN_CONFIG_ENVELOPE_KEYS) === PLUGIN_CONFIG_ENVELOPE_KEY;
}

/** Legacy envelope shapes (still accepted on decrypt for the migration hops). */
function isLegacyPluginConfigEnvelope(
  value: unknown,
): value is { __encryptedPluginConfig: string } | { __netcatty_plugin_config_v1: string } {
  const key = sealedEnvelopeKey(value, PLUGIN_CONFIG_ENVELOPE_KEYS);
  return key !== null && key !== PLUGIN_CONFIG_ENVELOPE_KEY;
}

/** Any accepted credential-envelope marker key on `value`, else null. */
function sealedCredentialEnvelopeKey(value: unknown): string | null {
  return sealedEnvelopeKey(value, PLUGIN_CREDENTIAL_ENVELOPE_KEYS);
}

export async function encryptProviderSecrets(conn: ProviderConnection): Promise<ProviderConnection> {
  const out = { ...conn };

  if (out.tokens) {
    const t = { ...out.tokens };
    t.accessToken = (await encryptField(t.accessToken)) ?? "";
    t.refreshToken = await encryptField(t.refreshToken);
    out.tokens = t;
  }

  // Config may be a valid falsy scalar (false, 0, "") — only null/undefined means absent.
  if (out.config != null) {
    const providerId = String(out.provider ?? "");
    const isBuiltin = providerId === "webdav"
      || providerId === "s3"
      || providerId === "github"
      || providerId === "google"
      || providerId === "onedrive";
    // Built-in providers use field-level encryption; plugin IDs always seal
    // the whole opaque config so field-name collisions cannot leak secrets.
    if (isBuiltin && typeof out.config === "object" && "authType" in out.config) {
      const c = { ...out.config } as WebDAVConfig;
      c.password = await encryptField(c.password);
      c.token = await encryptField(c.token);
      out.config = c;
    } else if (isBuiltin && typeof out.config === "object" && "secretAccessKey" in out.config) {
      const c = { ...out.config } as S3Config;
      c.secretAccessKey = (await encryptField(c.secretAccessKey)) ?? "";
      c.sessionToken = await encryptField(c.sessionToken);
      out.config = c;
    } else if (!isBuiltin) {
      // Always (re)seal opaque plugin config. An exact marker-shaped object may
      // be either a trusted host envelope (current or legacy marker) or
      // plugin-owned JSON that collides with our key — try unwrap; on failure
      // seal the whole value as opaque.
      let toSeal: unknown = out.config;
      const envelopeKey = sealedEnvelopeKey(out.config, PLUGIN_CONFIG_ENVELOPE_KEYS);
      if (envelopeKey !== null) {
        const sealedValue = (out.config as Record<string, unknown>)[envelopeKey] as string;
        const plain = await decryptField(sealedValue);
        if (plain != null && plain !== "") {
          try {
            toSeal = JSON.parse(plain);
          } catch {
            toSeal = out.config;
          }
        } else {
          toSeal = out.config;
        }
      }
      const sealed = await encryptField(JSON.stringify(toSeal));
      if (sealed) {
        out.config = {
          [PLUGIN_CONFIG_ENVELOPE_KEY]: sealed,
        } as ProviderConnection["config"];
      }
    }
  }

  // Seal durable plugin credential refs as one opaque blob (same threat model
  // as plugin config: do not leave kind/id/key plaintext in localStorage).
  if (out.credential != null && typeof out.credential === "object") {
    let toSeal: unknown = out.credential;
    const credentialEnvelopeKey = sealedCredentialEnvelopeKey(out.credential);
    if (credentialEnvelopeKey !== null) {
      const plain = await decryptField(
        (out.credential as Record<string, unknown>)[credentialEnvelopeKey] as string,
      );
      if (plain != null && plain !== "") {
        try {
          toSeal = JSON.parse(plain);
        } catch {
          toSeal = out.credential;
        }
      } else {
        toSeal = out.credential;
      }
    }
    const kind = (toSeal as { kind?: unknown }).kind;
    const id = (toSeal as { id?: unknown }).id;
    const key = (toSeal as { key?: unknown }).key;
    if ((kind === "secret" || kind === "credential") && typeof id === "string" && id.length > 0) {
      const normalized = {
        kind,
        id,
        ...(typeof key === "string" ? { key } : {}),
      };
      const sealed = await encryptField(JSON.stringify(normalized));
      if (sealed) {
        out.credential = {
          [PLUGIN_CREDENTIAL_ENVELOPE_KEY]: sealed,
        } as unknown as ProviderConnection["credential"];
      }
    } else if (sealedCredentialEnvelopeKey(toSeal) !== null) {
      // Marker-collision object that is not a durable ref (current or legacy
      // marker whose unwrap failed) — seal as opaque JSON under the new key.
      const sealed = await encryptField(JSON.stringify(toSeal));
      if (sealed) {
        out.credential = {
          [PLUGIN_CREDENTIAL_ENVELOPE_KEY]: sealed,
        } as unknown as ProviderConnection["credential"];
      }
    } else {
      // Drop leases / malformed shapes — never persist them at rest.
      delete out.credential;
    }
  }

  return out;
}

export async function decryptProviderSecrets(conn: ProviderConnection): Promise<ProviderConnection> {
  const out = { ...conn };

  if (out.tokens) {
    const t = { ...out.tokens };
    t.accessToken = (await decryptField(t.accessToken)) ?? "";
    t.refreshToken = await decryptField(t.refreshToken);
    out.tokens = t;
  }

  // Config may be a valid falsy scalar — only null/undefined means absent.
  if (out.config != null) {
    const providerId = String(out.provider ?? "");
    const isBuiltin = providerId === "webdav"
      || providerId === "s3"
      || providerId === "github"
      || providerId === "google"
      || providerId === "onedrive";
    if (isBuiltin && typeof out.config === "object" && "authType" in out.config) {
      const c = { ...out.config } as WebDAVConfig;
      c.password = await decryptField(c.password);
      c.token = await decryptField(c.token);
      out.config = c;
    } else if (isBuiltin && typeof out.config === "object" && "secretAccessKey" in out.config) {
      const c = { ...out.config } as S3Config;
      c.secretAccessKey = (await decryptField(c.secretAccessKey)) ?? "";
      c.sessionToken = await decryptField(c.sessionToken);
      out.config = c;
    } else if (isPluginConfigEnvelope(out.config) || isLegacyPluginConfigEnvelope(out.config)) {
      const envelopeKey = sealedEnvelopeKey(out.config, PLUGIN_CONFIG_ENVELOPE_KEYS) as string;
      const sealed = (out.config as Record<string, unknown>)[envelopeKey] as string;
      const plain = await decryptField(sealed);
      // plain may be JSON "false"/"0"/'""' — treat empty decrypt as failure only.
      if (plain != null && plain !== "") {
        try {
          out.config = JSON.parse(plain) as ProviderConnection["config"];
        } catch {
          // leave sealed if corrupt
        }
      }
    }
  }

  const credentialEnvelopeKey = sealedCredentialEnvelopeKey(out.credential);
  if (credentialEnvelopeKey !== null && out.credential != null && typeof out.credential === "object") {
    const plain = await decryptField(
      (out.credential as Record<string, unknown>)[credentialEnvelopeKey] as string,
    );
    if (plain != null && plain !== "") {
      try {
        const parsed = JSON.parse(plain) as {
          kind?: unknown;
          id?: unknown;
          key?: unknown;
        };
        if (
          (parsed.kind === "secret" || parsed.kind === "credential")
          && typeof parsed.id === "string"
          && parsed.id.length > 0
        ) {
          out.credential = {
            kind: parsed.kind,
            id: parsed.id,
            ...(typeof parsed.key === "string" ? { key: parsed.key } : {}),
          };
        } else {
          delete out.credential;
        }
      } catch {
        // leave sealed if corrupt
      }
    }
  }

  return out;
}

// ---------------------------------------------------------------------------
// Batch helpers
// ---------------------------------------------------------------------------

export function encryptHosts(hosts: Host[]): Promise<Host[]> {
  return Promise.all(hosts.map(encryptHostSecrets));
}

export function decryptHosts(hosts: Host[]): Promise<Host[]> {
  return Promise.all(hosts.map(decryptHostSecrets));
}

export function encryptKeys(keys: SSHKey[]): Promise<SSHKey[]> {
  return Promise.all(keys.map(encryptKeySecrets));
}

export function decryptKeys(keys: SSHKey[]): Promise<SSHKey[]> {
  return Promise.all(keys.map(decryptKeySecrets));
}

export function encryptIdentities(identities: Identity[]): Promise<Identity[]> {
  return Promise.all(identities.map(encryptIdentitySecrets));
}

export function decryptIdentities(identities: Identity[]): Promise<Identity[]> {
  return Promise.all(identities.map(decryptIdentitySecrets));
}
