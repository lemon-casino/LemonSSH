package capability

import "testing"

func builtinDecision(method string, mode PermissionMode, params map[string]any, cancelled bool) Decision {
	return Default().Evaluate(Request{
		RPCMethod:            method,
		PermissionMode:       mode,
		Params:               params,
		ChatSessionCancelled: cancelled,
	})
}

func publicDecision(method string, mode PermissionMode, params map[string]any) Decision {
	return Default().Evaluate(Request{
		RPCMethod:      method,
		Surface:        SurfacePublic,
		PermissionMode: mode,
		Params:         params,
	})
}

func globalDecision(method string, mode PermissionMode, params map[string]any) Decision {
	return Default().Evaluate(Request{
		RPCMethod:      method,
		Surface:        SurfaceGlobal,
		PermissionMode: mode,
		Params:         params,
	})
}

// TestBuiltinApprovalSet ports the CJS builtin approval-set expectations.
func TestBuiltinApprovalSet(t *testing.T) {
	approvals := Default().ApprovalRPCMethods(SurfaceBuiltin)
	if approvals["netcatty/jobStop"] {
		t.Errorf("jobStop must not require approval")
	}
	if approvals["netcatty/setCancelled"] {
		t.Errorf("setCancelled must not require approval")
	}
	if !approvals["netcatty/exec"] {
		t.Errorf("exec must require approval")
	}
	if !approvals["netcatty/sftp/write"] {
		t.Errorf("sftp/write must require approval")
	}
}

// TestVaultManagementWritesUseStandardPolicy ports the CJS standard-policy
// row check for the newer vault/portforward management writes.
func TestVaultManagementWritesUseStandardPolicy(t *testing.T) {
	ids := []string{
		"portforward.rules.create", "portforward.rules.update", "portforward.rules.duplicate", "portforward.rules.delete",
		"vault.note.delete", "vault.group.create", "vault.group.update", "vault.group.delete",
	}
	for _, id := range ids {
		def := Default().GetByID(id)
		if def == nil {
			t.Fatalf("missing capability %s", id)
		}
		if !def.Policy.Write || def.Policy.BypassesObserverBlock || def.Policy.BypassesApproval {
			t.Errorf("%s must use the standard write policy, got %+v", id, def.Policy)
		}
	}
}

func TestObserverModeBlocksWritesAllowsPoll(t *testing.T) {
	denied := builtinDecision("netcatty/exec", ModeObserver, map[string]any{"chatSessionId": "chat-1"}, false)
	if denied.Allowed {
		t.Errorf("observer must deny exec")
	}
	if denied.Error != ObserverDenyMessage {
		t.Errorf("observer deny message: got %q", denied.Error)
	}

	allowed := builtinDecision("netcatty/jobPoll", ModeObserver, map[string]any{"chatSessionId": "chat-1"}, false)
	if !allowed.Allowed || allowed.RequiresApproval {
		t.Errorf("observer must allow jobPoll without approval, got %+v", allowed)
	}
}

func TestConfirmModeRequiresApprovalForWrites(t *testing.T) {
	writeDecision := builtinDecision("netcatty/sftp/write", ModeConfirm, map[string]any{"chatSessionId": "chat-1"}, false)
	if !writeDecision.Allowed || !writeDecision.RequiresApproval {
		t.Errorf("confirm must approve sftp/write, got %+v", writeDecision)
	}

	readDecision := builtinDecision("netcatty/sftp/list", ModeConfirm, map[string]any{"chatSessionId": "chat-1"}, false)
	if !readDecision.Allowed || readDecision.RequiresApproval {
		t.Errorf("builtin sftp/list must not require approval, got %+v", readDecision)
	}
}

func TestPublicSurfaceGatesSensitiveReads(t *testing.T) {
	decision := publicDecision("public/sftp/list", ModeConfirm, map[string]any{"sessionId": "sess-1"})
	if !decision.Allowed || !decision.RequiresApproval {
		t.Errorf("public sftp/list must require approval in confirm mode, got %+v", decision)
	}
	if !Default().ApprovalRPCMethods(SurfacePublic)["public/sftp/list"] {
		t.Errorf("public/sftp/list missing from public approval set")
	}
}

func TestBuiltinWritesRequireChatSessionID(t *testing.T) {
	decision := builtinDecision("netcatty/exec", ModeAuto, nil, false)
	if decision.Allowed {
		t.Errorf("exec without chatSessionId must be denied")
	}
	if decision.Error != ChatSessionRequiredMessage {
		t.Errorf("chat session required message: got %q", decision.Error)
	}
}

func TestCancelledChatBlocksWritesAllowsReads(t *testing.T) {
	for _, method := range []string{"netcatty/exec", "netcatty/sftp/write"} {
		decision := builtinDecision(method, ModeAuto, map[string]any{"chatSessionId": "chat-1"}, true)
		if decision.Allowed {
			t.Errorf("%s must be denied when chat session is cancelled", method)
		}
		if decision.Error != ChatSessionCancelledMsg {
			t.Errorf("%s cancel message: got %q", method, decision.Error)
		}
	}

	read := builtinDecision("netcatty/sftp/list", ModeAuto, map[string]any{"chatSessionId": "chat-1"}, true)
	if !read.Allowed {
		t.Errorf("sftp/list must stay allowed when chat session is cancelled")
	}
}

func TestJobStopBypassesObserverAndCancel(t *testing.T) {
	decision := builtinDecision("netcatty/jobStop", ModeObserver, map[string]any{"chatSessionId": "chat-1"}, true)
	if !decision.Allowed {
		t.Errorf("jobStop must bypass observer and cancelled chat checks, got %+v", decision)
	}
	if decision.Error == ObserverDenyMessage {
		t.Errorf("jobStop must not hit the observer deny message")
	}
}

func TestUnknownMethodPassesPolicyUnresolved(t *testing.T) {
	decision := builtinDecision("auth/verify", ModeObserver, nil, false)
	if !decision.Allowed || decision.RequiresApproval || decision.Capability != nil {
		t.Errorf("unknown method must pass policy unresolved, got %+v", decision)
	}
}

func TestConfirmModeApprovalsForGlobalWrites(t *testing.T) {
	pf := publicDecision("public/portforward/start", ModeConfirm, map[string]any{"chatSessionId": "chat-1", "ruleId": "rule-1"})
	if !pf.RequiresApproval {
		t.Errorf("public portforward/start must require approval")
	}

	notes := globalDecision("vault/host/notes/set", ModeConfirm, map[string]any{"chatSessionId": "chat-1", "hostId": "host-1"})
	if !notes.RequiresApproval {
		t.Errorf("global vault/host/notes/set must require approval")
	}

	publicNotes := publicDecision("public/vault/hostNotes/set", ModeConfirm, map[string]any{"chatSessionId": "chat-1", "hostId": "host-1"})
	if !publicNotes.RequiresApproval {
		t.Errorf("public vault/hostNotes/set must require approval")
	}
}

// ---- grant matching (ports policy.test.cjs grant cases) ----

func execWithGrants(command string, grants []Grant) Decision {
	return Default().EvaluateWithGrants(Request{
		RPCMethod:      "netcatty/exec",
		PermissionMode: ModeConfirm,
		Params: map[string]any{
			"chatSessionId": "chat-1",
			"sessionId":     "session-a",
			"command":       command,
		},
	}, grants)
}

func terminalGrant(id, commandPattern string) Grant {
	return Grant{
		ID:             id,
		CapabilityID:   "terminal.execute",
		SessionPattern: "session-a",
		CommandPattern: commandPattern,
		CreatedAt:      1,
	}
}

func requireApproval(t *testing.T, command string, grants []Grant, wantApproval bool) {
	t.Helper()
	decision := execWithGrants(command, grants)
	if !decision.Allowed {
		t.Fatalf("command %q: unexpected denial %q", command, decision.Error)
	}
	if decision.RequiresApproval != wantApproval {
		t.Errorf("command %q: requiresApproval = %v, want %v", command, decision.RequiresApproval, wantApproval)
	}
	if !wantApproval && decision.MatchedGrantID == "" {
		t.Errorf("command %q: grant matched but MatchedGrantID empty", command)
	}
}

func TestGrantCoversSimpleCommand(t *testing.T) {
	requireApproval(t, "ls -la", []Grant{terminalGrant("grant-1", "ls *")}, false)
}

func TestCommentGrantDoesNotApproveMultilineCommand(t *testing.T) {
	command := "# 1a) clear the kernel_options_post profile field\ncobbler profile edit --name=openEuler-22.03-aarch64 --kernel-options-post=\"\""
	requireApproval(t, command, []Grant{terminalGrant("grant-comment", "# *")}, true)
}

func TestHereDocBodyGrantDoesNotApproveCommand(t *testing.T) {
	command := "cat <<'EOF'\nrm -rf /tmp/demo\nEOF"
	requireApproval(t, command, []Grant{terminalGrant("grant-rm", "rm *")}, true)
}

func TestPipedHereDocBodyGrantDoesNotApproveCommand(t *testing.T) {
	command := "cat <<EOF | grep needle\nrm -rf /tmp/demo\nEOF"
	requireApproval(t, command, []Grant{terminalGrant("grant-rm", "rm *")}, true)
}

func TestFdPrefixedHereDocBodyGrantDoesNotApproveCommand(t *testing.T) {
	command := "cat 0<<EOF\nrm -rf /tmp/demo\nEOF"
	requireApproval(t, command, []Grant{terminalGrant("grant-rm", "rm *")}, true)
}

func TestBackgroundCommandGrantDoesNotApproveNextCommand(t *testing.T) {
	requireApproval(t, "cd /tmp; sleep 1 & rm -rf demo", []Grant{terminalGrant("grant-sleep", "sleep *")}, true)
}

func TestCwdSubstitutionDoesNotHideLaterCommand(t *testing.T) {
	requireApproval(t, "cd \"$(pwd)\"; ls -la", []Grant{terminalGrant("grant-ls", "ls *")}, true)
}

func TestQuotedHereDocOperatorTextDoesNotHideLaterCommands(t *testing.T) {
	command := "cd /tmp; echo '<<EOF'\nrm -rf demo"
	requireApproval(t, command, []Grant{terminalGrant("grant-echo", "echo *")}, true)
}

func TestMixedQuotedHereDocDelimiterKeepsLaterCommandsGrantable(t *testing.T) {
	command := "cat <<E\"OF\"\nbody text\nEOF\nls -la"
	requireApproval(t, command, []Grant{terminalGrant("grant-cat", "cat *")}, true)
}

func TestArithmeticShiftsDoNotHideFollowingCommands(t *testing.T) {
	command := "ls $((1 << 2))\nrm -rf demo"
	requireApproval(t, command, []Grant{terminalGrant("grant-ls", "ls *")}, true)
}

func TestAnsiCQuotedHereDocDelimiterKeepsLaterCommandsGrantable(t *testing.T) {
	command := "cat <<$'E\\x4fF'\nbody text\nEOF\nrm -rf demo"
	requireApproval(t, command, []Grant{terminalGrant("grant-cat", "cat *")}, true)
}

func TestDollarQuotedHereDocDelimiterKeepsLaterCommandsGrantable(t *testing.T) {
	command := "cat <<$'EOF'\nbody text\nEOF\nrm -rf demo"
	requireApproval(t, command, []Grant{terminalGrant("grant-cat", "cat *")}, true)
}

func TestArgsPatternGrantWithoutCommand(t *testing.T) {
	grants := []Grant{{
		ID:             "grant-session",
		CapabilityID:   "terminal.start",
		SessionPattern: "*",
		ArgsPattern:    map[string]string{"sessionId": "session-*"},
		CreatedAt:      1,
	}}
	decision := Default().EvaluateWithGrants(Request{
		RPCMethod:      "netcatty/jobStart",
		PermissionMode: ModeConfirm,
		Params:         map[string]any{"chatSessionId": "chat-1", "sessionId": "session-9"},
	}, grants)
	if !decision.Allowed || decision.RequiresApproval || decision.MatchedGrantID != "grant-session" {
		t.Errorf("args-pattern grant should clear approval, got %+v", decision)
	}

	mismatch := Default().EvaluateWithGrants(Request{
		RPCMethod:      "netcatty/jobStart",
		PermissionMode: ModeConfirm,
		Params:         map[string]any{"chatSessionId": "chat-1", "sessionId": "other-9"},
	}, grants)
	if !mismatch.RequiresApproval {
		t.Errorf("non-matching args pattern must keep approval requirement")
	}
}

func TestGlobAndRegexPatterns(t *testing.T) {
	cases := []struct {
		pattern string
		value   string
		want    bool
	}{
		{"*", "anything", true},
		{"prod-*", "prod-1", true},
		{"prod-*", "staging-1", false},
		{"ls *", "ls", true}, // trailing " *" allows empty args (Wildcard.match rule)
		{"rm *", "rm -rf demo", true},
		{"/^vault_(hosts|notes)_list$/", "vault_hosts_list", true},
		{"/^vault_(hosts|notes)_list$/i", "VAULT_HOSTS_LIST", true},
		{"/([a-z+)\\1/", "x", false}, // invalid/unsupported syntax fails closed
		{"host:prod-*", "prod-web", true},
		{"host:prod-*", "dev-web", false},
		{"exact", "exact", true},
	}
	for _, tc := range cases {
		if got := PatternMatches(tc.pattern, tc.value); got != tc.want {
			t.Errorf("PatternMatches(%q, %q) = %v, want %v", tc.pattern, tc.value, got, tc.want)
		}
	}
}

func TestSanitizePermissionGrants(t *testing.T) {
	sanitized := SanitizePermissionGrants([]Grant{
		{CapabilityID: "terminal.execute", CommandPattern: " ls * ", Note: "note"},
		{CapabilityID: "   ", CommandPattern: "x"},
	})
	if len(sanitized) != 1 {
		t.Fatalf("sanitize kept %d grants, want 1", len(sanitized))
	}
	rule := sanitized[0]
	if rule.ID == "" || rule.SessionPattern != "*" || rule.CommandPattern != "ls *" || rule.CreatedAt <= 0 {
		t.Errorf("sanitize defaults missing: %+v", rule)
	}
	if SanitizePermissionGrants(nil) != nil {
		t.Errorf("sanitize of empty input must be empty")
	}
}
