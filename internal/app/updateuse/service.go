// Package updateuse owns the shell-neutral in-app update use cases: GitHub
// Releases discovery, artifact download with progress reporting, integrity
// verification (checksums.txt plus the optional ed25519-signed release
// manifest from internal/platform/updater) and the self-replace install
// (atomic binary swap + delayed relaunch).
//
// The distribution format this channel serves is the repository's own release
// artifact: a single self-contained LemonSSH-{version}-{goos}-{goarch}
// executable produced by scripts/package-wails.mjs alongside checksums.txt.
// Platforms without a matching artifact report supported=false so shells can
// degrade to the Releases page.
//
// Per the Wails v3 migration, this package is the canonical owner of update
// state. Shell facades (cmd/lemonssh) adapt it to Wails services and events
// and must not re-implement its behavior. It must never import a shell.
package updateuse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lemon-casino/lemonssh/internal/platform/updater"
)

// Status is the update lifecycle state surfaced to the renderer.
type Status string

const (
	StatusIdle        Status = "idle"
	StatusAvailable   Status = "available"
	StatusDownloading Status = "downloading"
	StatusReady       Status = "ready"
	StatusError       Status = "error"
)

// Wails event names pushed to every renderer window. The frontend facade in
// infrastructure/runtime/wails/wailsRuntimeClient.ts subscribes to these.
const (
	EventAvailable        = "update:available"
	EventNotAvailable     = "update:not-available"
	EventDownloadProgress = "update:download-progress"
	EventDownloaded       = "update:downloaded"
	EventError            = "update:error"
)

const (
	// DefaultReleasesURL mirrors the GitHub API endpoint the frontend
	// notification path uses (infrastructure/services/updateService.ts).
	DefaultReleasesURL = "https://api.github.com/repos/lemon-casino/LemonSSH/releases/latest"
	// checksumAsset is the sha256 manifest the packaging script writes next
	// to every artifact (scripts/package-wails.mjs).
	checksumAsset = "checksums.txt"
	// signedManifestAsset is the optional ed25519-signed release manifest
	// verified through internal/platform/updater when a public key is
	// configured at build time.
	signedManifestAsset = "release-manifest.json"
	checkInterval       = time.Hour
	progressThrottle    = 150 * time.Millisecond
	// updateDownloadLimit caps any single download at 1 GiB — far above
	// every artifact this channel serves, but bounded all the same.
	updateDownloadLimit = int64(1) << 30
)

// Errors surfaced to facades; messages are user-presentable.
var (
	ErrNoUpdate         = errors.New("no update available to download")
	ErrNotReady         = errors.New("no downloaded update ready to install")
	ErrChecksumMismatch = errors.New("downloaded artifact failed checksum verification")
	ErrNoExecutable     = errors.New("running executable path unavailable")
)

// CheckResult is the outcome of one release check. It matches the renderer
// bridge contract (types/global/lemonssh-bridge-app.d.ts).
type CheckResult struct {
	Available    bool   `json:"available"`
	Supported    bool   `json:"supported"`
	Checking     bool   `json:"checking,omitempty"`
	Ready        bool   `json:"ready,omitempty"`
	Downloading  bool   `json:"downloading,omitempty"`
	Version      string `json:"version,omitempty"`
	ReleaseNotes string `json:"releaseNotes,omitempty"`
	ReleaseDate  string `json:"releaseDate,omitempty"`
	Error        string `json:"error,omitempty"`
}

// DownloadResult is the outcome of a download request. The promise resolves
// when the download finishes; progress streams through EventDownloadProgress.
type DownloadResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// StatusSnapshot is the renderer-facing state hydration payload.
type StatusSnapshot struct {
	Status     Status `json:"status"`
	Percent    int    `json:"percent"`
	Error      string `json:"error"`
	Version    string `json:"version"`
	IsChecking bool   `json:"isChecking"`
}

// Progress is one download progress event.
type Progress struct {
	Percent        float64 `json:"percent"`
	BytesPerSecond float64 `json:"bytesPerSecond"`
	Transferred    int64   `json:"transferred"`
	Total          int64   `json:"total"`
}

// ReleaseAsset is one downloadable file attached to a release.
type ReleaseAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Release mirrors the subset of the GitHub Releases payload this package
// consumes.
type Release struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	HTMLURL     string         `json:"html_url"`
	Draft       bool           `json:"draft"`
	Prerelease  bool           `json:"prerelease"`
	PublishedAt string         `json:"published_at"`
	Assets      []ReleaseAsset `json:"assets"`
}

// Options configures a Service. Zero values select production defaults.
type Options struct {
	// CurrentVersion is the running build version ("0.0.0" marks dev builds,
	// which disable the channel like the frontend guard does).
	CurrentVersion string
	GOOS           string // defaults to runtime.GOOS
	GOARCH         string // defaults to runtime.GOARCH
	ReleasesURL    string // defaults to DefaultReleasesURL
	HTTPClient     *http.Client
	// StorageDir holds downloaded update artifacts (profile "updates" dir).
	StorageDir string
	// PublicKeyHex optionally holds an ed25519 public key (hex). When set,
	// a release-manifest.json asset must carry a valid signature whose
	// artifact digest matches the download; when empty the signed manifest
	// is skipped and only checksums.txt (when published) is enforced.
	PublicKeyHex string
	// Executable overrides the running binary path (tests).
	Executable string
	// Emit receives Wails event broadcasts (name, payload). Nil disables.
	Emit func(name string, payload any)
	// Quit requests application shutdown after a successful install.
	Quit func()
	// StartDetached spawns the relaunch helper process (tests inject a
	// recorder). Nil uses the platform default.
	StartDetached func(name string, argv []string) error
	// AutoUpdate is the initial auto-download toggle.
	AutoUpdate bool
	// PersistAutoUpdate / PersistLastCheck / PersistedLastCheck back the
	// toggle and the startup-check throttle with the profile store. Nil
	// hooks disable persistence.
	PersistAutoUpdate  func(enabled bool) error
	PersistLastCheck   func(at int64) error
	PersistedLastCheck func() int64
	Now                func() time.Time
}

// Service owns the update state machine. It is safe for concurrent use.
type Service struct {
	mu sync.Mutex

	opts   Options
	client *http.Client
	now    func() time.Time

	status        Status
	percent       int
	errorMsg      string
	version       string
	notes         string
	publishedAt   string
	artifactName  string
	artifactURL   string
	totalSize     int64
	releaseAssets []ReleaseAsset

	autoUpdate   bool
	checking     bool
	downloading  bool
	lastProgress time.Time
}

// New builds a Service from options.
func New(opts Options) *Service {
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.GOARCH == "" {
		opts.GOARCH = runtime.GOARCH
	}
	if opts.ReleasesURL == "" {
		opts.ReleasesURL = DefaultReleasesURL
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	s := &Service{
		opts:       opts,
		client:     opts.HTTPClient,
		now:        opts.Now,
		status:     StatusIdle,
		autoUpdate: opts.AutoUpdate,
	}
	if s.client == nil {
		s.client = &http.Client{Timeout: 30 * time.Second}
	}
	return s
}

// SetEmitter wires the event broadcaster after construction (the shell owns
// the Wails event loop and installs its Emit closure late).
func (s *Service) SetEmitter(emit func(name string, payload any)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opts.Emit = emit
}

// SetQuit wires the application shutdown hook used after a successful
// install.
func (s *Service) SetQuit(quit func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opts.Quit = quit
}

// StatusSnapshot reports the current state for renderer hydration.
func (s *Service) StatusSnapshot() StatusSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return StatusSnapshot{
		Status:     s.status,
		Percent:    s.percent,
		Error:      s.errorMsg,
		Version:    s.version,
		IsChecking: s.checking,
	}
}

// AutoUpdate reports the auto-download toggle.
func (s *Service) AutoUpdate() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.autoUpdate
}

// SetAutoUpdate stores and persists the auto-download toggle.
func (s *Service) SetAutoUpdate(enabled bool) {
	s.mu.Lock()
	s.autoUpdate = enabled
	persist := s.opts.PersistAutoUpdate
	s.mu.Unlock()
	if persist != nil {
		if err := persist(enabled); err != nil {
			log.Printf("update: persist auto-update toggle failed: %v", err)
		}
	}
}

// Check fetches the latest release and updates state. It returns the
// renderer-facing result; when auto-download is enabled and a compatible
// artifact exists the download starts in the background.
func (s *Service) Check() CheckResult {
	s.mu.Lock()
	if s.checking {
		result := s.checkResultLocked()
		result.Checking = true
		s.mu.Unlock()
		return result
	}
	s.checking = true
	s.mu.Unlock()

	result, release, err := s.fetchLatest(s.opts.CurrentVersion)

	s.mu.Lock()
	s.checking = false
	if err != nil {
		result = CheckResult{Available: false, Supported: s.supportedLocked(), Error: err.Error()}
	}
	if release != nil {
		artifact := compatibleAsset(release.Assets, result.Version, s.opts.GOOS, s.opts.GOARCH)
		if artifact == nil {
			// An update exists but not for this platform/arch: the
			// self-update channel cannot serve it, so report unsupported and
			// let the shell open the Releases page instead.
			result.Available = false
			result.Supported = false
			result.Version = ""
			result.ReleaseNotes = ""
			result.ReleaseDate = ""
		} else {
			s.status = StatusAvailable
			s.percent = 0
			s.errorMsg = ""
			s.version = result.Version
			s.notes = result.ReleaseNotes
			s.publishedAt = result.ReleaseDate
			s.artifactName = artifact.Name
			s.artifactURL = artifact.BrowserDownloadURL
			s.totalSize = artifact.Size
			s.releaseAssets = release.Assets
		}
	} else if err == nil {
		// No compatible update: keep any existing download state (a ready
		// install must not be invalidated by a later "no update" answer).
		if s.status == StatusIdle || s.status == StatusAvailable {
			s.status = StatusIdle
		}
		s.releaseAssets = nil
		s.errorMsg = result.Error
		if !result.Available {
			s.version = ""
		}
	}
	available := result.Available
	autoDownload := available && s.autoUpdate && s.status != StatusDownloading && s.status != StatusReady
	s.mu.Unlock()

	if err != nil {
		s.emit(EventError, map[string]any{"error": err.Error()})
		return result
	}
	if available {
		s.emit(EventAvailable, map[string]any{
			"version":      result.Version,
			"releaseNotes": result.ReleaseNotes,
			"releaseDate":  result.ReleaseDate,
		})
		if autoDownload {
			go func() { _ = s.Download() }()
		}
		return result
	}
	s.emit(EventNotAvailable, nil)
	return result
}

// checkResultLocked renders the current state as a CheckResult.
// Caller must hold s.mu.
func (s *Service) checkResultLocked() CheckResult {
	return CheckResult{
		Available:    s.status == StatusAvailable,
		Supported:    s.supportedLocked(),
		Ready:        s.status == StatusReady,
		Downloading:  s.status == StatusDownloading,
		Version:      s.version,
		ReleaseNotes: s.notes,
		ReleaseDate:  s.publishedAt,
		Error:        s.errorMsg,
	}
}

// supportedLocked reports whether the running build can self-update at all.
func (s *Service) supportedLocked() bool {
	return !IsDevVersion(s.opts.CurrentVersion)
}

// fetchLatest queries the Releases API for the newest stable release. A
// non-nil release signals "newer version published"; callers pick the
// platform artifact from release.Assets.
func (s *Service) fetchLatest(currentVersion string) (CheckResult, *Release, error) {
	if IsDevVersion(currentVersion) {
		// Dev builds cannot self-update meaningfully; the shell degrades to
		// the Releases page exactly like the Electron shell did.
		return CheckResult{Available: false, Supported: false}, nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.opts.ReleasesURL, nil)
	if err != nil {
		return CheckResult{}, nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := s.client.Do(req)
	if err != nil {
		return CheckResult{}, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CheckResult{}, nil, fmt.Errorf("releases API returned %d", resp.StatusCode)
	}
	var release Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&release); err != nil {
		return CheckResult{}, nil, err
	}
	if release.Draft || release.Prerelease || release.TagName == "" {
		return CheckResult{Available: false, Supported: true}, nil, nil
	}
	version := strings.TrimPrefix(strings.TrimPrefix(release.TagName, "v"), "V")
	if CompareVersions(version, currentVersion) <= 0 {
		return CheckResult{Available: false, Supported: true}, nil, nil
	}
	return CheckResult{
		Available:    true,
		Supported:    true,
		Version:      version,
		ReleaseNotes: release.Body,
		ReleaseDate:  release.PublishedAt,
	}, &release, nil
}

// Download downloads the pending artifact, verifies it and transitions the
// state machine to ready. Progress streams through EventDownloadProgress.
func (s *Service) Download() DownloadResult {
	s.mu.Lock()
	if s.downloading || s.status == StatusDownloading || s.status == StatusReady {
		s.mu.Unlock()
		return DownloadResult{Success: true}
	}
	if s.status != StatusAvailable || s.artifactURL == "" {
		message := ErrNoUpdate.Error()
		s.status = StatusError
		s.errorMsg = message
		s.mu.Unlock()
		s.emit(EventError, map[string]any{"error": message})
		return DownloadResult{Success: false, Error: message}
	}
	url := s.artifactURL
	name := s.artifactName
	total := s.totalSize
	s.downloading = true
	s.status = StatusDownloading
	s.percent = 0
	s.errorMsg = ""
	s.lastProgress = time.Time{}
	s.mu.Unlock()

	fail := func(err error) DownloadResult {
		s.mu.Lock()
		s.downloading = false
		s.status = StatusError
		s.errorMsg = err.Error()
		s.percent = 0
		s.mu.Unlock()
		s.emit(EventError, map[string]any{"error": err.Error()})
		return DownloadResult{Success: false, Error: err.Error()}
	}

	if err := os.MkdirAll(s.opts.StorageDir, 0o755); err != nil {
		return fail(err)
	}
	finalPath := filepath.Join(s.opts.StorageDir, name)
	partPath := finalPath + ".part"
	_ = os.Remove(partPath)

	checksums, err := s.fetchChecksums()
	if err != nil {
		// checksums.txt published but unreadable: refuse an unverifiable
		// binary rather than installing bytes nobody vouched for.
		return fail(fmt.Errorf("checksum manifest unavailable: %w", err))
	}
	if err := s.downloadToFile(url, partPath, total); err != nil {
		_ = os.Remove(partPath)
		return fail(err)
	}
	if err := s.downloadSignedManifest(); err != nil {
		_ = os.Remove(partPath)
		return fail(fmt.Errorf("release manifest unavailable: %w", err))
	}
	digest, err := fileSHA256(partPath)
	if err != nil {
		_ = os.Remove(partPath)
		return fail(err)
	}
	if err := verifyIntegrity(digest, name, checksums, s.opts.StorageDir, s.opts.PublicKeyHex, s.opts.GOOS, s.opts.GOARCH, s.opts.Now); err != nil {
		_ = os.Remove(partPath)
		return fail(err)
	}
	if err := os.Rename(partPath, finalPath); err != nil {
		_ = os.Remove(partPath)
		return fail(err)
	}

	s.mu.Lock()
	s.downloading = false
	s.status = StatusReady
	s.percent = 100
	s.mu.Unlock()
	s.emit(EventDownloaded, nil)
	return DownloadResult{Success: true}
}

// Install atomically replaces the running executable with the downloaded
// artifact, schedules the delayed relaunch and requests shutdown.
func (s *Service) Install() error {
	s.mu.Lock()
	if s.status != StatusReady || s.artifactName == "" {
		s.mu.Unlock()
		return ErrNotReady
	}
	artifactPath := filepath.Join(s.opts.StorageDir, s.artifactName)
	s.mu.Unlock()

	exePath := s.opts.Executable
	if exePath == "" {
		resolved, err := os.Executable()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrNoExecutable, err)
		}
		exePath = resolved
	}

	if err := swapExecutable(exePath, artifactPath); err != nil {
		return err
	}
	log.Printf("update: replaced %s with %s; relaunching", exePath, artifactPath)
	scheduleRelaunch(exePath, s.opts.StartDetached)
	if s.opts.Quit != nil {
		s.opts.Quit()
	}
	return nil
}

// AutoCheck runs the throttled startup check (called from the shell after
// the first window's runtime is ready). Auto-download then proceeds only
// when the toggle is enabled.
func (s *Service) AutoCheck() {
	if IsDevVersion(s.opts.CurrentVersion) {
		return
	}
	if s.opts.PersistedLastCheck != nil {
		if last := s.opts.PersistedLastCheck(); last > 0 && s.now().UnixMilli()-last < checkInterval.Milliseconds() {
			return
		}
	}
	result := s.Check()
	if result.Error != "" {
		return // transient: never persist a failed check as "recent"
	}
	if s.opts.PersistLastCheck != nil {
		if err := s.opts.PersistLastCheck(s.now().UnixMilli()); err != nil {
			log.Printf("update: persist last-check stamp failed: %v", err)
		}
	}
}

func (s *Service) emit(name string, payload any) {
	if s.opts.Emit == nil {
		return
	}
	s.opts.Emit(name, payload)
}

// fetchChecksums downloads checksums.txt from the pending release. A missing
// asset yields an empty manifest with nil error; any other failure is an
// error so callers can refuse the download.
func (s *Service) fetchChecksums() (map[string]string, error) {
	asset := s.assetByName(checksumAsset)
	if asset == nil {
		return nil, nil
	}
	data, err := s.fetchURL(asset.BrowserDownloadURL, 1<<20)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return parseChecksums(string(data)), nil
}

// assetByName looks up a pending release asset by exact name.
func (s *Service) assetByName(name string) *ReleaseAsset {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.releaseAssets {
		if s.releaseAssets[i].Name == name {
			asset := s.releaseAssets[i]
			return &asset
		}
	}
	return nil
}

// downloadSignedManifest stages release-manifest.json next to the artifact
// when the release publishes one.
func (s *Service) downloadSignedManifest() error {
	asset := s.assetByName(signedManifestAsset)
	if asset == nil {
		return nil
	}
	data, err := s.fetchURL(asset.BrowserDownloadURL, 1<<20)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(filepath.Join(s.opts.StorageDir, signedManifestAsset), data, 0o644)
}

// downloadToFile streams url into path, emitting throttled progress events.
func (s *Service) downloadToFile(url, path string, total int64) error {
	body, err := s.fetchURLReader(url)
	if err != nil {
		return err
	}
	defer body.Close()
	if total <= 0 {
		total = body.total
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, 64<<10)
	var transferred int64
	start := s.now()
	for {
		n, readErr := body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
			transferred += int64(n)
			s.reportProgress(transferred, total, start)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	s.reportProgress(transferred, total, start)
	return nil
}

// reportProgress emits a throttled download progress event (always emits the
// first and the final report).
func (s *Service) reportProgress(transferred, total int64, start time.Time) {
	now := s.now()
	s.mu.Lock()
	shouldEmit := s.downloading && now.Sub(s.lastProgress) >= progressThrottle
	if shouldEmit {
		s.lastProgress = now
	}
	percent := 0
	if total > 0 {
		percent = int(float64(transferred) / float64(total) * 100)
		if percent > 100 {
			percent = 100
		}
		s.percent = percent
	}
	s.mu.Unlock()

	elapsed := now.Sub(start).Seconds()
	var bytesPerSecond float64
	if elapsed > 0 {
		bytesPerSecond = float64(transferred) / elapsed
	}
	if shouldEmit {
		s.emit(EventDownloadProgress, Progress{
			Percent:        float64(percent),
			BytesPerSecond: bytesPerSecond,
			Transferred:    transferred,
			Total:          total,
		})
	}
}

// boundedBody wraps one HTTP response body with a size cap.
type boundedBody struct {
	io.Reader
	total  int64
	closer io.Closer
}

func (b *boundedBody) Close() error { return b.closer.Close() }

// fetchURLReader GETs url and wraps the body with a size cap. Non-200
// responses surface as errors carrying the status code (404s are
// distinguishable via isNotFound).
func (s *Service) fetchURLReader(url string) (*boundedBody, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		statusErr := fmt.Errorf("download returned %d", resp.StatusCode)
		if resp.StatusCode == http.StatusNotFound {
			return nil, notFoundError{statusErr}
		}
		return nil, statusErr
	}
	return &boundedBody{
		Reader: io.LimitReader(resp.Body, updateDownloadLimit),
		total:  resp.ContentLength,
		closer: resp.Body,
	}, nil
}

// fetchURL performs a bounded GET and returns the body bytes.
func (s *Service) fetchURL(url string, limit int64) ([]byte, error) {
	body, err := s.fetchURLReader(url)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(io.LimitReader(body, limit))
}

type notFoundError struct{ err error }

func (e notFoundError) Error() string { return e.err.Error() }
func (e notFoundError) Unwrap() error { return e.err }

func isNotFound(err error) bool {
	var target notFoundError
	return errors.As(err, &target)
}

// parseChecksums reads sha256sum-style lines: "<hex>  <name>".
func parseChecksums(content string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		digest := strings.ToLower(fields[0])
		name := strings.TrimPrefix(fields[1], "*")
		if len(digest) != 64 {
			continue
		}
		if _, err := hex.DecodeString(digest); err != nil {
			continue
		}
		result[name] = digest
	}
	return result
}

// verifyIntegrity enforces the published digests for one artifact.
// checksums non-nil enforces the checksums.txt entry; a signed release
// manifest (when published and a public key is configured) additionally
// requires a valid ed25519 signature over its artifact table.
func verifyIntegrity(digest, artifactName string, checksums map[string]string, storageDir, publicKeyHex, goos, goarch string, now func() time.Time) error {
	// A verified signed manifest is the strongest authority and wins over
	// checksums.txt; checksums.txt only applies when no verified manifest
	// digest is available.
	manifestDigest, sigErr := verifySignedManifest(storageDir, publicKeyHex, goos, goarch, now)
	if sigErr != nil {
		return sigErr
	}
	expected := manifestDigest
	if expected == "" && checksums != nil {
		found, ok := checksums[artifactName]
		if !ok {
			return fmt.Errorf("%w: checksums.txt has no entry for %s", ErrChecksumMismatch, artifactName)
		}
		expected = found
	}
	if expected == "" {
		log.Printf("update: no published digest for %s; continuing unverified", artifactName)
		return nil
	}
	if !strings.EqualFold(digest, expected) {
		return fmt.Errorf("%w: got %s want %s", ErrChecksumMismatch, digest, expected)
	}
	return nil
}

// verifySignedManifest reads release-manifest.json (when staged) and
// verifies it with internal/platform/updater. It returns the artifact digest
// declared by a *verified* manifest, or "" when verification was skipped
// (asset absent, or no public key configured).
func verifySignedManifest(storageDir, publicKeyHex, goos, goarch string, now func() time.Time) (string, error) {
	if publicKeyHex == "" {
		return "", nil
	}
	raw, err := os.ReadFile(filepath.Join(storageDir, signedManifestAsset))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	var manifest updater.ReleaseManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", fmt.Errorf("release manifest unreadable: %w", err)
	}
	if err := updater.VerifyManifest(manifest, publicKeyHex, 0, now()); err != nil {
		return "", fmt.Errorf("release manifest rejected: %w", err)
	}
	return manifest.Artifacts[goos+"-"+goarch], nil
}

// compatibleAsset picks the release artifact for this platform:
// LemonSSH-{version}-{goos}-{goarch}[.exe] (scripts/package-wails.mjs
// artifactBasename). A relaxed extension-less match is attempted second so a
// stamping drift cannot strand the channel.
func compatibleAsset(assets []ReleaseAsset, version, goos, goarch string) *ReleaseAsset {
	exact := fmt.Sprintf("LemonSSH-%s-%s-%s%s", version, goos, goarch, exeSuffix(goos))
	prefix := fmt.Sprintf("LemonSSH-%s-%s-%s", version, goos, goarch)
	for i := range assets {
		if assets[i].Name == exact {
			return &assets[i]
		}
	}
	for i := range assets {
		name := assets[i].Name
		if strings.HasPrefix(name, prefix) && !strings.Contains(strings.TrimPrefix(name, prefix), "-") {
			return &assets[i]
		}
	}
	return nil
}

func exeSuffix(goos string) string {
	if goos == "windows" {
		return ".exe"
	}
	return ""
}

// fileSHA256 hex-digests a file.
func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// swapExecutable replaces the running binary: copy the artifact next to it
// (same volume), rename the running image to ".old", move the copy into
// place, and restore on failure. Windows permits renaming a running image,
// which makes the swap atomic enough for self-update.
func swapExecutable(exePath, artifactPath string) error {
	if exePath == "" || artifactPath == "" {
		return ErrNoExecutable
	}
	newPath := exePath + ".new"
	oldPath := exePath + ".old"
	if err := copyFile(artifactPath, newPath); err != nil {
		return err
	}
	if err := os.Rename(exePath, oldPath); err != nil {
		_ = os.Remove(newPath)
		return fmt.Errorf("updateuse: stage running image: %w", err)
	}
	if err := os.Rename(newPath, exePath); err != nil {
		_ = os.Rename(oldPath, exePath)
		_ = os.Remove(newPath)
		return fmt.Errorf("updateuse: activate new image: %w", err)
	}
	// Best effort: the previous image may still be locked by the OS.
	_ = os.Remove(oldPath)
	return nil
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(destination)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return nil
}

// CompareVersions compares dotted numeric versions ("1.2.3", "v1.2",
// "0.0.0-dev"). Non-numeric segments parse by their leading digits.
// Returns 1 when a > b, -1 when a < b and 0 when equal.
func CompareVersions(a, b string) int {
	partsA := parseVersion(a)
	partsB := parseVersion(b)
	for i := 0; i < len(partsA) || i < len(partsB); i++ {
		var numA, numB int
		if i < len(partsA) {
			numA = partsA[i]
		}
		if i < len(partsB) {
			numB = partsB[i]
		}
		if numA > numB {
			return 1
		}
		if numA < numB {
			return -1
		}
	}
	return 0
}

// IsDevVersion reports whether a version cannot participate in update
// checks: empty or every numeric segment zero ("0.0.0", "0.0.0-dev",
// "0.0.0-wails-skeleton").
func IsDevVersion(version string) bool {
	if strings.TrimSpace(version) == "" {
		return true
	}
	for _, part := range parseVersion(version) {
		if part != 0 {
			return false
		}
	}
	return true
}

func parseVersion(version string) []int {
	clean := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(version), "v"), "V")
	// Pre-release / build metadata never makes a version "newer": a stable
	// install must not be re-offered the same version's -rc.1.
	if cut := strings.IndexAny(clean, "-+"); cut >= 0 {
		clean = clean[:cut]
	}
	segments := strings.Split(clean, ".")
	parts := make([]int, 0, len(segments))
	for _, segment := range segments {
		if segment == "" {
			parts = append(parts, 0)
			continue
		}
		num, err := strconv.Atoi(segment)
		if err != nil {
			// "0-wails-skeleton" and friends: leading digits still count.
			digits := 0
			for digits < len(segment) && segment[digits] >= '0' && segment[digits] <= '9' {
				digits++
			}
			if digits == 0 {
				parts = append(parts, 0)
				continue
			}
			num, _ = strconv.Atoi(segment[:digits])
		}
		parts = append(parts, num)
	}
	return parts
}
