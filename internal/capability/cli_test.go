package capability

import (
	"reflect"
	"strings"
	"testing"
)

// Ports cliAdapter.test.cjs (CJS CLI fallback authority).
func TestGetCLIRPCMethodResolvesCommands(t *testing.T) {
	cases := []struct {
		command []string
		want    string
	}{
		{[]string{"exec"}, "netcatty/exec"},
		{[]string{"attachment", "read"}, "netcatty/readAttachment"},
		{[]string{"sftp", "list"}, "netcatty/sftp/list"},
		{[]string{"vault", "host", "get"}, "vault/host/get"},
		{[]string{"portforward", "rules", "list"}, "portforward/rules/list"},
		{[]string{"capabilities"}, ""},
	}
	for _, tc := range cases {
		def := Default().GetByCLICommand(tc.command)
		var got string
		if def != nil {
			got = def.ResolveCLIRPCMethod()
		}
		if got != tc.want {
			t.Errorf("GetByCLICommand(%v) rpc method = %q, want %q", tc.command, got, tc.want)
		}
	}
}

func TestListCLICapabilitiesImplementedOnly(t *testing.T) {
	entries := Default().ListCLICapabilities()
	want := map[string]bool{
		"terminal.execute": false,
		"attachment.list":  false,
		"attachment.read":  false,
		"vault.host.get":   false,
	}
	for _, entry := range entries {
		if _, seen := want[entry.ID]; seen {
			want[entry.ID] = true
		}
		if entry.Status != StatusImplemented {
			t.Errorf("entry %s must be implemented, got %s", entry.ID, entry.Status)
		}
		if entry.RPCMethod == "" {
			t.Errorf("entry %s must carry an rpc method", entry.ID)
		}
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("list is missing %s", id)
		}
	}
}

func TestBuildCatalogCLIParams(t *testing.T) {
	cases := []struct {
		name          string
		capabilityID  string
		opts          map[string]any
		want          map[string]any
		wantErrSubstr string
	}{
		{
			name:         "vault host get flags",
			capabilityID: "vault.host.get",
			opts:         map[string]any{"hostId": "host-1"},
			want:         map[string]any{"hostId": "host-1"},
		},
		{
			name:         "attachment filename",
			capabilityID: "attachment.read",
			opts:         map[string]any{"filename": "hosts.csv"},
			want:         map[string]any{"filename": "hosts.csv"},
		},
		{
			name:         "snippet variables JSON",
			capabilityID: "vault.snippets.run",
			opts: map[string]any{
				"snippetId": "snip-1",
				"sessionId": "sess-1",
				"variables": `{"name":"prod"}`,
			},
			want: map[string]any{
				"snippetId": "snip-1",
				"sessionId": "sess-1",
				"variables": map[string]any{"name": "prod"},
			},
		},
		{
			name:         "snippet multi-line run mode",
			capabilityID: "vault.snippets.create",
			opts: map[string]any{
				"label":            "login",
				"content":          "user\npass",
				"multiLineRunMode": "lineDelay",
			},
			want: map[string]any{
				"label":            "login",
				"content":          "user\npass",
				"multiLineRunMode": "lineDelay",
			},
		},
		{
			name:         "dynamic script group targets stay strings",
			capabilityID: "vault.scripts.targets.set",
			opts: map[string]any{
				"scriptId":     "script-1",
				"targetGroups": `["Production","Staging/Web"]`,
			},
			want: map[string]any{
				"scriptId":     "script-1",
				"targetGroups": `["Production","Staging/Web"]`,
			},
		},
		{
			name:          "missing required field",
			capabilityID:  "vault.host.get",
			opts:          map[string]any{},
			wantErrSubstr: "Missing required --host-id",
		},
		{
			name:          "invalid variables JSON",
			capabilityID:  "vault.snippets.run",
			opts:          map[string]any{"snippetId": "snip-1", "sessionId": "sess-1", "variables": "{not-json"},
			wantErrSubstr: "--variables must be valid JSON",
		},
		{
			name:         "unknown fields ignored",
			capabilityID: "vault.host.get",
			opts:         map[string]any{"hostId": "host-1", "unrelated": "x"},
			want:         map[string]any{"hostId": "host-1"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildCatalogCLIParams(tc.capabilityID, tc.opts)
			if tc.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got params %v", tc.wantErrSubstr, got)
				}
				argErr, ok := err.(*CLIArgError)
				if !ok || argErr.Code != "INVALID_ARGUMENT" {
					t.Fatalf("error must be INVALID_ARGUMENT CLIArgError, got %v", err)
				}
				if !strings.Contains(argErr.Message, tc.wantErrSubstr) {
					t.Errorf("error message %q must contain %q", argErr.Message, tc.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("params: got %v want %v", got, tc.want)
			}
			for key, wantValue := range tc.want {
				if !reflect.DeepEqual(got[key], wantValue) {
					t.Errorf("params[%q] = %v, want %v", key, got[key], wantValue)
				}
			}
		})
	}
}

func TestCLIHandlesEmptyFilesAndCompleteOptionalFields(t *testing.T) {
	params, err := BuildCatalogCLIParams("sftp.write", map[string]any{"sessionId": "s", "remotePath": "/empty", "content": ""})
	if err != nil || params["content"] != "" {
		t.Fatalf("cannot truncate file: %v %v", params, err)
	}
	params, err = BuildCatalogCLIParams("attachment.read", map[string]any{"filePath": "/attached/a"})
	if err != nil || params["filePath"] != "/attached/a" {
		t.Fatalf("lost file path: %v %v", params, err)
	}
	for _, offset := range []string{"1junk", "NaN", "Infinity", "-1", "1.5"} {
		if _, err := BuildCatalogCLIParams("terminal.poll", map[string]any{"jobId": "j", "offset": offset}); err == nil {
			t.Errorf("accepted offset %q", offset)
		}
	}
}
