package capability

import "strings"

func policy(write, sensitiveRead, longRunning, requiresChatSession, bypassesObserverBlock, bypassesApproval, bypassesChatCancel bool) Policy {
	return Policy{
		Write:                 write,
		SensitiveRead:         sensitiveRead,
		LongRunning:           longRunning,
		RequiresChatSession:   requiresChatSession,
		BypassesObserverBlock: bypassesObserverBlock,
		BypassesApproval:      bypassesApproval,
		BypassesChatCancel:    bypassesChatCancel,
	}
}

// readOnlyChatPolicy is the read archetype used across meta/terminal/harness:
// chat-scoped, observer-safe, never approved, never cancelled by chat stop.
func readOnlyChatPolicy(sensitiveRead bool) Policy {
	return policy(false, sensitiveRead, false, true, false, true, true)
}

// readOnlyVaultPolicy is the vault/portforward read archetype: not
// chat-scoped but otherwise identical to readOnlyChatPolicy.
func readOnlyVaultPolicy(sensitiveRead bool) Policy {
	return policy(false, sensitiveRead, false, false, false, true, true)
}

// vaultWritePolicy is the vault/portforward write archetype: confirm-gated
// and subject to observer block and chat cancel.
func vaultWritePolicy(longRunning bool) Policy {
	return policy(true, false, longRunning, false, false, false, false)
}

func confirmTrue() *bool { b := true; return &b }

func sftpBinding(rpcMethod, publicMethod, mcpTool, command string, publicConfirm bool) map[Surface]SurfaceBinding {
	surfaces := map[Surface]SurfaceBinding{
		SurfaceBuiltin: {RPCMethod: rpcMethod},
		SurfacePublic:  {RPCMethod: publicMethod, MCPTool: mcpTool},
		SurfaceCLI:     {Command: strings.Split(command, " ")},
	}
	if publicConfirm {
		surfaces[SurfacePublic] = SurfaceBinding{RPCMethod: publicMethod, MCPTool: mcpTool, ConfirmInConfirmMode: confirmTrue()}
	}
	return surfaces
}

func sftpCapability(id, description string, overrides Policy, surfaces map[Surface]SurfaceBinding) Definition {
	base := Policy{
		Write:                 false,
		SensitiveRead:         false,
		LongRunning:           true,
		RequiresChatSession:   true,
		BypassesObserverBlock: false,
		BypassesApproval:      true,
		BypassesChatCancel:    true,
	}
	if overrides != (Policy{}) {
		if overrides.SensitiveRead {
			base.SensitiveRead = true
		}
		if overrides.Write {
			base.Write = true
			base.BypassesApproval = false
			base.BypassesChatCancel = false
		}
	}
	return Definition{ID: id, Domain: "sftp", Status: StatusImplemented, Description: description, Policy: base, Surfaces: surfaces}
}

func vaultGroupCapability(id, description, action, mcpTool string, write bool) Definition {
	return Definition{
		ID:          id,
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: description,
		Policy:      policy(write, false, false, false, false, !write, !write),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "vault/groups/" + action},
			SurfacePublic: {RPCMethod: "public/vault/groups/" + action, MCPTool: mcpTool},
		},
	}
}

func portforwardRuleMutation(id, description, action, mcpTool string) Definition {
	return Definition{
		ID:          id,
		Domain:      "portforward",
		Status:      StatusImplemented,
		Description: description,
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "portforward/rules/" + action},
			SurfacePublic: {RPCMethod: "public/portforward/rules/" + action, MCPTool: mcpTool},
		},
	}
}

// Catalog is the frozen W05 port of electron/capabilities/catalog. Order is
// load-bearing: CJS registry lookups resolve surface collisions (duplicate
// rpcMethod on one surface) last-wins, so Go must keep the same sequence.
var Catalog = []Definition{
	// ---- meta.cjs ----
	{
		ID:          "session.environment",
		Domain:      "session",
		Status:      StatusImplemented,
		Description: "List scoped terminal sessions available to the agent.",
		Policy:      readOnlyChatPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceBuiltin: {RPCMethod: "netcatty/getContext", MCPTool: "get_environment"},
			SurfacePublic:  {RPCMethod: "public/getEnvironment", MCPTool: "get_environment"},
			SurfaceCLI:     {Command: []string{"env"}},
		},
	},
	{
		ID:          "meta.status",
		Domain:      "meta",
		Status:      StatusImplemented,
		Description: "Return bridge runtime status and policy configuration.",
		Policy:      readOnlyVaultPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceBuiltin: {RPCMethod: "netcatty/getStatus"},
			SurfacePublic:  {RPCMethod: "public/getStatus"},
			SurfaceCLI:     {Command: []string{"status"}},
		},
	},
	{
		ID:          "attachment.list",
		Domain:      "attachment",
		Status:      StatusImplemented,
		Description: "List user-attached files in the current AI chat scope.",
		Policy:      readOnlyChatPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceBuiltin: {RPCMethod: "netcatty/listAttachments", MCPTool: "list_attachments"},
			SurfaceCLI:     {Command: []string{"attachment", "list"}},
		},
	},
	{
		ID:          "attachment.read",
		Domain:      "attachment",
		Status:      StatusImplemented,
		Description: "Read a user-attached file from the current AI chat scope.",
		Policy:      readOnlyChatPolicy(true),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceBuiltin: {RPCMethod: "netcatty/readAttachment", MCPTool: "read_attachment"},
			SurfaceCLI:     {Command: []string{"attachment", "read"}},
		},
	},
	{
		ID:          "session.cancel",
		Domain:      "session",
		Status:      StatusImplemented,
		Description: "Cancel in-flight operations for a chat session.",
		Policy:      readOnlyChatPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceBuiltin: {RPCMethod: "netcatty/setCancelled"},
			SurfaceCLI:     {Command: []string{"cancel"}},
		},
	},
	{
		ID:          "session.resume",
		Domain:      "session",
		Status:      StatusImplemented,
		Description: "Resume write operations for a cancelled chat session.",
		Policy:      readOnlyChatPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceBuiltin: {RPCMethod: "netcatty/setCancelled"},
			SurfaceCLI:     {Command: []string{"resume"}},
		},
	},
	{
		ID:          "session.get",
		Domain:      "session",
		Status:      StatusImplemented,
		Description: "Get metadata for a single scoped session.",
		Policy:      readOnlyChatPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceBuiltin: {RPCMethod: "netcatty/getContext"},
			SurfaceCLI:     {Command: []string{"session"}},
		},
	},
	{
		ID:          "session.close",
		Domain:      "session",
		Status:      StatusImplemented,
		Description: "Close a terminal session previously opened by host_open in the current AI scope.",
		Policy:      policy(true, false, false, true, true, true, true),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "session/close"},
			SurfacePublic: {RPCMethod: "public/session/close", MCPTool: "session_close"},
		},
	},

	// ---- terminal.cjs ----
	{
		ID:          "terminal.execute",
		Domain:      "terminal",
		Status:      StatusImplemented,
		Description: "Execute a short command in a terminal session and wait for completion.",
		Policy:      policy(true, false, true, true, false, false, false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceBuiltin: {RPCMethod: "netcatty/exec", MCPTool: "terminal_execute"},
			SurfacePublic:  {RPCMethod: "public/terminalExecute", MCPTool: "terminal_execute"},
			SurfaceCLI:     {Command: []string{"exec"}},
		},
	},
	{
		ID:          "terminal.start",
		Domain:      "terminal",
		Status:      StatusImplemented,
		Description: "Start a long-running command in a terminal session.",
		Policy:      policy(true, false, true, true, false, false, false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceBuiltin: {RPCMethod: "netcatty/jobStart", MCPTool: "terminal_start"},
			SurfacePublic:  {RPCMethod: "public/terminalStart", MCPTool: "terminal_start"},
			SurfaceCLI:     {Command: []string{"job-start"}},
		},
	},
	{
		ID:          "terminal.poll",
		Domain:      "terminal",
		Status:      StatusImplemented,
		Description: "Poll incremental output from a long-running terminal job.",
		Policy:      readOnlyChatPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceBuiltin: {RPCMethod: "netcatty/jobPoll", MCPTool: "terminal_poll"},
			SurfacePublic:  {RPCMethod: "public/terminalPoll", MCPTool: "terminal_poll"},
			SurfaceCLI:     {Command: []string{"job-poll"}},
		},
	},
	{
		ID:          "terminal.stop",
		Domain:      "terminal",
		Status:      StatusImplemented,
		Description: "Stop a long-running terminal job.",
		Policy:      policy(true, false, false, true, true, true, true),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceBuiltin: {RPCMethod: "netcatty/jobStop", MCPTool: "terminal_stop"},
			SurfacePublic:  {RPCMethod: "public/terminalStop", MCPTool: "terminal_stop"},
			SurfaceCLI:     {Command: []string{"job-stop"}},
		},
	},

	// ---- sftp.cjs ----
	sftpCapability(
		"sftp.list",
		"List a remote directory over the session file backend (SFTP or SCP-mode).",
		Policy{SensitiveRead: true},
		sftpBinding("netcatty/sftp/list", "public/sftp/list", "sftp_list", "sftp list", true),
	),
	sftpCapability(
		"sftp.read",
		"Read a remote file over the session file backend (SFTP or SCP-mode).",
		Policy{SensitiveRead: true},
		sftpBinding("netcatty/sftp/read", "public/sftp/readFile", "sftp_read_file", "sftp read", true),
	),
	sftpCapability(
		"sftp.write",
		"Write a remote file over the session file backend (SFTP or SCP-mode).",
		Policy{Write: true},
		sftpBinding("netcatty/sftp/write", "public/sftp/writeFile", "sftp_write_file", "sftp write", false),
	),
	sftpCapability(
		"sftp.download",
		"Download a remote file to a local path.",
		Policy{Write: true},
		sftpBinding("netcatty/sftp/download", "public/sftp/download", "sftp_download", "sftp download", false),
	),
	sftpCapability(
		"sftp.upload",
		"Upload a local file to a remote path.",
		Policy{Write: true},
		sftpBinding("netcatty/sftp/upload", "public/sftp/upload", "sftp_upload", "sftp upload", false),
	),
	sftpCapability(
		"sftp.stat",
		"Get remote file metadata over the session file backend (SFTP or SCP-mode).",
		Policy{SensitiveRead: true},
		sftpBinding("netcatty/sftp/stat", "public/sftp/stat", "sftp_stat", "sftp stat", true),
	),
	sftpCapability(
		"sftp.home",
		"Get the remote home directory for a session.",
		Policy{SensitiveRead: true},
		sftpBinding("netcatty/sftp/home", "public/sftp/home", "sftp_home", "sftp home", true),
	),
	sftpCapability(
		"sftp.mkdir",
		"Create a remote directory over the session file backend (SFTP or SCP-mode).",
		Policy{Write: true},
		sftpBinding("netcatty/sftp/mkdir", "public/sftp/mkdir", "sftp_mkdir", "sftp mkdir", false),
	),
	sftpCapability(
		"sftp.delete",
		"Delete a remote file or directory over the session file backend (SFTP or SCP-mode).",
		Policy{Write: true},
		sftpBinding("netcatty/sftp/delete", "public/sftp/delete", "sftp_delete", "sftp delete", false),
	),
	sftpCapability(
		"sftp.rename",
		"Rename a remote file or directory over the session file backend (SFTP or SCP-mode).",
		Policy{Write: true},
		sftpBinding("netcatty/sftp/rename", "public/sftp/rename", "sftp_rename", "sftp rename", false),
	),
	sftpCapability(
		"sftp.chmod",
		"Change remote file permissions over the session file backend (SFTP or SCP-mode).",
		Policy{Write: true},
		sftpBinding("netcatty/sftp/chmod", "public/sftp/chmod", "sftp_chmod", "sftp chmod", false),
	),

	// ---- vault.cjs ----
	{
		ID:          "vault.host.get",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Get host metadata from the vault.",
		Policy:      readOnlyVaultPolicy(true),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"vault", "host", "get"}},
			SurfaceGlobal: {RPCMethod: "vault/host/get"},
			SurfacePublic: {RPCMethod: "public/vault/host/get", MCPTool: "host_get"},
		},
	},
	{
		ID:          "vault.host.list",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "List saved hosts in the vault (metadata only — no passwords or keys).",
		Policy:      readOnlyVaultPolicy(true),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "vault/hosts/list"},
			SurfacePublic: {RPCMethod: "public/vault/hosts/list", MCPTool: "vault_hosts_list"},
		},
	},
	{
		ID:          "vault.host.open",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Open a vault host by creating a new terminal tab and starting the connection. Returns the new sessionId so you can run terminal/SFTP tools against it. Use vault_hosts_list first when you only know the label or hostname.",
		// Sidebar Catty is scoped to already-open terminals/workspaces and must
		// not expand that scope mid-turn. Keep host_open for MCP / CLI / global.
		AgentKinds: []AgentKind{AgentKindGlobal},
		Policy:     vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"vault", "host", "open"}},
			SurfaceGlobal: {RPCMethod: "vault/hosts/open"},
			SurfacePublic: {RPCMethod: "public/vault/hosts/open", MCPTool: "host_open"},
		},
	},
	{
		ID:          "vault.hosts.create",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Create vault hosts from structured host objects. Use when the user wants to add/create a host (Vault → Hosts). NOT for Vault → Notes sidebar documentation.",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "vault/hosts/create"},
			SurfacePublic: {RPCMethod: "public/vault/hosts/create", MCPTool: "vault_hosts_create"},
		},
	},
	{
		ID:          "vault.host.update",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Update selected fields on an existing vault host. Use vault_hosts_list first to resolve the hostId.",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "vault/hosts/update"},
			SurfacePublic: {RPCMethod: "public/vault/hosts/update", MCPTool: "vault_hosts_update"},
		},
	},
	{
		ID:          "vault.host.delete",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Delete a saved vault host by id. Use vault_hosts_list first to resolve the hostId.",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "vault/hosts/delete"},
			SurfacePublic: {RPCMethod: "public/vault/hosts/delete", MCPTool: "vault_hosts_delete"},
		},
	},
	{
		ID:          "vault.host.import",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Parse known host export file formats (PuTTY, MobaXterm, CSV, SecureCRT, ssh_config) into vault hosts. For arbitrary unstructured text, map to host objects and use vault_hosts_create instead.",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "vault/hosts/import"},
			SurfacePublic: {RPCMethod: "public/vault/hosts/import", MCPTool: "vault_hosts_import"},
		},
	},
	{
		ID:          "vault.host.notes.get",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Read host metadata notes attached to a saved host (Host Details panel — not Vault sidebar Notes).",
		Policy:      readOnlyVaultPolicy(true),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"vault", "host-notes", "get"}},
			SurfaceGlobal: {RPCMethod: "vault/host/notes/get"},
			SurfacePublic: {RPCMethod: "public/vault/hostNotes/get", MCPTool: "host_notes_get"},
		},
	},
	{
		ID:          "vault.host.notes.set",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Update host metadata notes on a saved host (Host Details panel — not Vault sidebar Notes).",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"vault", "host-notes", "set"}},
			SurfaceGlobal: {RPCMethod: "vault/host/notes/set"},
			SurfacePublic: {RPCMethod: "public/vault/hostNotes/set", MCPTool: "host_notes_set"},
		},
	},
	{
		ID:          "vault.note.list",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "List notes in Vault → Notes (markdown notes visible in the vault sidebar).",
		Policy:      readOnlyVaultPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "vault/notes/list"},
			SurfacePublic: {RPCMethod: "public/vault/notes/list", MCPTool: "vault_notes_list"},
		},
	},
	{
		ID:          "vault.note.get",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Get a Vault → Notes entry by id (full markdown content).",
		Policy:      readOnlyVaultPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "vault/notes/get"},
			SurfacePublic: {RPCMethod: "public/vault/notes/get", MCPTool: "vault_notes_get"},
		},
	},
	{
		ID:          "vault.note.create",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Create a note in Vault → Notes sidebar (markdown documentation). NOT for adding SSH hosts — use vault_hosts_create for that.",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "vault/notes/create"},
			SurfacePublic: {RPCMethod: "public/vault/notes/create", MCPTool: "vault_notes_create"},
		},
	},
	{
		ID:          "vault.note.update",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Update an existing Vault → Notes entry by id.",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "vault/notes/update"},
			SurfacePublic: {RPCMethod: "public/vault/notes/update", MCPTool: "vault_notes_update"},
		},
	},
	{
		ID:          "vault.note.delete",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Delete a Vault → Notes entry by id.",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "vault/notes/delete"},
			SurfacePublic: {RPCMethod: "public/vault/notes/delete", MCPTool: "vault_notes_delete"},
		},
	},
	{
		ID:          "vault.identity.list",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "List reusable vault identities without passwords, private keys, or passphrases.",
		Policy:      readOnlyVaultPolicy(true),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "vault/identities/list"},
			SurfacePublic: {RPCMethod: "public/vault/identities/list", MCPTool: "vault_identities_list"},
		},
	},
	{
		ID:          "vault.proxyProfile.list",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "List reusable proxy profiles without credentials.",
		Policy:      readOnlyVaultPolicy(true),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceGlobal: {RPCMethod: "vault/proxyProfiles/list"},
			SurfacePublic: {RPCMethod: "public/vault/proxyProfiles/list", MCPTool: "vault_proxy_profiles_list"},
		},
	},
	vaultGroupCapability("vault.group.list", "List vault groups and their safe default settings.", "list", "vault_groups_list", false),
	vaultGroupCapability("vault.group.create", "Create a vault group with optional default connection settings.", "create", "vault_groups_create", true),
	vaultGroupCapability("vault.group.update", "Update or rename a vault group and its default connection settings.", "update", "vault_groups_update", true),
	vaultGroupCapability("vault.group.delete", "Delete a vault group, moving its hosts to the root unless deleteHosts is true.", "delete", "vault_groups_delete", true),
	{
		ID:          "vault.snippets.list",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "List code snippets stored in the vault.",
		Policy:      readOnlyVaultPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"snippets", "list"}},
			SurfaceGlobal: {RPCMethod: "vault/snippets/list"},
			SurfacePublic: {RPCMethod: "public/vault/snippets/list", MCPTool: "snippets_list"},
		},
	},
	{
		ID:          "vault.snippets.get",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Get a single code snippet from the vault.",
		Policy:      readOnlyVaultPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"snippets", "get"}},
			SurfaceGlobal: {RPCMethod: "vault/snippets/get"},
			SurfacePublic: {RPCMethod: "public/vault/snippets/get", MCPTool: "snippets_get"},
		},
	},
	{
		ID:          "vault.snippets.run",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Run a vault snippet or automation script in a terminal session. Text snippets paste shell commands; scripts (kind=script) run via the nct JavaScript runtime.",
		Policy:      policy(true, false, true, true, false, false, false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"snippets", "run"}},
			SurfaceGlobal: {RPCMethod: "vault/snippets/run"},
			SurfacePublic: {RPCMethod: "public/vault/snippets/run", MCPTool: "snippets_run"},
		},
	},
	{
		ID:          "vault.snippets.create",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Create a vault snippet or automation script (set kind=script for nct automation).",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"snippets", "create"}},
			SurfaceGlobal: {RPCMethod: "vault/snippets/create"},
			SurfacePublic: {RPCMethod: "public/vault/snippets/create", MCPTool: "snippets_create"},
		},
	},
	{
		ID:          "vault.snippets.update",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Update an existing vault snippet or automation script by id.",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"snippets", "update"}},
			SurfaceGlobal: {RPCMethod: "vault/snippets/update"},
			SurfacePublic: {RPCMethod: "public/vault/snippets/update", MCPTool: "snippets_update"},
		},
	},
	{
		ID:          "vault.snippets.delete",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Delete a vault snippet or automation script by id.",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"snippets", "delete"}},
			SurfaceGlobal: {RPCMethod: "vault/snippets/delete"},
			SurfacePublic: {RPCMethod: "public/vault/snippets/delete", MCPTool: "snippets_delete"},
		},
	},
	{
		ID:          "vault.scripts.list",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "List automation scripts (kind=script) in the vault.",
		Policy:      readOnlyVaultPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"scripts", "list"}},
			SurfaceGlobal: {RPCMethod: "vault/scripts/list"},
			SurfacePublic: {RPCMethod: "public/vault/scripts/list", MCPTool: "scripts_list"},
		},
	},
	{
		ID:          "vault.scripts.get",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Get a single automation script including JavaScript source.",
		Policy:      readOnlyVaultPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"scripts", "get"}},
			SurfaceGlobal: {RPCMethod: "vault/scripts/get"},
			SurfacePublic: {RPCMethod: "public/vault/scripts/get", MCPTool: "scripts_get"},
		},
	},
	{
		ID:          "vault.scripts.create",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Create an automation script using the nct JavaScript API. Call scripts_reference first when authoring nct automation.",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"scripts", "create"}},
			SurfaceGlobal: {RPCMethod: "vault/scripts/create"},
			SurfacePublic: {RPCMethod: "public/vault/scripts/create", MCPTool: "scripts_create"},
		},
	},
	{
		ID:          "vault.scripts.update",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Update an automation script by id (partial fields).",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"scripts", "update"}},
			SurfaceGlobal: {RPCMethod: "vault/scripts/update"},
			SurfacePublic: {RPCMethod: "public/vault/scripts/update", MCPTool: "scripts_update"},
		},
	},
	{
		ID:          "vault.scripts.delete",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Delete an automation script and remove host connect bindings.",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"scripts", "delete"}},
			SurfaceGlobal: {RPCMethod: "vault/scripts/delete"},
			SurfacePublic: {RPCMethod: "public/vault/scripts/delete", MCPTool: "scripts_delete"},
		},
	},
	{
		ID:          "vault.scripts.run",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Run an automation script in a terminal session via the nct runtime. Set wait=true to block until completion.",
		Policy:      policy(true, false, true, true, false, false, false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"scripts", "run"}},
			SurfaceGlobal: {RPCMethod: "vault/scripts/run"},
			SurfacePublic: {RPCMethod: "public/vault/scripts/run", MCPTool: "scripts_run"},
		},
	},
	{
		ID:          "vault.scripts.reference",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Return Netcatty automation script syntax: nct API, triggers, host targeting, and source wrapping rules.",
		Policy:      readOnlyVaultPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"scripts", "reference"}},
			SurfaceGlobal: {RPCMethod: "vault/scripts/reference"},
			SurfacePublic: {RPCMethod: "public/vault/scripts/reference", MCPTool: "scripts_reference"},
		},
	},
	{
		ID:          "vault.scripts.runs.list",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "List automation script runs (optionally filter by sessionId).",
		Policy:      readOnlyVaultPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"scripts", "runs", "list"}},
			SurfaceGlobal: {RPCMethod: "vault/scripts/runs/list"},
			SurfacePublic: {RPCMethod: "public/vault/scripts/runs/list", MCPTool: "scripts_runs_list"},
		},
	},
	{
		ID:          "vault.scripts.run.stop",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Stop a running automation script by runId.",
		Policy:      policy(true, false, false, true, false, false, false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"scripts", "run", "stop"}},
			SurfaceGlobal: {RPCMethod: "vault/scripts/run/stop"},
			SurfacePublic: {RPCMethod: "public/vault/scripts/run/stop", MCPTool: "scripts_run_stop"},
		},
	},
	{
		ID:          "vault.scripts.run.pause",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Pause a running automation script by runId.",
		Policy:      policy(true, false, false, true, false, false, false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"scripts", "run", "pause"}},
			SurfaceGlobal: {RPCMethod: "vault/scripts/run/pause"},
			SurfacePublic: {RPCMethod: "public/vault/scripts/run/pause", MCPTool: "scripts_run_pause"},
		},
	},
	{
		ID:          "vault.scripts.run.resume",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Resume a paused automation script by runId.",
		Policy:      policy(true, false, false, true, false, false, false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"scripts", "run", "resume"}},
			SurfaceGlobal: {RPCMethod: "vault/scripts/run/resume"},
			SurfacePublic: {RPCMethod: "public/vault/scripts/run/resume", MCPTool: "scripts_run_resume"},
		},
	},
	{
		ID:          "vault.scripts.targets.set",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Set host IDs, dynamic group paths, or targetsAllHosts for an automation script. onConnect host IDs sync host connect queues.",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"scripts", "targets", "set"}},
			SurfaceGlobal: {RPCMethod: "vault/scripts/targets/set"},
			SurfacePublic: {RPCMethod: "public/vault/scripts/targets/set", MCPTool: "scripts_targets_set"},
		},
	},
	{
		ID:          "vault.host.connectScripts.list",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "List resolved onConnect automation scripts for a host (global, dynamic group, then host queue).",
		Policy:      readOnlyVaultPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"vault", "host", "connect-scripts", "list"}},
			SurfaceGlobal: {RPCMethod: "vault/host/connectScripts/list"},
			SurfacePublic: {RPCMethod: "public/vault/hostConnectScripts/list", MCPTool: "host_connect_scripts_list"},
		},
	},
	{
		ID:          "vault.host.connectScripts.set",
		Domain:      "vault",
		Status:      StatusImplemented,
		Description: "Set ordered onConnect script IDs for a host (host-specific queue; globals run separately).",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"vault", "host", "connect-scripts", "set"}},
			SurfaceGlobal: {RPCMethod: "vault/host/connectScripts/set"},
			SurfacePublic: {RPCMethod: "public/vault/hostConnectScripts/set", MCPTool: "host_connect_scripts_set"},
		},
	},

	// ---- portforward.cjs ----
	{
		ID:          "portforward.rules.list",
		Domain:      "portforward",
		Status:      StatusImplemented,
		Description: "List persisted port forwarding rules.",
		Policy:      readOnlyVaultPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"portforward", "rules", "list"}},
			SurfaceGlobal: {RPCMethod: "portforward/rules/list"},
			SurfacePublic: {RPCMethod: "public/portforward/rules/list", MCPTool: "portforward_rules_list"},
		},
	},
	{
		ID:          "portforward.tunnels.list",
		Domain:      "portforward",
		Status:      StatusImplemented,
		Description: "List active port forwarding tunnels.",
		Policy:      readOnlyVaultPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"portforward", "tunnels", "list"}},
			SurfaceGlobal: {RPCMethod: "portforward/tunnels/list"},
			SurfacePublic: {RPCMethod: "public/portforward/tunnels/list", MCPTool: "portforward_tunnels_list"},
		},
	},
	portforwardRuleMutation("portforward.rules.create", "Create a persisted port forwarding rule.", "create", "portforward_rules_create"),
	portforwardRuleMutation("portforward.rules.update", "Update a persisted port forwarding rule.", "update", "portforward_rules_update"),
	portforwardRuleMutation("portforward.rules.duplicate", "Duplicate a persisted port forwarding rule.", "duplicate", "portforward_rules_duplicate"),
	portforwardRuleMutation("portforward.rules.delete", "Delete a persisted port forwarding rule.", "delete", "portforward_rules_delete"),
	{
		ID:          "portforward.start",
		Domain:      "portforward",
		Status:      StatusImplemented,
		Description: "Start a port forwarding tunnel for a rule.",
		Policy:      vaultWritePolicy(true),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"portforward", "start"}},
			SurfaceGlobal: {RPCMethod: "portforward/start"},
			SurfacePublic: {RPCMethod: "public/portforward/start", MCPTool: "portforward_start"},
		},
	},
	{
		ID:          "portforward.stop",
		Domain:      "portforward",
		Status:      StatusImplemented,
		Description: "Stop an active port forwarding tunnel.",
		Policy:      vaultWritePolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCLI:    {Command: []string{"portforward", "stop"}},
			SurfaceGlobal: {RPCMethod: "portforward/stop"},
			SurfacePublic: {RPCMethod: "public/portforward/stop", MCPTool: "portforward_stop"},
		},
	},

	// ---- harness.cjs (sidebar-only renderer-local tools) ----
	harnessCapability("harness.tool_output.read", "tool_output_read", "Read stored tool output by handle id when a prior tool result was truncated."),
	harnessCapability("harness.workspace.get_info", "workspace_get_info", "Get information about the current workspace, including all terminal sessions and their connection status."),
	harnessCapability("harness.workspace.get_session_info", "workspace_get_session_info", "Get detailed information about a specific terminal or SFTP session."),
	harnessCapability("harness.terminal.read_context", "terminal_read_context", "Read a bounded slice of the current terminal screen or scrollback from the active AI scope."),
	harnessCapability("harness.web.search", "web_search", "Search the web for current information when configured in AI settings."),
	harnessCapability("harness.url.fetch", "url_fetch", "Fetch and read the content of an HTTPS URL."),
}

func harnessCapability(id, toolName, description string) Definition {
	return Definition{
		ID:          id,
		Domain:      "harness",
		Status:      StatusImplemented,
		Description: description,
		Policy:      readOnlyChatPolicy(false),
		Surfaces: map[Surface]SurfaceBinding{
			SurfaceCatty: {ToolName: toolName},
		},
	}
}
