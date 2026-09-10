const VAULT_KEYS = new Set([
  "netcatty_hosts_v1",
  "netcatty_keys_v1",
  "netcatty_groups_v1",
  "netcatty_snippets_v1",
  "netcatty_snippet_packages_v1",
  "netcatty_notes_v1",
  "netcatty_note_groups_v1",
  "netcatty_snippet_var_values_v1",
  "netcatty_known_hosts_v1",
  "netcatty_identities_v1",
  "netcatty_proxy_profiles_v1",
  "netcatty_port_forwarding_v1",
]);

const SESSION_KEYS = new Set([
  "netcatty_session_restore_v1",
  "netcatty_connection_logs_v1",
  "netcatty_connection_log_terminal_data_v1",
  "netcatty_shell_history_v1",
]);

export function profileDomainForKey(key: string): "vault" | "sessions" | "settings" {
  if (VAULT_KEYS.has(key)) return "vault";
  if (SESSION_KEYS.has(key)) return "sessions";
  return "settings";
}
