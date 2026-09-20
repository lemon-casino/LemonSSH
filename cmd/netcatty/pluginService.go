package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	archivepath "path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/binaricat/netcatty/internal/platform/credentials"
	"github.com/binaricat/netcatty/internal/plugin/host"
	"github.com/binaricat/netcatty/internal/plugin/manifest"
	"github.com/binaricat/netcatty/internal/plugin/native"
	"github.com/binaricat/netcatty/internal/plugin/permissions"
	pluginstore "github.com/binaricat/netcatty/internal/plugin/store"
	"github.com/binaricat/netcatty/internal/plugin/ui"
	"github.com/binaricat/netcatty/internal/plugin/v1reject"
	"github.com/binaricat/netcatty/internal/plugin/wasm"
)

type PluginService struct {
	mu         sync.Mutex
	store      *pluginstore.Store
	packageDir string
	runtime    *wasm.Runtime
	native     *native.Runtime
	broker     *permissions.Broker
}

func newPluginService() *PluginService {
	runtime, _ := wasm.NewRuntime(context.Background())
	broker := permissions.NewBroker(nil)
	nativeRuntime := native.NewRuntime(nil, 8)
	nativeRuntime.SetBroker(broker)
	service := &PluginService{store: pluginstore.New(), runtime: runtime, native: nativeRuntime, broker: broker}
	// Recovery: staged installs from an interrupted publish never run.
	service.store.RecoverStaged()
	return service
}

func newPluginServiceAt(path string) (*PluginService, error) {
	inventory, err := pluginstore.Open(path)
	if err != nil {
		return nil, err
	}
	service := newPluginService()
	service.store = inventory
	service.packageDir = filepath.Join(filepath.Dir(path), "packages")
	if err := os.MkdirAll(service.packageDir, 0o700); err != nil {
		return nil, err
	}
	service.restoreEnabledPackages()
	return service, nil
}

func (s *PluginService) List() []*pluginstore.PackageRecord {
	return s.store.List()
}

// parseManifestV2 unmarshals and validates a manifest v2 document.
func parseManifestV2(manifestJSON string) (manifest.Manifest, error) {
	legacy, err := v1reject.DetectV1([]byte(manifestJSON))
	if err != nil {
		return manifest.Manifest{}, err
	}
	if legacy {
		return manifest.Manifest{}, v1reject.Reject("package")
	}
	var parsed manifest.Manifest
	if err := json.Unmarshal([]byte(manifestJSON), &parsed); err != nil {
		return manifest.Manifest{}, fmt.Errorf("manifest json: %w", err)
	}
	if err := manifest.Validate(parsed); err != nil {
		return manifest.Manifest{}, err
	}
	return parsed, nil
}

func (s *PluginService) Install(pluginID, version, sha256Hex, manifestJSON string) (*pluginstore.PackageRecord, error) {
	// The manifest is the contract: an invalid v2 document never enters the
	// store, so the WASM/native runtimes can trust the stored snapshot.
	parsed, err := parseManifestV2(manifestJSON)
	if err != nil {
		return nil, err
	}
	if parsed.Name != pluginID || parsed.Version != version {
		return nil, fmt.Errorf("plugin identity does not match manifest")
	}
	return s.store.Install(pluginID, version, sha256Hex, json.RawMessage(manifestJSON))
}

// StageInstall begins a two-phase publish: validate, stage, then commit.
func (s *PluginService) StageInstall(pluginID, version, sha256Hex, manifestJSON string) (*pluginstore.PackageRecord, error) {
	parsed, err := parseManifestV2(manifestJSON)
	if err != nil {
		return nil, err
	}
	if parsed.Name != pluginID || parsed.Version != version {
		return nil, fmt.Errorf("plugin identity does not match manifest")
	}
	return s.store.StageInstall(pluginID, version, sha256Hex, json.RawMessage(manifestJSON))
}

// CommitStaged promotes a staged plugin to installed.
func (s *PluginService) CommitStaged(pluginID string) (*pluginstore.PackageRecord, error) {
	return s.store.CommitStaged(pluginID)
}

// RecoverStaged drops installs that never committed (interrupted publish).
func (s *PluginService) RecoverStaged() int {
	return s.store.RecoverStaged()
}

func (s *PluginService) Settings(pluginID string) (map[string]any, error) {
	return (host.Host{Store: s.store, Broker: s.broker, Credentials: credentials.NewOSProvider()}).Settings(pluginID)
}

func (s *PluginService) SetSetting(pluginID, settingID, valueJSON string) error {
	return (host.Host{Store: s.store, Broker: s.broker, Credentials: credentials.NewOSProvider()}).SetSetting(pluginID, settingID, valueJSON)
}

func (s *PluginService) UISchema(pluginID string) (*ui.Schema, error) {
	return (host.Host{Store: s.store, Broker: s.broker, Credentials: credentials.NewOSProvider()}).UI(pluginID)
}

// GrantPermission is a trusted host UI approval entrypoint, never a plugin RPC.
func (s *PluginService) GrantPermission(pluginID, kind, resource, mode, lifetime string) error {
	return (host.Host{Store: s.store, Broker: s.broker, Credentials: credentials.NewOSProvider()}).Grant(pluginID, kind, resource, mode, lifetime)
}

func (s *PluginService) AuthorizePermission(pluginID, kind, resource, mode string) error {
	return (host.Host{Store: s.store, Broker: s.broker, Credentials: credentials.NewOSProvider()}).Authorize(pluginID, kind, resource, mode)
}

func (s *PluginService) SetEnabled(pluginID string, enabled bool) error {
	state := pluginstore.StateDisabled
	if enabled {
		state = pluginstore.StateEnabled
	}
	if enabled {
		record, ok := s.store.Get(pluginID)
		if !ok {
			return pluginstore.ErrNotInstalled
		}
		if archivePath := record.Labels["packagePath"]; archivePath != "" {
			_, _, wasmBytes, err := readPluginArchive(archivePath)
			if err != nil {
				return err
			}
			if err = s.InstantiateWASM(pluginID, wasmBytes); err != nil && err != wasm.ErrAlreadyInstant {
				return err
			}
		}
	}
	if err := s.store.SetState(pluginID, state); err != nil {
		return err
	}
	if !enabled {
		if s.runtime != nil {
			_ = s.runtime.CloseModule(pluginID)
		}
		s.broker.RevokeAll(pluginID)
	}
	return nil
}

func (s *PluginService) InstantiateWASM(pluginID string, wasmBytes []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime == nil {
		runtime, err := wasm.NewRuntime(context.Background())
		if err != nil {
			return err
		}
		s.runtime = runtime
	}
	config := wasm.Config{PluginID: pluginID, WASMBytes: wasmBytes}
	// The stored manifest caps linear memory so a plugin cannot grow past the
	// entrypoint.memoryMB it declared at install time.
	if record, ok := s.store.Get(pluginID); ok {
		var parsed manifest.Manifest
		if err := json.Unmarshal(record.Manifest, &parsed); err == nil && parsed.Entrypoint.MemoryMB > 0 {
			config.MemoryLimitPages = wasm.MemoryPagesFromMB(parsed.Entrypoint.MemoryMB)
		}
	}
	return s.runtime.InstantiateWithConfig(context.Background(), config)
}

// GrantNative records a session-scoped companion.execute grant. StartNative
// remains fail-closed until this grant exists.
func (s *PluginService) GrantNative(pluginID string) error {
	if s.broker == nil {
		return nil
	}
	_, err := s.broker.Grant(pluginID, "companion.execute:write", permissions.LifetimeSession, 0)
	return err
}

// NativeStartRequest is the Wails-facing native plugin spawn payload.
type NativeStartRequest struct {
	PluginID   string            `json:"pluginId"`
	BinaryPath string            `json:"binaryPath"`
	SHA256     string            `json:"sha256"`
	Args       []string          `json:"args"`
	WorkDir    string            `json:"workDir"`
	Env        map[string]string `json:"env"`
}

// StartNative verifies, authorizes and spawns a contained native plugin process.
func (s *PluginService) StartNative(request NativeStartRequest) error {
	return s.native.Start(context.Background(), native.Spec{
		PluginID:   request.PluginID,
		BinaryPath: request.BinaryPath,
		SHA256:     request.SHA256,
		Args:       request.Args,
		WorkDir:    request.WorkDir,
		Env:        request.Env,
	})
}

// StopNative terminates a native plugin process.
func (s *PluginService) StopNative(pluginID string) error {
	return s.native.Stop(pluginID)
}

// CallNative sends one framed RPC request to a live native plugin.
func (s *PluginService) CallNative(pluginID, method, paramsJSON string) (string, error) {
	var params json.RawMessage
	if paramsJSON != "" {
		params = json.RawMessage(paramsJSON)
	}
	result, err := s.native.Call(context.Background(), pluginID, method, params)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// NativeRunning reports whether a native plugin process is live.
func (s *PluginService) NativeRunning(pluginID string) bool {
	return s.native.IsRunning(pluginID)
}

type PluginPackageInstallOptions struct {
	Enable bool `json:"enable"`
}

const maxPluginPackageBytes = 64 << 20

func safePluginArchivePath(raw string) (string, bool) {
	if raw == "" || strings.Contains(raw, `\`) {
		return "", false
	}
	trimmed := strings.TrimSuffix(raw, "/")
	cleaned := archivepath.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || archivepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	if cleaned != trimmed {
		return "", false
	}
	return cleaned, true
}

func readPluginArchive(path string) (manifest.Manifest, string, []byte, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return manifest.Manifest{}, "", nil, fmt.Errorf("open plugin package: %w", err)
	}
	defer archive.Close()
	if len(archive.File) > 512 {
		return manifest.Manifest{}, "", nil, fmt.Errorf("plugin package has too many files")
	}
	var manifestBytes []byte
	files := map[string]*zip.File{}
	for _, file := range archive.File {
		name, safe := safePluginArchivePath(file.Name)
		if !safe {
			return manifest.Manifest{}, "", nil, fmt.Errorf("unsafe plugin package path %q", file.Name)
		}
		if file.UncompressedSize64 > maxPluginPackageBytes {
			return manifest.Manifest{}, "", nil, fmt.Errorf("plugin file is too large")
		}
		files[name] = file
		if name == "lemonssh.plugin.json" || name == "netcatty.plugin.json" || name == "manifest.json" {
			reader, openErr := file.Open()
			if openErr != nil {
				return manifest.Manifest{}, "", nil, openErr
			}
			manifestBytes, openErr = io.ReadAll(io.LimitReader(reader, maxPluginPackageBytes+1))
			reader.Close()
			if openErr != nil {
				return manifest.Manifest{}, "", nil, openErr
			}
		}
	}
	if len(manifestBytes) == 0 {
		return manifest.Manifest{}, "", nil, fmt.Errorf("plugin package does not contain a v2 manifest")
	}
	parsed, err := parseManifestV2(string(manifestBytes))
	if err != nil {
		return manifest.Manifest{}, "", nil, err
	}
	wasmFile := files[filepath.ToSlash(parsed.Entrypoint.WASM)]
	if wasmFile == nil {
		return manifest.Manifest{}, "", nil, fmt.Errorf("plugin WASM entrypoint is missing")
	}
	reader, err := wasmFile.Open()
	if err != nil {
		return manifest.Manifest{}, "", nil, err
	}
	wasmBytes, err := io.ReadAll(io.LimitReader(reader, maxPluginPackageBytes+1))
	reader.Close()
	if err != nil {
		return manifest.Manifest{}, "", nil, fmt.Errorf("read plugin WASM: %w", err)
	}
	if len(wasmBytes) > maxPluginPackageBytes {
		return manifest.Manifest{}, "", nil, fmt.Errorf("plugin WASM is too large")
	}
	sum := sha256.Sum256(wasmBytes)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), parsed.Entrypoint.SHA256) {
		return manifest.Manifest{}, "", nil, fmt.Errorf("plugin WASM checksum mismatch")
	}
	return parsed, string(manifestBytes), wasmBytes, nil
}

func (s *PluginService) InstallPackage(archivePath string, options PluginPackageInstallOptions) (*pluginstore.PackageRecord, error) {
	parsed, manifestJSON, wasmBytes, err := readPluginArchive(archivePath)
	if err != nil {
		return nil, err
	}
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, err
	}
	archiveSum := sha256.Sum256(archiveBytes)
	record, err := s.store.Install(parsed.Name, parsed.Version, hex.EncodeToString(archiveSum[:]), json.RawMessage(manifestJSON))
	if err != nil {
		return nil, err
	}
	if s.packageDir != "" {
		destination := filepath.Join(s.packageDir, parsed.Name+"-"+parsed.Version+".ncpkg")
		if writeErr := os.WriteFile(destination, archiveBytes, 0o600); writeErr != nil {
			_, _ = s.Uninstall(parsed.Name)
			return nil, writeErr
		}
		if labelErr := s.store.SetLabels(parsed.Name, map[string]string{"packagePath": destination}); labelErr != nil {
			_, _ = s.Uninstall(parsed.Name)
			return nil, labelErr
		}
	}
	if options.Enable {
		if err = s.InstantiateWASM(parsed.Name, wasmBytes); err != nil {
			_, _ = s.Uninstall(parsed.Name)
			return nil, err
		}
		if err = s.store.SetState(parsed.Name, pluginstore.StateEnabled); err != nil {
			_, _ = s.Uninstall(parsed.Name)
			return nil, err
		}
	}
	record, _ = s.store.Get(parsed.Name)
	return record, nil
}

func (s *PluginService) restoreEnabledPackages() {
	for _, record := range s.store.List() {
		if record == nil || record.State != pluginstore.StateEnabled {
			continue
		}
		archivePath := record.Labels["packagePath"]
		if archivePath == "" {
			_ = s.store.SetState(record.PluginID, pluginstore.StateDisabled)
			continue
		}
		_, _, wasmBytes, err := readPluginArchive(archivePath)
		if err != nil || s.InstantiateWASM(record.PluginID, wasmBytes) != nil {
			_ = s.store.SetState(record.PluginID, pluginstore.StateDisabled)
		}
	}
}

func (s *PluginService) ResetSetting(pluginID, settingID string) error {
	if _, err := s.UISchema(pluginID); err != nil {
		return err
	}
	return s.store.DeleteSetting(pluginID, settingID)
}

func (s *PluginService) Restart(pluginID string) (*pluginstore.PackageRecord, error) {
	record, ok := s.store.Get(pluginID)
	if !ok {
		return nil, pluginstore.ErrNotInstalled
	}
	if s.native != nil && s.native.IsRunning(pluginID) {
		_ = s.native.Stop(pluginID)
	}
	if s.runtime != nil {
		_ = s.runtime.CloseModule(pluginID)
	}
	if record.State == pluginstore.StateDisabled {
		return record, nil
	}
	if err := s.SetEnabled(pluginID, true); err != nil {
		return nil, err
	}
	record, _ = s.store.Get(pluginID)
	return record, nil
}

func (s *PluginService) Uninstall(pluginID string) (bool, error) {
	if s.native != nil && s.native.IsRunning(pluginID) {
		_ = s.native.Stop(pluginID)
	}
	if s.runtime != nil {
		_ = s.runtime.CloseModule(pluginID)
	}
	if s.broker != nil {
		s.broker.RevokeAll(pluginID)
	}
	record, _ := s.store.Get(pluginID)
	if err := s.store.Uninstall(pluginID); err != nil {
		return false, err
	}
	if record != nil && record.Labels["packagePath"] != "" {
		_ = os.Remove(record.Labels["packagePath"])
	}
	return true, nil
}
