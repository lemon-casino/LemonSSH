export const STORAGE_KEY_WORKBENCH_SESSION_TREE_WIDTH = 'lemonssh_workbench_session_tree_width_v1';
export const STORAGE_KEY_HOSTS = 'lemonssh_hosts_v1';
export const STORAGE_KEY_KEYS = 'lemonssh_keys_v1';
export const STORAGE_KEY_GROUPS = 'lemonssh_groups_v1';
export const STORAGE_KEY_CUSTOM_GROUPS = STORAGE_KEY_GROUPS;
export const STORAGE_KEY_SNIPPETS = 'lemonssh_snippets_v1';
export const STORAGE_KEY_SNIPPET_PACKAGES = 'lemonssh_snippet_packages_v1';
export const STORAGE_KEY_NOTES = 'lemonssh_notes_v1';
export const STORAGE_KEY_NOTE_GROUPS = 'lemonssh_note_groups_v1';
/** Last-filled values per snippet id for {{variable}} placeholders. */
export const STORAGE_KEY_SNIPPET_VAR_VALUES = 'lemonssh_snippet_var_values_v1';
export const STORAGE_KEY_THEME = 'lemonssh_theme_v1';
export const STORAGE_KEY_COLOR = 'lemonssh_color_v1';
export const STORAGE_KEY_ACCENT_MODE = 'lemonssh_accent_mode_v1';
export const STORAGE_KEY_UI_THEME_LIGHT = 'lemonssh_ui_theme_light_v1';
export const STORAGE_KEY_UI_THEME_DARK = 'lemonssh_ui_theme_dark_v1';
/** Mirrors each scheme's resolved {background, accent} so the boot splash (index.html early script) matches the real UI theme before React mounts. */
export const STORAGE_KEY_BOOT_THEME = 'lemonssh_boot_theme_v1';
export const STORAGE_KEY_UI_FONT_FAMILY = 'lemonssh_ui_font_family_v1';
export const STORAGE_KEY_SYNC = 'lemonssh_sync_v1';
/** Per-provider OAuth client IDs (public values; users register their own desktop-app clients). */
export const STORAGE_KEY_SYNC_OAUTH_CLIENT_IDS = 'lemonssh_sync_oauth_client_ids_v1';
/** Google desktop-app client secret (installed clients still send it on token exchange). */
export const STORAGE_KEY_SYNC_OAUTH_CLIENT_SECRETS = 'lemonssh_sync_oauth_client_secrets_v1';
/** Device-local experimental convergent-sync toggle; never included in SyncPayload settings. */
export const STORAGE_KEY_CONVERGENT_SYNC_CONFIG = 'lemonssh_convergent_sync_config_v2';
export const STORAGE_KEY_TERM_THEME = 'lemonssh_term_theme_v1';
export const STORAGE_KEY_TERM_FOLLOW_APP_THEME = 'lemonssh_term_follow_app_theme_v1';
export const STORAGE_KEY_TERM_THEME_DARK = 'lemonssh_term_theme_dark_v1';
export const STORAGE_KEY_TERM_THEME_LIGHT = 'lemonssh_term_theme_light_v1';
export const STORAGE_KEY_TERM_FONT_FAMILY = 'lemonssh_term_font_family_v1';
export const STORAGE_KEY_TERM_FONT_SIZE = 'lemonssh_term_font_size_v1';
export const STORAGE_KEY_TERM_SETTINGS = 'lemonssh_term_settings_v1';
export const STORAGE_KEY_HOTKEY_SCHEME = 'lemonssh_hotkey_scheme_v1';
export const STORAGE_KEY_CUSTOM_KEY_BINDINGS = 'lemonssh_custom_key_bindings_v1';
export const STORAGE_KEY_HOTKEY_RECORDING = 'lemonssh_hotkey_recording_v1';
export const STORAGE_KEY_CUSTOM_CSS = 'lemonssh_custom_css_v1';
export const STORAGE_KEY_UI_LANGUAGE = 'lemonssh_ui_language_v1';
export const STORAGE_KEY_PORT_FORWARDING = 'lemonssh_port_forwarding_v1';
/** Width (px) shared by port forwarding edit, wizard, and host picker panels. */
export const STORAGE_KEY_PORT_FORWARDING_PANEL_WIDTH = 'lemonssh_port_forwarding_panel_width_v1';
export const STORAGE_KEY_PF_PREFER_FORM_MODE = 'lemonssh_pf_prefer_form_mode_v1';
export const STORAGE_KEY_PF_VIEW_MODE = 'lemonssh_pf_view_mode_v1';
export const STORAGE_KEY_KNOWN_HOSTS = 'lemonssh_known_hosts_v1';
export const STORAGE_KEY_SHELL_HISTORY = 'lemonssh_shell_history_v1';
export const STORAGE_KEY_CONNECTION_LOGS = 'lemonssh_connection_logs_v1';
/** Side store for unsaved connection-log terminal replay buffers (main blob omits them for perf). */
export const STORAGE_KEY_CONNECTION_LOG_TERMINAL_DATA = 'lemonssh_connection_log_terminal_data_v1';
export const STORAGE_KEY_SESSION_RESTORE = 'lemonssh_session_restore_v1';
export const STORAGE_KEY_RESTORE_PREVIOUS_SESSION = 'lemonssh_restore_previous_session_v1';
export const STORAGE_KEY_RESTORE_TERMINAL_CWD = 'lemonssh_restore_terminal_cwd_v1';
/** Cold-start landing: vault (home) or local terminal when nothing is restored. */
export const STORAGE_KEY_STARTUP_LANDING = 'lemonssh_startup_landing_v1';
export const STORAGE_KEY_IDENTITIES = 'lemonssh_identities_v1';
export const STORAGE_KEY_PROXY_PROFILES = 'lemonssh_proxy_profiles_v1';
export const STORAGE_KEY_VAULT_HOSTS_VIEW_MODE = 'lemonssh_vault_hosts_view_mode_v1';
export const STORAGE_KEY_VAULT_HOSTS_SORT_MODE = 'lemonssh_vault_hosts_sort_mode_v1';
export const STORAGE_KEY_VAULT_HOSTS_TREE_EXPANDED = 'lemonssh_vault_hosts_tree_expanded_v1';
export const STORAGE_KEY_VAULT_SIDEBAR_COLLAPSED = 'lemonssh_vault_sidebar_collapsed_v1';
export const STORAGE_KEY_VAULT_SIDEBAR_WIDTH = 'lemonssh_vault_sidebar_width_v1';
export const STORAGE_KEY_VAULT_KEYS_VIEW_MODE = 'lemonssh_vault_keys_view_mode_v1';
export const STORAGE_KEY_VAULT_PROXY_PROFILES_VIEW_MODE = 'lemonssh_vault_proxy_profiles_view_mode_v1';
export const STORAGE_KEY_VAULT_SNIPPETS_VIEW_MODE = 'lemonssh_vault_snippets_view_mode_v1';
export const STORAGE_KEY_VAULT_NOTES_VIEW_MODE = 'lemonssh_vault_notes_view_mode_v1';
export const STORAGE_KEY_VAULT_NOTES_EDITOR_MODE = 'lemonssh_vault_notes_editor_mode_v1';
export const STORAGE_KEY_VAULT_NOTES_SELECTED_GROUP = 'lemonssh_vault_notes_selected_group_v1';
export const STORAGE_KEY_VAULT_NOTES_TREE_WIDTH = 'lemonssh_vault_notes_tree_width_v1';
export const STORAGE_KEY_VAULT_NOTES_FONT_FAMILY = 'lemonssh_vault_notes_font_family_v1';
export const STORAGE_KEY_VAULT_NOTES_FONT_SIZE = 'lemonssh_vault_notes_font_size_v1';
export const STORAGE_KEY_VAULT_NOTES_CODE_FONT_SIZE = 'lemonssh_vault_notes_code_font_size_v1';
/** Inline snippet/script edit panel width (px). */
export const STORAGE_KEY_SNIPPETS_PANEL_WIDTH = 'lemonssh_snippets_panel_width_v1';
/** Inline vault host/group details panel width (px). */
export const STORAGE_KEY_VAULT_HOST_PANEL_WIDTH = 'lemonssh_vault_host_panel_width_v1';
/** Inline snippet script editor height (px) in vault edit panel. */
export const STORAGE_KEY_SNIPPET_SCRIPT_EDITOR_HEIGHT = 'lemonssh_snippet_script_editor_height_v1';
/** Automation script Monaco editor height (px) in vault sidebar. */
export const STORAGE_KEY_SCRIPT_EDITOR_HEIGHT = 'lemonssh_script_editor_height_v1';
/** Terminal compose bar total height (px). */
export const STORAGE_KEY_COMPOSE_BAR_HEIGHT = 'lemonssh_compose_bar_height_v1';
/** Snippet IDs pinned to the terminal compose bar quick strip. */
export const STORAGE_KEY_COMPOSE_BAR_PINNED_SNIPPETS = 'lemonssh_compose_bar_pinned_snippets_v1';
export const STORAGE_KEY_VAULT_KNOWN_HOSTS_VIEW_MODE = 'lemonssh_vault_known_hosts_view_mode_v1';
/** Device-local: silently import system OpenSSH known_hosts on Vault load (default true). */
export const STORAGE_KEY_AUTO_IMPORT_SYSTEM_KNOWN_HOSTS = 'lemonssh_auto_import_system_known_hosts_v1';

// Update check
export const STORAGE_KEY_UPDATE_LAST_CHECK = 'lemonssh_update_last_check_v1';
export const STORAGE_KEY_UPDATE_DISMISSED_VERSION = 'lemonssh_update_dismissed_version_v1';
export const STORAGE_KEY_UPDATE_LATEST_RELEASE = 'lemonssh_update_latest_release_v1';
export const STORAGE_KEY_AUTO_UPDATE_ENABLED = 'lemonssh_auto_update_enabled_v1';
export const STORAGE_KEY_LOCAL_VAULT_BACKUP_MAX_COUNT = 'lemonssh_local_vault_backup_max_count_v1';
export const STORAGE_KEY_LOCAL_VAULT_BACKUP_LAST_APP_VERSION = 'lemonssh_local_vault_backup_last_app_version_v1';

/**
 * Cross-window barrier: set while a local vault restore is applying so
 * auto-sync in another window doesn't upload a pre-restore snapshot
 * concurrently. The value is an epoch-ms deadline — auto-sync treats any
 * value in the future as "restore in progress" and any value in the past
 * as a stale lock that can be ignored. See useAutoSync and
 * CloudSyncSettings for readers/writers.
 */
export const STORAGE_KEY_VAULT_RESTORE_IN_PROGRESS_UNTIL = 'lemonssh_vault_restore_in_progress_until_v1';

/**
 * Apply-in-progress sentinel. Set before a destructive applySyncPayload
 * starts writing and cleared after it completes successfully. If this
 * value is present on a later startup, the previous apply was
 * interrupted mid-way (renderer crash, power loss, IPC failure) and the
 * local vault is a partial mix of pre-apply and post-apply state.
 * Auto-sync must refuse to push in that window — otherwise the partial
 * state would silently overwrite an intact cloud copy — until the user
 * manually restores from a protective backup or completes a full merge.
 * The value is a JSON-encoded record (startedAt, protectiveBackupId,
 * source) so the UI can surface a specific recovery hint rather than a
 * generic "something broke" warning.
 */
export const STORAGE_KEY_VAULT_APPLY_IN_PROGRESS = 'lemonssh_vault_apply_in_progress_v1';

// SFTP File Opener Associations
export const STORAGE_KEY_SFTP_FILE_ASSOCIATIONS = 'lemonssh_sftp_file_associations_v1';
export const STORAGE_KEY_SFTP_DEFAULT_OPENER = 'lemonssh_sftp_default_opener_v1';

// SFTP Local Bookmarks
export const STORAGE_KEY_SFTP_LOCAL_BOOKMARKS = 'lemonssh_sftp_local_bookmarks_v1';

// SFTP Global Bookmarks (shared across all hosts)
export const STORAGE_KEY_SFTP_GLOBAL_BOOKMARKS = 'lemonssh_sftp_global_bookmarks_v1';

// SFTP Settings
export const STORAGE_KEY_SFTP_DOUBLE_CLICK_BEHAVIOR = 'lemonssh_sftp_double_click_behavior_v1';
export const STORAGE_KEY_SFTP_AUTO_SYNC = 'lemonssh_sftp_auto_sync_v1';
export const STORAGE_KEY_SFTP_SHOW_HIDDEN_FILES = 'lemonssh_sftp_show_hidden_files_v1';
export const STORAGE_KEY_SFTP_USE_COMPRESSED_UPLOAD = 'lemonssh_sftp_use_compressed_upload_v1';
export const STORAGE_KEY_SFTP_TRANSFER_CENTER = 'lemonssh_sftp_transfer_center_v1';
export const STORAGE_KEY_SFTP_AUTO_OPEN_SIDEBAR = 'lemonssh_sftp_auto_open_sidebar_v1';
export const STORAGE_KEY_SFTP_FOLLOW_TERMINAL_CWD = 'lemonssh_sftp_follow_terminal_cwd_v1';
export const STORAGE_KEY_SFTP_DEFAULT_VIEW_MODE = 'lemonssh_sftp_default_view_mode_v1';
export const STORAGE_KEY_SFTP_HOST_VIEW_MODES = 'lemonssh_sftp_host_view_modes_v1';
export const STORAGE_KEY_SFTP_VISIBLE_COLUMNS = 'lemonssh_sftp_visible_columns_v1';
export const STORAGE_KEY_SFTP_DIRECTORIES_FIRST = 'lemonssh_sftp_directories_first_v1';
/** Dense SFTP toolbar actions: show / collapse / hide + order. */
export const STORAGE_KEY_SFTP_TOOLBAR_LAYOUT = 'lemonssh_sftp_toolbar_layout_v1';
/** Dense terminal session toolbar actions: show / collapse / hide + order. */
export const STORAGE_KEY_TERMINAL_TOOLBAR_LAYOUT = 'lemonssh_terminal_toolbar_layout_v1';
/** Terminal host-tree sidebar toolbar: show / collapse / hide + order. */
export const STORAGE_KEY_TERMINAL_HOST_TREE_TOOLBAR_LAYOUT =
  'lemonssh_terminal_host_tree_toolbar_layout_v1';
/** Side-panel tab strip: show / collapse / hide + order (supersedes order-only key when present). */
export const STORAGE_KEY_TERMINAL_SIDE_PANEL_TAB_LAYOUT = 'lemonssh_terminal_side_panel_tab_layout_v1';
/** System Manager sub-tabs (Overview / Processes / …): show / collapse / hide + order. */
export const STORAGE_KEY_SYSTEM_MANAGER_TAB_LAYOUT = 'lemonssh_system_manager_tab_layout_v1';
export const STORAGE_KEY_SFTP_TRANSFER_PANEL_HEIGHT = 'lemonssh_sftp_transfer_panel_height_v1';
export const STORAGE_KEY_SFTP_TRANSFER_CHILD_NAME_WIDTH = 'lemonssh_sftp_transfer_child_name_width_v1';

// Editor Settings
export const STORAGE_KEY_EDITOR_WORD_WRAP = 'lemonssh_editor_word_wrap_v1';

// Session Logs Settings
export const STORAGE_KEY_SESSION_LOGS_ENABLED = 'lemonssh_session_logs_enabled_v1';
export const STORAGE_KEY_SESSION_LOGS_DIR = 'lemonssh_session_logs_dir_v1';
export const STORAGE_KEY_SESSION_LOGS_FORMAT = 'lemonssh_session_logs_format_v1';
export const STORAGE_KEY_SESSION_LOGS_TIMESTAMPS_ENABLED = 'lemonssh_session_logs_timestamps_enabled_v1';
export const STORAGE_KEY_SSH_DEBUG_LOGS_ENABLED = 'lemonssh_ssh_debug_logs_enabled_v1';
export const STORAGE_KEY_SSH_DEEP_LINK_ENABLED = 'lemonssh_ssh_deep_link_enabled_v1';
export const STORAGE_KEY_JMS_DEEP_LINK_ENABLED = 'lemonssh_jms_deep_link_enabled_v1';
/** Windows Explorer "Open in LemonSSH" folder context menu (device-local). */
export const STORAGE_KEY_EXPLORER_CONTEXT_MENU_ENABLED = 'lemonssh_explorer_context_menu_enabled_v1';

// Archived legacy key records that are no longer supported by the app (e.g. biometric/WebAuthn/FIDO2 experiments).
export const STORAGE_KEY_LEGACY_KEYS = 'lemonssh_legacy_keys_v1';

// Managed Sources - external files that manage groups of hosts (e.g., ~/.ssh/config)
export const STORAGE_KEY_MANAGED_SOURCES = 'lemonssh_managed_sources_v1';

// Global Toggle Window Settings (Quake Mode)
export const STORAGE_KEY_TOGGLE_WINDOW_HOTKEY = 'lemonssh_toggle_window_hotkey_v1';
export const STORAGE_KEY_CLOSE_TO_TRAY = 'lemonssh_close_to_tray_v1';
export const STORAGE_KEY_CLOSE_BEHAVIOR = 'lemonssh_close_behavior_v1';
export const STORAGE_KEY_LAYOUT_MODE = 'lemonssh_layout_mode_v1';
export const STORAGE_KEY_WORKBENCH_SESSION_TREE_EXPANDED = 'lemonssh_workbench_session_tree_expanded_v1';
/** App-level HTTP(S) proxy for cloud sync / AI (not SSH ProxyJump). */
export const STORAGE_KEY_HTTP_NETWORK_PROXY = 'lemonssh_http_network_proxy_v1';
export const STORAGE_KEY_GLOBAL_HOTKEY_ENABLED = 'lemonssh_global_hotkey_enabled_v1';
export const STORAGE_KEY_WINDOW_OPACITY = 'lemonssh_window_opacity_v1';
// Custom Terminal Themes
export const STORAGE_KEY_CUSTOM_THEMES = 'lemonssh_custom_themes_v1';

// AI Settings
export const STORAGE_KEY_AI_PROVIDERS = 'lemonssh_ai_providers_v1';
export const STORAGE_KEY_AI_ACTIVE_PROVIDER = 'lemonssh_ai_active_provider_v1';
export const STORAGE_KEY_AI_ACTIVE_MODEL = 'lemonssh_ai_active_model_v1';
export const STORAGE_KEY_AI_PERMISSION_MODE = 'lemonssh_ai_permission_mode_v1';
export const STORAGE_KEY_AI_TOOL_INTEGRATION_MODE = 'lemonssh_ai_tool_integration_mode_v1';
export const STORAGE_KEY_AI_HOST_PERMISSIONS = 'lemonssh_ai_host_permissions_v1';
export const STORAGE_KEY_AI_EXTERNAL_AGENTS = 'lemonssh_ai_external_agents_v1';
export const STORAGE_KEY_AI_DEFAULT_AGENT = 'lemonssh_ai_default_agent_v1';
export const STORAGE_KEY_AI_COMMAND_BLOCKLIST = 'lemonssh_ai_command_blocklist_v1';
export const STORAGE_KEY_AI_COMMAND_TIMEOUT = 'lemonssh_ai_command_timeout_v1';
export const STORAGE_KEY_AI_RESPONSE_IDLE_TIMEOUT = 'lemonssh_ai_response_idle_timeout_v1';
export const STORAGE_KEY_AI_MAX_ITERATIONS = 'lemonssh_ai_max_iterations_v1';
export const STORAGE_KEY_AI_SESSIONS = 'lemonssh_ai_sessions_v1';
export const STORAGE_KEY_AI_ACTIVE_SESSION_MAP = 'lemonssh_ai_active_session_map_v1';
export const STORAGE_KEY_AI_AGENT_MODEL_MAP = 'lemonssh_ai_agent_model_map_v1';
export const STORAGE_KEY_AI_AGENT_PROVIDER_MAP = 'lemonssh_ai_agent_provider_map_v1';
export const STORAGE_KEY_AI_AGENT_THINKING_MAP = 'lemonssh_ai_agent_thinking_map_v1';
export const STORAGE_KEY_AI_COMPOSER_MODEL_PREFS = 'lemonssh_ai_composer_model_prefs_v1';
export const STORAGE_KEY_AI_WEB_SEARCH = 'lemonssh_ai_web_search_v1';
export const STORAGE_KEY_AI_QUICK_MESSAGES = 'lemonssh_ai_quick_messages_v1';
/** Confirm-mode permission grant memory (capability + session/command patterns). */
export const STORAGE_KEY_AI_PERMISSION_GRANTS = 'lemonssh_ai_permission_grants_v1';
export const STORAGE_KEY_AI_SHOW_TERMINAL_SELECTION_ACTION = 'lemonssh_ai_show_terminal_selection_action_v1';
/** External MCP: whether the user last enabled the public catalog MCP endpoint. */
export const STORAGE_KEY_AI_EXTERNAL_MCP_ENABLED = 'lemonssh_ai_external_mcp_enabled_v1';
/** External MCP lifecycle mode: temporary (idle timeout) or persistent (restore on launch). */
export const STORAGE_KEY_AI_EXTERNAL_MCP_MODE = 'lemonssh_ai_external_mcp_mode_v1';
/** External MCP idle timeout in minutes (temporary mode only). */
export const STORAGE_KEY_AI_EXTERNAL_MCP_IDLE_TIMEOUT_MINUTES = 'lemonssh_ai_external_mcp_idle_timeout_minutes_v1';
/** External MCP: whether host_open should surface/focus the main window (default true). */
export const STORAGE_KEY_AI_EXTERNAL_MCP_FOCUS_ON_HOST_OPEN = 'lemonssh_ai_external_mcp_focus_on_host_open_v1';
/** Idle timeout for terminal sessions opened by an AI through host_open. */
export const STORAGE_KEY_AI_SESSION_IDLE_TIMEOUT_MINUTES = 'lemonssh_ai_session_idle_timeout_minutes_v1';
/** External MCP: whether host_open sessions stay hidden from the tab bar (default false). */
export const STORAGE_KEY_AI_EXTERNAL_MCP_SILENT_SESSIONS = 'lemonssh_ai_external_mcp_silent_sessions_v1';
/** AI panel diagnostic hide list (comma-separated part names). */
export const STORAGE_KEY_AI_PANEL_DIAGNOSTIC_HIDE = 'lemonssh.aiDebug.hide';
/** AI panel React profiler toggle. */
export const STORAGE_KEY_AI_PANEL_DIAGNOSTIC_PROFILE = 'lemonssh.aiDebug.profile';

// SFTP Transfer Concurrency
export const STORAGE_KEY_SFTP_TRANSFER_CONCURRENCY = 'lemonssh_sftp_transfer_concurrency_v1';
/**
 * Legacy key only. Folder full-tree pre-scan was removed; values are ignored
 * so old localStorage / sync payloads do not resurrect a live setting.
 */
export const STORAGE_KEY_SFTP_FOLDER_PRESCAN = 'lemonssh_sftp_folder_prescan_v1';
/** Skip files when target size + mtime already match the source (rsync-like). */
export const STORAGE_KEY_SFTP_SKIP_UNCHANGED = 'lemonssh_sftp_skip_unchanged_v1';
/**
 * @deprecated Legacy transfer-pool idle TTL. No longer read; SSH keep-alive uses
 * STORAGE_KEY_SSH_TRANSPORT_IDLE_TTL_MS. Kept so old localStorage entries are ignored safely.
 */
export const STORAGE_KEY_SFTP_TRANSFER_POOL_IDLE_TTL_MS = 'lemonssh_sftp_transfer_pool_idle_ttl_ms_v1';
/** Shared SSH transport idle park TTL in ms (0 = keep until app quit). */
export const STORAGE_KEY_SSH_TRANSPORT_IDLE_TTL_MS = 'lemonssh_ssh_transport_idle_ttl_ms_v1';

// Workspace Focus Indicator Style
export const STORAGE_KEY_WORKSPACE_FOCUS_STYLE = 'lemonssh_workspace_focus_style_v1';

// Vault: Show Recently Connected hosts section
export const STORAGE_KEY_SHOW_RECENT_HOSTS = 'lemonssh_show_recent_hosts_v1';
export const STORAGE_KEY_HOST_CLICK_BEHAVIOR = 'lemonssh_host_click_behavior_v1';
export const STORAGE_KEY_SHOW_ONLY_UNGROUPED_HOSTS_IN_ROOT = 'lemonssh_show_only_ungrouped_hosts_in_root_v1';

// Top tabs: Show standalone SFTP view tab
export const STORAGE_KEY_SHOW_SFTP_TAB = 'lemonssh_show_sftp_tab_v1';
export const STORAGE_KEY_SHOW_HOST_TREE_SIDEBAR = 'lemonssh_show_host_tree_sidebar_v1';

// Shortcuts: Cmd/Ctrl+[1...9] and Ctrl+Tab skip pinned Vault/SFTP tabs
export const STORAGE_KEY_SHELL_ONLY_TAB_NUMBER_SHORTCUTS = 'lemonssh_shell_only_tab_number_shortcuts_v1';

// Shortcuts: show 1...9 badge on tabs that match number switch shortcuts
export const STORAGE_KEY_SHOW_TAB_NUMBER_BADGES = 'lemonssh_show_tab_number_badges_v1';

// Shortcuts: disable terminal font zoom shortcuts
export const STORAGE_KEY_DISABLE_TERMINAL_FONT_ZOOM = 'lemonssh_disable_terminal_font_zoom_v1';

/** Host IDs for which the "enable Network Device Mode" suggestion has already been shown/handled (suggest once per host). */
export const STORAGE_KEY_NETWORK_DEVICE_SUGGEST_HANDLED = 'lemonssh_network_device_suggest_handled_v1';

// Group Configurations (default settings inherited by hosts)
export const STORAGE_KEY_GROUP_CONFIGS = 'lemonssh_group_configs_v1';
/** Crash-recovery journal for the plugin importer multi-key Vault commit. */
export const STORAGE_KEY_PLUGIN_IMPORT_TRANSACTION = 'lemonssh_plugin_import_transaction_v1';

// Side Panel
export const STORAGE_KEY_SIDE_PANEL_WIDTH = 'lemonssh_side_panel_width';
export const STORAGE_KEY_TERMINAL_SIDE_PANEL_TAB_ORDER = 'lemonssh_terminal_side_panel_tab_order_v1';
export const STORAGE_KEY_TERMINAL_SIDE_PANEL_AUTO_OPEN = 'lemonssh_terminal_side_panel_auto_open_v1';
export const STORAGE_KEY_TERMINAL_SIDE_PANEL_AUTO_OPEN_TAB = 'lemonssh_terminal_side_panel_auto_open_tab_v1';
export const STORAGE_KEY_WORKSPACE_FOCUS_SIDEBAR_WIDTH = 'lemonssh_workspace_focus_sidebar_width';
export const STORAGE_KEY_TERMINAL_HOST_TREE_WIDTH = 'lemonssh_terminal_host_tree_width_v1';
export const STORAGE_KEY_TERMINAL_HOST_TREE_COLLAPSED = 'lemonssh_terminal_host_tree_collapsed_v1';
export const STORAGE_KEY_TERMINAL_COMPOSE_BAR_OPEN = 'lemonssh_terminal_compose_bar_open_v1';
export const STORAGE_KEY_TERMINAL_SEARCH_OPEN = 'lemonssh_terminal_search_open_v1';
export const STORAGE_KEY_TERMINAL_ENCODING_BY_HOST_PREFIX = 'lemonssh_terminal_encoding_by_host_v1:';
export const STORAGE_KEY_TERMINAL_YMODEM_SEND_DIR = 'lemonssh_terminal_ymodem_send_dir_v1';

// Port Forwarding (transient cross-window broadcast key)
export const STORAGE_KEY_PF_RECONNECT_CANCEL = '__lemonssh_pf_cancel_reconnect';

// Default SSH Key Passphrases (for ~/.ssh keys not managed in the vault)
export const STORAGE_KEY_DEFAULT_KEY_PASSPHRASES = 'lemonssh_default_key_passphrases_v1';

// Plugin sync sidecars / availability. Literals MUST match domain/sync
// SYNC_STORAGE_KEYS (PLUGIN_SIDECARS_* / AVAILABLE_PLUGIN_SYNC_PROVIDERS).
export const STORAGE_KEY_PLUGIN_SIDECARS_LAST_KNOWN = 'lemonssh_plugin_sidecars_last_known_v1';
export const STORAGE_KEY_PLUGIN_SIDECARS_PENDING_REMOTE = 'lemonssh_plugin_sidecars_pending_remote_v1';
export const STORAGE_KEY_AVAILABLE_PLUGIN_SYNC_PROVIDERS = 'lemonssh_available_plugin_sync_providers_v1';

// Debug Flags (no _v1 suffix — developer-only, not persisted data)
export const STORAGE_KEY_DEBUG_HOTKEYS = 'debug.hotkeys';
export const STORAGE_KEY_DEBUG_UPDATE_DEMO = 'debug.updateDemo';
