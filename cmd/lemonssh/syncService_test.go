package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestParseS3SettingsUsesRendererFieldNames pins the wire format the renderer
// sends (S3Adapter passes its domain config verbatim): secretAccessKey (not
// the old secretKey), forcePathStyle, allowInsecure and prefix must all be
// honored by the Go transport.
func TestParseS3SettingsUsesRendererFieldNames(t *testing.T) {
	raw := json.RawMessage(`{
		"endpoint": "https://s3.example.com",
		"region": "us-east-1",
		"bucket": "vault",
		"accessKeyId": "AKIAEXAMPLE",
		"secretAccessKey": "renderer-secret",
		"sessionToken": "tok",
		"prefix": "/backups/lemonssh/",
		"forcePathStyle": false,
		"allowInsecure": true
	}`)
	settings, err := parseS3Settings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if settings.secretAccessKey != "renderer-secret" {
		t.Fatalf("secretAccessKey = %q", settings.secretAccessKey)
	}
	if settings.usePathStyle {
		t.Fatal("explicit forcePathStyle=false must select virtual-host style")
	}
	if !settings.allowInsecure {
		t.Fatal("allowInsecure must be honored")
	}
	if settings.accessKeyID != "AKIAEXAMPLE" || settings.sessionToken != "tok" {
		t.Fatalf("credentials = %q/%q", settings.accessKeyID, settings.sessionToken)
	}
	if got := settings.snapshotKey(); got != "backups/lemonssh/lemonssh-vault.json" {
		t.Fatalf("snapshot key = %q", got)
	}
}

// TestParseS3SettingsDefaults: forcePathStyle defaults to true (matching the
// renderer fallback's `forcePathStyle ?? true`, which keeps MinIO/localhost
// endpoints working), and an empty prefix addresses the bucket root.
func TestParseS3SettingsDefaults(t *testing.T) {
	settings, err := parseS3Settings(json.RawMessage(`{"endpoint":"https://s3.example.com","region":"r","bucket":"b","accessKeyId":"a","secretAccessKey":"s"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !settings.usePathStyle {
		t.Fatal("forcePathStyle must default to true")
	}
	if settings.allowInsecure {
		t.Fatal("allowInsecure must default to false")
	}
	if got := settings.snapshotKey(); got != "lemonssh-vault.json" {
		t.Fatalf("snapshot key = %q", got)
	}
}

// TestParseS3SettingsLegacySecretKeyAlias: configs stored under the previous
// field name keep working instead of silently signing with an empty secret.
func TestParseS3SettingsLegacySecretKeyAlias(t *testing.T) {
	settings, err := parseS3Settings(json.RawMessage(`{"endpoint":"https://s3.example.com","region":"r","bucket":"b","accessKeyId":"a","secretKey":"legacy-secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	if settings.secretAccessKey != "legacy-secret" {
		t.Fatalf("legacy secretKey alias = %q", settings.secretAccessKey)
	}
	// secretAccessKey wins when both are present.
	both, err := parseS3Settings(json.RawMessage(`{"secretAccessKey":"new","secretKey":"old"}`))
	if err != nil {
		t.Fatal(err)
	}
	if both.secretAccessKey != "new" {
		t.Fatalf("secretAccessKey must win over the legacy alias, got %q", both.secretAccessKey)
	}
}

func TestS3SnapshotKeyMatchesRendererGetObjectKey(t *testing.T) {
	cases := []struct {
		prefix string
		want   string
	}{
		{"", "lemonssh-vault.json"},
		{"   ", "lemonssh-vault.json"},
		{"/", "lemonssh-vault.json"},
		{"/backups/", "backups/lemonssh-vault.json"},
		{"lemonssh//", "lemonssh/lemonssh-vault.json"},
	}
	for _, tc := range cases {
		if got := s3SnapshotKey(tc.prefix); got != tc.want {
			t.Fatalf("s3SnapshotKey(%q) = %q, want %q", tc.prefix, got, tc.want)
		}
	}
	if got := s3LegacySnapshotKey(""); got != "netcatty-vault.json" {
		t.Fatalf("s3LegacySnapshotKey(\"\") = %q, want netcatty-vault.json", got)
	}
}

// TestWebdavClientAuthValidation: digest and token configs fail closed with a
// clear message when their secret is missing, unknown auth types are rejected
// and the basic default still builds.
func TestWebdavClientAuthValidation(t *testing.T) {
	if _, err := webdavClient(CloudSyncWebDAVConfig{Endpoint: "https://dav.example.com", AuthType: "digest", Username: "u"}); err == nil || !strings.Contains(err.Error(), "digest auth requires username and password") {
		t.Fatalf("digest without password: got %v", err)
	}
	if _, err := webdavClient(CloudSyncWebDAVConfig{Endpoint: "https://dav.example.com", AuthType: "digest"}); err == nil {
		t.Fatal("digest without credentials must fail")
	}
	if _, err := webdavClient(CloudSyncWebDAVConfig{Endpoint: "https://dav.example.com", AuthType: "token"}); err == nil || !strings.Contains(err.Error(), "token auth requires a token") {
		t.Fatalf("token without token: got %v", err)
	}
	if _, err := webdavClient(CloudSyncWebDAVConfig{Endpoint: "https://dav.example.com", AuthType: "ntlm"}); err == nil || !strings.Contains(err.Error(), "unsupported WebDAV auth type") {
		t.Fatalf("unknown auth type: got %v", err)
	}
	for _, authType := range []string{"", "basic", "Digest", "TOKEN"} {
		config := CloudSyncWebDAVConfig{Endpoint: "https://dav.example.com", AuthType: authType, Username: "u", Password: "p", Token: "t"}
		if _, err := webdavClient(config); err != nil {
			t.Fatalf("authType %q: %v", authType, err)
		}
	}
}
