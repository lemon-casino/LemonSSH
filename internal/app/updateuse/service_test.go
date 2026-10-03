package updateuse

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/binaricat/lemonssh/internal/platform/updater"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.2.3", "1.2.2", 1},
		{"v1.2.3", "1.2.10", -1},
		{"0.0.1", "0.0.0", 1},
		{"0.0.0-dev", "0.0.0", 0},
		{"2.0", "1.9.9", 1},
		{"1.0.0-rc.1", "1.0.0", 0},
	}
	for _, tc := range cases {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestIsDevVersion(t *testing.T) {
	for _, version := range []string{"", "  ", "0.0.0", "0.0.0-dev", "0.0.0-wails-skeleton", "v0"} {
		if !IsDevVersion(version) {
			t.Fatalf("IsDevVersion(%q) = false, want true", version)
		}
	}
	for _, version := range []string{"0.0.1", "1.2.3", "0.1.0"} {
		if IsDevVersion(version) {
			t.Fatalf("IsDevVersion(%q) = true, want false", version)
		}
	}
}

func TestCompatibleAsset(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "checksums.txt"},
		{Name: "LemonSSH-1.2.3-windows-amd64.exe"},
		{Name: "LemonSSH-1.2.3-linux-arm64"},
	}
	if got := compatibleAsset(assets, "1.2.3", "windows", "amd64"); got == nil || got.Name != "LemonSSH-1.2.3-windows-amd64.exe" {
		t.Fatalf("exact match failed: %+v", got)
	}
	if got := compatibleAsset(assets, "1.2.3", "linux", "arm64"); got == nil || got.Name != "LemonSSH-1.2.3-linux-arm64" {
		t.Fatalf("linux match failed: %+v", got)
	}
	if got := compatibleAsset(assets, "1.2.3", "darwin", "arm64"); got != nil {
		t.Fatalf("missing platform must not match: %+v", got)
	}
}

type fixture struct {
	server      *httptest.Server
	service     *Service
	storageDir  string
	events      []string
	releases    int
	releasesHit int
	mu          sync.Mutex
}

// newFixture spins a fake Releases API on top of files under dir:
// releases.json is the API payload; other files are served by name as
// download assets.
func newFixture(t *testing.T, dir, currentVersion string, mutate func(*Options)) *fixture {
	t.Helper()
	f := &fixture{storageDir: filepath.Join(dir, "updates")}
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.releasesHit++
		f.mu.Unlock()
		raw, err := os.ReadFile(filepath.Join(dir, "releases.json"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	})
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		_, _ = w.Write(raw)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)

	opts := Options{
		CurrentVersion: currentVersion,
		ReleasesURL:    f.server.URL + "/releases/latest",
		StorageDir:     f.storageDir,
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		Emit: func(name string, _ any) {
			f.mu.Lock()
			f.events = append(f.events, name)
			f.mu.Unlock()
		},
	}
	if mutate != nil {
		mutate(&opts)
	}
	f.service = New(opts)
	return f
}

func (f *fixture) eventCount(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	count := 0
	for _, event := range f.events {
		if event == name {
			count++
		}
	}
	return count
}

func TestCheckDetectsNewerRelease(t *testing.T) {
	dir := t.TempDir()
	artifact := "LemonSSH-1.2.3-" + runtime.GOOS + "-" + runtime.GOARCH + exeSuffix(runtime.GOOS)
	body := []byte("new binary payload")
	if err := os.WriteFile(filepath.Join(dir, artifact), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(fmt.Sprintf("%s  %s\n", sha256Hex(body), artifact)), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, dir, "1.0.0", nil)
	writeReleaseWithServer(t, dir, "1.2.3", f.server.URL)

	result := f.service.Check()
	if !result.Available || !result.Supported {
		t.Fatalf("Check() = %+v, want available+supported", result)
	}
	if result.Version != "1.2.3" || result.ReleaseNotes != "release notes for 1.2.3" {
		t.Fatalf("Check() payload wrong: %+v", result)
	}
	if got := f.eventCount(EventAvailable); got != 1 {
		t.Fatalf("available events = %d, want 1", got)
	}
	snapshot := f.service.StatusSnapshot()
	if snapshot.Status != StatusAvailable || snapshot.Version != "1.2.3" {
		t.Fatalf("snapshot = %+v, want available@1.2.3", snapshot)
	}
}

func TestCheckWithoutCompatibleArtifactReportsUnsupported(t *testing.T) {
	dir := t.TempDir()
	f := newFixture(t, dir, "1.0.0", func(o *Options) {
		o.GOOS = "plan9" // no artifact published for this platform
		o.GOARCH = "amd64"
	})
	writeReleaseWithServer(t, dir, "1.2.3", f.server.URL)

	result := f.service.Check()
	if result.Available || result.Supported {
		t.Fatalf("Check() = %+v, want unavailable+unsupported so the shell opens the Releases page", result)
	}
	if got := f.eventCount(EventAvailable); got != 0 {
		t.Fatalf("available events = %d, want 0", got)
	}
}

func TestCheckSameVersionReportsNotAvailable(t *testing.T) {
	dir := t.TempDir()
	f := newFixture(t, dir, "1.2.3", nil)
	writeReleaseWithServer(t, dir, "1.2.3", f.server.URL)

	result := f.service.Check()
	if result.Available {
		t.Fatalf("Check() = %+v, want not available", result)
	}
	if got := f.eventCount(EventNotAvailable); got != 1 {
		t.Fatalf("not-available events = %d, want 1", got)
	}
}

func TestCheckDevVersionSkipsNetwork(t *testing.T) {
	dir := t.TempDir()
	f := newFixture(t, dir, "0.0.0", nil)
	writeReleaseWithServer(t, dir, "9.9.9", f.server.URL)

	result := f.service.Check()
	if result.Available || result.Supported {
		t.Fatalf("Check() = %+v, want unsupported for a dev build", result)
	}
	f.mu.Lock()
	hits := f.releasesHit
	f.mu.Unlock()
	if hits != 0 {
		t.Fatalf("releases API hit %d times for a dev build, want 0", hits)
	}
}

func TestDownloadVerifiesChecksumAndReportsProgress(t *testing.T) {
	dir := t.TempDir()
	artifact := "LemonSSH-1.2.3-" + runtime.GOOS + "-" + runtime.GOARCH + exeSuffix(runtime.GOOS)
	body := []byte("verified binary payload")
	if err := os.WriteFile(filepath.Join(dir, artifact), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(fmt.Sprintf("%s  %s\n", sha256Hex(body), artifact)), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, dir, "1.0.0", nil)
	writeReleaseWithServer(t, dir, "1.2.3", f.server.URL)
	f.service.Check()

	if result := f.service.Download(); !result.Success {
		t.Fatalf("Download() = %+v, want success", result)
	}
	downloaded, err := os.ReadFile(filepath.Join(f.storageDir, artifact))
	if err != nil {
		t.Fatal(err)
	}
	if string(downloaded) != string(body) {
		t.Fatalf("downloaded bytes differ")
	}
	if got := f.eventCount(EventDownloadProgress); got == 0 {
		t.Fatal("no progress events emitted")
	}
	if got := f.eventCount(EventDownloaded); got != 1 {
		t.Fatalf("downloaded events = %d, want 1", got)
	}
	if snapshot := f.service.StatusSnapshot(); snapshot.Status != StatusReady || snapshot.Percent != 100 {
		t.Fatalf("snapshot = %+v, want ready@100", snapshot)
	}
	if _, err := os.Stat(filepath.Join(f.storageDir, artifact+".part")); !os.IsNotExist(err) {
		t.Fatalf("staging file must be gone, err=%v", err)
	}
}

func TestDownloadRejectsTamperedArtifact(t *testing.T) {
	dir := t.TempDir()
	artifact := "LemonSSH-1.2.3-" + runtime.GOOS + "-" + runtime.GOARCH + exeSuffix(runtime.GOOS)
	body := []byte("genuine payload")
	if err := os.WriteFile(filepath.Join(dir, artifact), body, 0o644); err != nil {
		t.Fatal(err)
	}
	// checksums describe different bytes than the served artifact
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(fmt.Sprintf("%s  %s\n", sha256Hex([]byte("tampered payload")), artifact)), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, dir, "1.0.0", nil)
	writeReleaseWithServer(t, dir, "1.2.3", f.server.URL)
	f.service.Check()

	result := f.service.Download()
	if result.Success {
		t.Fatal("Download() succeeded with a wrong checksum")
	}
	if snapshot := f.service.StatusSnapshot(); snapshot.Status != StatusError {
		t.Fatalf("snapshot = %+v, want error", snapshot)
	}
	if got := f.eventCount(EventError); got == 0 {
		t.Fatal("no error event emitted")
	}
	if _, err := os.Stat(filepath.Join(f.storageDir, artifact+".part")); !os.IsNotExist(err) {
		t.Fatalf("staging file must be removed on failure, err=%v", err)
	}
}

func TestDownloadWithoutCheckFails(t *testing.T) {
	dir := t.TempDir()
	f := newFixture(t, dir, "1.0.0", nil)
	result := f.service.Download()
	if result.Success || result.Error == "" {
		t.Fatalf("Download() = %+v, want a typed failure", result)
	}
}

func TestAutoDownloadRunsWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	artifact := "LemonSSH-1.2.3-" + runtime.GOOS + "-" + runtime.GOARCH + exeSuffix(runtime.GOOS)
	body := []byte("auto download payload")
	if err := os.WriteFile(filepath.Join(dir, artifact), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(fmt.Sprintf("%s  %s\n", sha256Hex(body), artifact)), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, dir, "1.0.0", func(o *Options) { o.AutoUpdate = true })
	writeReleaseWithServer(t, dir, "1.2.3", f.server.URL)

	f.service.Check()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if f.service.StatusSnapshot().Status == StatusReady {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("auto-download never reached ready: %+v", f.service.StatusSnapshot())
}

func TestCheckWhileInFlightReportsChecking(t *testing.T) {
	dir := t.TempDir()
	releaseGate := make(chan struct{})
	gated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-releaseGate
		raw, _ := os.ReadFile(filepath.Join(dir, "releases.json"))
		_, _ = w.Write(raw)
	}))
	defer gated.Close()
	// A compatible artifact must exist, otherwise the check legitimately
	// reports unsupported instead of available.
	artifact := "LemonSSH-1.2.3-" + runtime.GOOS + "-" + runtime.GOARCH + exeSuffix(runtime.GOOS)
	if err := os.WriteFile(filepath.Join(dir, artifact), []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeReleaseWithServer(t, dir, "1.2.3", gated.URL)
	// Point the service at the gated handler so the first Check blocks.
	f := newFixture(t, dir, "1.0.0", func(o *Options) {
		o.ReleasesURL = gated.URL + "/releases/latest"
	})

	done := make(chan CheckResult, 1)
	go func() { done <- f.service.Check() }()
	time.Sleep(50 * time.Millisecond)
	inFlight := f.service.Check()
	if !inFlight.Checking {
		t.Fatalf("concurrent Check() = %+v, want checking", inFlight)
	}
	close(releaseGate)
	result := <-done
	if !result.Available {
		t.Fatalf("first Check() = %+v, want available", result)
	}
}

func TestInstallSwapsBinaryRelaunchesAndQuits(t *testing.T) {
	dir := t.TempDir()
	artifact := "LemonSSH-1.2.3-" + runtime.GOOS + "-" + runtime.GOARCH + exeSuffix(runtime.GOOS)
	body := []byte("installed binary")
	if err := os.WriteFile(filepath.Join(dir, artifact), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(fmt.Sprintf("%s  %s\n", sha256Hex(body), artifact)), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newFixture(t, dir, "1.0.0", nil)
	writeReleaseWithServer(t, dir, "1.2.3", f.server.URL)
	f.service.Check()
	if result := f.service.Download(); !result.Success {
		t.Fatalf("Download() = %+v", result)
	}

	exeDir := t.TempDir()
	exePath := filepath.Join(exeDir, "LemonSSH")
	if err := os.WriteFile(exePath, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Never touch the real running binary in tests.
	f.service.opts.Executable = exePath
	var launches [][]string
	var quitCount int
	var mu sync.Mutex
	f.service.opts.StartDetached = func(name string, argv []string) error {
		mu.Lock()
		defer mu.Unlock()
		launches = append(launches, append([]string{name}, argv...))
		return nil
	}
	f.service.opts.Quit = func() {
		mu.Lock()
		defer mu.Unlock()
		quitCount++
	}

	if err := f.service.Install(); err != nil {
		t.Fatalf("Install() = %v", err)
	}
	// The relaunch helper must already be spawned before Install returns:
	// the process is about to quit, so nothing scheduled afterwards would
	// ever run.
	mu.Lock()
	launchCount, quitsNow := len(launches), quitCount
	mu.Unlock()
	if launchCount != 1 || quitsNow != 1 {
		t.Fatalf("Install() spawns relaunch+quit synchronously; launches=%d quits=%d", launchCount, quitsNow)
	}
	installed, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(body) {
		t.Fatalf("installed binary = %q, want %q", installed, body)
	}
	// strconv.Quote doubles the backslashes on Windows; normalise before
	// matching the executable path.
	argv := strings.ReplaceAll(strings.Join(launches[0], " "), `\\`, `\`)
	if !strings.Contains(argv, exePath) {
		t.Fatalf("relaunch argv %v does not reference %s", launches[0], exePath)
	}
}

func TestInstallWithoutReadyArtifactFails(t *testing.T) {
	dir := t.TempDir()
	f := newFixture(t, dir, "1.0.0", nil)
	if err := f.service.Install(); err == nil {
		t.Fatal("Install() without a ready artifact must fail")
	}
}

func TestAutoCheckThrottle(t *testing.T) {
	dir := t.TempDir()
	now := time.UnixMilli(1_000_000)
	var persisted int64
	f := newFixture(t, dir, "1.0.0", func(o *Options) {
		o.Now = func() time.Time { return now }
		o.PersistedLastCheck = func() int64 { return persisted }
		o.PersistLastCheck = func(at int64) error { persisted = at; return nil }
	})
	writeReleaseWithServer(t, dir, "1.2.3", f.server.URL)

	f.service.AutoCheck()
	f.mu.Lock()
	hits := f.releasesHit
	f.mu.Unlock()
	if hits != 1 {
		t.Fatalf("first AutoCheck hit releases %d times, want 1", hits)
	}
	if persisted != now.UnixMilli() {
		t.Fatalf("last-check stamp = %d, want %d", persisted, now.UnixMilli())
	}

	// A second run within the hour must skip the network entirely.
	f.service.AutoCheck()
	f.mu.Lock()
	hits = f.releasesHit
	f.mu.Unlock()
	if hits != 1 {
		t.Fatalf("throttled AutoCheck hit releases %d times, want 1", hits)
	}
}

func TestAutoCheckSkipsFailedChecksForThrottle(t *testing.T) {
	dir := t.TempDir()
	f := newFixture(t, dir, "1.0.0", nil)
	// No releases.json on disk → the handler 500s → Check errors.
	f.service.opts.ReleasesURL = f.server.URL + "/releases/latest"
	var persisted int64
	f.service.opts.PersistedLastCheck = func() int64 { return persisted }
	f.service.opts.PersistLastCheck = func(at int64) error { persisted = at; return nil }

	f.service.AutoCheck()
	if persisted != 0 {
		t.Fatalf("failed check must not persist a stamp, got %d", persisted)
	}
}

func TestSignedManifestVerification(t *testing.T) {
	dir := t.TempDir()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	version := "1.2.3"
	artifact := "LemonSSH-" + version + "-" + runtime.GOOS + "-" + runtime.GOARCH + exeSuffix(runtime.GOOS)
	body := []byte("signed payload")
	digest := sha256Hex(body)
	if err := os.WriteFile(filepath.Join(dir, artifact), body, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := updater.ReleaseManifest{
		Version:     version,
		PublishedMS: time.Now().UnixMilli(),
		Artifacts:   map[string]string{runtime.GOOS + "-" + runtime.GOARCH: digest},
	}
	signature, err := updater.SignManifest(manifest, hex.EncodeToString(privateKey))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Signature = signature
	signed, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, signedManifestAsset), signed, 0o644); err != nil {
		t.Fatal(err)
	}
	// checksums.txt deliberately omits the artifact so only the signed
	// manifest can vouch for it.
	if err := os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	f := newFixture(t, dir, "1.0.0", func(o *Options) {
		o.PublicKeyHex = hex.EncodeToString(publicKey)
	})
	writeReleaseWithServer(t, dir, version, f.server.URL)
	f.service.Check()
	if result := f.service.Download(); !result.Success {
		t.Fatalf("Download() = %+v, want success via signed manifest", result)
	}

	// A wrong public key must reject the manifest and fail the download.
	f2 := newFixture(t, dir, "1.0.0", func(o *Options) {
		otherKey, _, _ := ed25519.GenerateKey(rand.Reader)
		o.PublicKeyHex = hex.EncodeToString(otherKey)
	})
	writeReleaseWithServer(t, dir, version, f2.server.URL)
	f2.service.Check()
	if result := f2.service.Download(); result.Success {
		t.Fatal("Download() accepted a manifest signed by the wrong key")
	}
}

// --- helpers ---

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// writeReleaseWithServer writes the fake API payload pointing every asset at
// the fixture's /download/ handler on the live test server.
func writeReleaseWithServer(t *testing.T, dir, version, serverURL string) {
	t.Helper()
	release := Release{
		TagName:     "v" + version,
		Name:        "LemonSSH " + version,
		Body:        "release notes for " + version,
		PublishedAt: "2026-09-01T00:00:00Z",
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "releases.json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		release.Assets = append(release.Assets, ReleaseAsset{
			Name:               name,
			Size:               info.Size(),
			BrowserDownloadURL: serverURL + "/download/" + name,
		})
	}
	raw, err := json.Marshal(release)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "releases.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}
