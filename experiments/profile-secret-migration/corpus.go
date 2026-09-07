package migrationprobe

import "errors"

var requiredFixtureIDs = []string{
	"host.password", "host.telnet-password", "host.proxy-password",
	"key.private-key", "key.passphrase", "identity.password", "proxy-profile.password",
	"group.password", "group.telnet-password", "group.proxy-password", "default-key.passphrase",
	"ai.provider-api-key", "ai.external-agent-api-key", "ai.web-search-api-key",
	"sync.oauth-access-token", "sync.oauth-refresh-token", "sync.webdav-password",
	"sync.webdav-token", "sync.s3-secret", "sync.s3-session-token", "legacy.github-token",
	"legacy.gist-token", "plugin.sync-opaque-config", "plugin.durable-credential-ref",
	"raw.cloud-master-password", "raw.local-backup-payload", "raw.plugin-secret-store-value",
	"edge.unicode", "edge.multiline-private-key", "edge.max-plaintext", "edge.enc-v1-prefix",
}

var requiredMetadata = []metadataReceipt{
	{MetadataID: "metadata.app-lock-verifier", Classification: "verifier"},
	{MetadataID: "metadata.sync-master-key-config", Classification: "key-derivation-config"},
	{MetadataID: "metadata.empty-absent-optional", Classification: "empty-or-absent"},
}

func validateCorpusReceipt(receipt sanitizedReceipt) error {
	if receipt.ProtocolVersion != ProtocolVersion || receipt.Provider != ProviderName ||
		receipt.ProviderVersion != ProviderVersion || receipt.EnvelopeVersion != EnvelopeVersion {
		return errors.New("corpus receipt version mismatch")
	}
	if !receipt.Passed || !receipt.AllSealed || !receipt.AllOpened || !receipt.ExactMatches || len(receipt.ErrorCodes) != 0 {
		return errors.New("corpus receipt status mismatch")
	}
	if receipt.Counts.TotalRecords != len(requiredFixtureIDs)+len(requiredMetadata) ||
		receipt.Counts.SecretRecords != len(requiredFixtureIDs) ||
		receipt.Counts.MetadataRecords != len(requiredMetadata) ||
		receipt.Counts.TotalRecords > MaxRecords {
		return errors.New("corpus receipt count mismatch")
	}
	if len(receipt.Fixtures) != len(requiredFixtureIDs) || len(receipt.Metadata) != len(requiredMetadata) {
		return errors.New("corpus receipt inventory mismatch")
	}
	for index, id := range requiredFixtureIDs {
		fixture := receipt.Fixtures[index]
		expectedFormat := "enc:v1"
		if index >= 24 && index <= 26 {
			expectedFormat = "safeStorage-raw"
		}
		if fixture.FixtureID != id || fixture.Purpose != "profile-secret-migration/"+id || fixture.SourceFormat != expectedFormat {
			return errors.New("corpus fixture mismatch")
		}
	}
	for index, expected := range requiredMetadata {
		if receipt.Metadata[index] != expected {
			return errors.New("corpus metadata mismatch")
		}
	}
	return nil
}
