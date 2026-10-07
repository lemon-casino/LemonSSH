package store

// Staging, atomic promotion, backup manifests and migration receipts. The
// crash matrix is: crash before promotion (staging discarded or resumed),
// crash during promotion (backup path holds the prior store), crash after
// promotion but before receipt (receipt regenerated from the promoted store).

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BackupManifest describes the protective backup written at promotion.
type BackupManifest struct {
	CreatedAtMS    int64  `json:"createdAtMs"`
	OriginalPath   string `json:"originalPath"`
	BackupPath     string `json:"backupPath"`
	OriginalSHA256 string `json:"originalSha256"`
	SizeBytes      int64  `json:"sizeBytes"`
}

// MigrationReceipt is written after a successful promotion.
type MigrationReceipt struct {
	CompletedAtMS     int64           `json:"completedAtMs"`
	TargetPath        string          `json:"targetPath"`
	BackupManifest    *BackupManifest `json:"backupManifest"`
	SourceFingerprint string          `json:"sourceFingerprint"`
	SchemaVersion     int             `json:"schemaVersion"`
}

// StageProfile builds a complete staging store from mutations and returns its
// path. The staging store is independent of the live store; promotion is a
// later, explicit step.
func StageProfile(stagingDir string, mutations []Mutation) (string, error) {
	if err := os.MkdirAll(stagingDir, 0o700); err != nil {
		return "", err
	}
	stagingPath := filepath.Join(stagingDir, "profile.db")
	_ = os.Remove(stagingPath)
	staging, err := Open(stagingPath, nil)
	if err != nil {
		return "", err
	}
	defer staging.Close()
	if _, err := staging.Write(WriteRequest{Mutations: mutations}); err != nil {
		return "", err
	}
	if err := markComplete(stagingPath); err != nil {
		return "", err
	}
	return stagingPath, nil
}

// completeMarker distinguishes a finished staging store from a crashed one.
const completeMarker = "staging.complete"

func markComplete(stagingPath string) error {
	return os.WriteFile(stagingPath+"."+completeMarker, []byte("ok"), 0o600)
}

func stagingComplete(stagingPath string) bool {
	_, err := os.Stat(stagingPath + "." + completeMarker)
	return err == nil
}

// PromoteProfile atomically replaces targetPath with stagingPath. It writes a
// protective backup of the current target plus a backup manifest, then moves
// the staging store into place, then writes the migration receipt. Any
// failure leaves either the original or the promoted store in place, never a
// partial file.
func PromoteProfile(stagingPath, targetPath, backupDir, sourceFingerprint string) (*MigrationReceipt, error) {
	if !stagingComplete(stagingPath) {
		return nil, ErrStagingNotComplete
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return nil, err
	}

	manifest, err := backupOriginal(targetPath, backupDir)
	if err != nil {
		return nil, err
	}

	// Close the staging handle before renaming: bbolt keeps the file locked
	// while open, and Windows cannot rename an open file.
	if err := closeBolt(stagingPath); err != nil {
		return nil, err
	}
	if err := os.Rename(stagingPath, targetPath); err != nil {
		return nil, fmt.Errorf("profile promotion rename failed: %w", err)
	}
	_ = os.Remove(stagingPath + "." + completeMarker)

	receipt := &MigrationReceipt{
		CompletedAtMS:     time.Now().UnixMilli(),
		TargetPath:        targetPath,
		BackupManifest:    manifest,
		SourceFingerprint: sourceFingerprint,
		SchemaVersion:     SchemaVersion,
	}
	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(receiptPath(targetPath), receiptBytes, 0o600); err != nil {
		return nil, err
	}
	return receipt, nil
}

func receiptPath(targetPath string) string {
	return targetPath + ".migration-receipt.json"
}

// ReadMigrationReceipt returns the receipt for a promoted store, if present.
func ReadMigrationReceipt(targetPath string) (*MigrationReceipt, error) {
	data, err := os.ReadFile(receiptPath(targetPath))
	if err != nil {
		return nil, err
	}
	var receipt MigrationReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func backupOriginal(targetPath, backupDir string) (*BackupManifest, error) {
	original, err := os.ReadFile(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // fresh profile; nothing to back up
		}
		return nil, err
	}
	sum := sha256.Sum256(original)
	createdMS := time.Now().UnixMilli()
	manifest := &BackupManifest{
		CreatedAtMS:    createdMS,
		OriginalPath:   targetPath,
		OriginalSHA256: hex.EncodeToString(sum[:]),
		SizeBytes:      int64(len(original)),
		BackupPath:     filepath.Join(backupDir, fmt.Sprintf("profile-%d.db", createdMS)),
	}
	if err := os.WriteFile(manifest.BackupPath, original, 0o600); err != nil {
		return nil, err
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(manifest.BackupPath+".manifest.json", manifestBytes, 0o600); err != nil {
		return nil, err
	}
	return manifest, nil
}
