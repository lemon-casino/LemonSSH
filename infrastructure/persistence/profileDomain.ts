const VAULT_KEYS = new Set([
  "lemonssh_hosts_v1",
  "lemonssh_keys_v1",
  "lemonssh_groups_v1",
  "lemonssh_snippets_v1",
  "lemonssh_snippet_packages_v1",
  "lemonssh_notes_v1",
  "lemonssh_note_groups_v1",
  "lemonssh_snippet_var_values_v1",
  "lemonssh_known_hosts_v1",
  "lemonssh_identities_v1",
  "lemonssh_proxy_profiles_v1",
  "lemonssh_port_forwarding_v1",
  "lemonssh_default_key_passphrases_v1",
  "lemonssh_sftp_global_bookmarks_v1",
  "lemonssh_sftp_local_bookmarks_v1",
  "lemonssh_sftp_file_associations_v1",
  "lemonssh_sftp_host_view_modes_v1",
  "lemonssh_sftp_transfer_center_v1",
]);

const SESSION_KEYS = new Set([
  "lemonssh_session_restore_v1",
  "lemonssh_connection_logs_v1",
  "lemonssh_connection_log_terminal_data_v1",
  "lemonssh_shell_history_v1",
]);

export function profileDomainForKey(key: string): "vault" | "sessions" | "settings" {
  if (VAULT_KEYS.has(key)) return "vault";
  if (SESSION_KEYS.has(key)) return "sessions";
  return "settings";
}

// Domains the canonical cutover (SYNC-01) hydrates from the Go profile store.
// The "logs" and "plugin-v1" domains are host-internal and never surface as
// hook-readable storage keys.
export const CANONICAL_PROFILE_DOMAINS = ["settings", "vault", "sessions"] as const;

// AI-related storage stays localStorage-canonical until P6-05 lands; it must
// never be promoted into the Go profile store nor hydrated out of it.
const AI_DEBUG_KEYS = new Set(["lemonssh.aiDebug.hide", "lemonssh.aiDebug.profile"]);

export function isAIManagedStorageKey(key: string): boolean {
  return key.startsWith("lemonssh_ai_") || AI_DEBUG_KEYS.has(key);
}
