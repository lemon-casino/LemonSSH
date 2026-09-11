// Command updatefeed produces the ed25519-signed update feed (P6-03, REL-02)
// consumed by the in-app updater: a keygen subcommand for self-managed keys
// and a sign subcommand that hashes release artifacts and writes latest.json
// with the signature computed over the canonical manifest body.
//
// Usage:
//
//	go run ./cmd/updatefeed keygen
//	go run ./cmd/updatefeed sign -key <private-hex> -version 0.0.2 \
//	    -notes "release notes" -out dist/wails/latest.json dist/wails/LemonSSH-0.0.2-*.exe
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/binaricat/netcatty/internal/platform/updater"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "keygen":
		if err := runKeygen(); err != nil {
			fail(err)
		}
	case "sign":
		if err := runSign(os.Args[2:]); err != nil {
			fail(err)
		}
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: updatefeed keygen | updatefeed sign -key <hex> -version <v> [-notes n] -out latest.json <artifact>...")
	os.Exit(2)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "updatefeed:", err)
	os.Exit(1)
}

func runKeygen() error {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	fmt.Printf("publicKeyHex=%s\nprivateKeyHex=%s\n",
		hex.EncodeToString(publicKey), hex.EncodeToString(privateKey))
	fmt.Fprintln(os.Stderr, "keep the private key offline; the public key ships with the app")
	return nil
}

func runSign(args []string) error {
	flags := flag.NewFlagSet("sign", flag.ContinueOnError)
	privateKeyHex := flags.String("key", "", "ed25519 private key (hex)")
	version := flags.String("version", "", "release version, e.g. 0.0.2")
	notes := flags.String("notes", "", "release notes stored in the feed")
	out := flags.String("out", "latest.json", "output feed path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	artifacts := flags.Args()
	if *privateKeyHex == "" || *version == "" || *out == "" || len(artifacts) == 0 {
		return errors.New("sign requires -key, -version, -out and at least one artifact")
	}
	privateKey, err := hex.DecodeString(strings.TrimSpace(*privateKeyHex))
	if err != nil {
		return fmt.Errorf("decode private key: %w", err)
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("private key must be %d bytes, got %d", ed25519.PrivateKeySize, len(privateKey))
	}

	platformMap, err := artifactMap(artifacts)
	if err != nil {
		return err
	}
	manifest := updater.ReleaseManifest{
		Version:     *version,
		PublishedMS: time.Now().UnixMilli(),
		Artifacts:   platformMap,
		Notes:       *notes,
	}
	signature, err := updater.SignManifest(manifest, hex.EncodeToString(privateKey))
	if err != nil {
		return fmt.Errorf("sign manifest: %w", err)
	}
	manifest.Signature = signature
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(*out, append(encoded, '\n'), 0o600); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d artifacts)\n", *out, len(platformMap))
	return nil
}

// artifactMap hashes every artifact and infers its platform-arch key from the
// file name (GOOS tokens windows/darwin/linux plus amd64/arm64/x64/arm64).
func artifactMap(paths []string) (map[string]string, error) {
	platforms := make(map[string]string)
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("%s is a directory", path)
		}
		platform, err := platformFromName(filepath.Base(path))
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		sum := sha256.Sum256(data)
		platforms[platform] = hex.EncodeToString(sum[:])
	}
	return platforms, nil
}

func platformFromName(name string) (string, error) {
	lower := strings.ToLower(name)
	goos := ""
	for _, candidate := range []string{"windows", "darwin", "linux"} {
		if strings.Contains(lower, candidate) {
			goos = candidate
			break
		}
	}
	goarch := ""
	for _, candidate := range []string{"amd64", "x64", "arm64", "arm"} {
		if strings.Contains(lower, candidate) {
			goarch = candidate
			break
		}
	}
	if goarch == "x64" {
		goarch = "amd64"
	}
	if goos == "" || goarch == "" {
		return "", fmt.Errorf("%s: cannot infer platform-arch from file name", name)
	}
	keys := []string{goos + "-" + goarch, goos + "_" + goarch}
	sort.Strings(keys)
	return keys[0], nil
}
