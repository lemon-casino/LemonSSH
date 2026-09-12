// Package manifest defines the plugin manifest v2 contract (P5-01): WASM
// entrypoints, optional native variants, declared permissions and
// contributions. Validation is the single authority for what may be installed;
// v1 JavaScript/Node entrypoints are rejected outright (WV3-005).
package manifest

import (
	"errors"
	"fmt"
	"github.com/binaricat/netcatty/internal/plugin/ui"
	"regexp"
	"strings"
)

const Version = 2

var (
	ErrInvalid        = errors.New("invalid plugin manifest")
	namePattern       = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)
	versionPattern    = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)
	identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
)

type Manifest struct {
	UI             *ui.Schema     `json:"ui,omitempty"`
	APIVersion     int            `json:"apiVersion"`
	Name           string         `json:"name"`
	Version        string         `json:"version"`
	DisplayName    string         `json:"displayName"`
	Description    string         `json:"description,omitempty"`
	Entrypoint     Entrypoint     `json:"entrypoint"`
	Permissions    []Permission   `json:"permissions,omitempty"`
	Contributions  []Contribution `json:"contributions,omitempty"`
	MinHostVersion string         `json:"minHostVersion,omitempty"`
}

// Entrypoint v2 is WASM-only. v1 fields (main.browser / main.node) have no
// representation here: their presence fails validation.
type Entrypoint struct {
	WASM     string `json:"wasm"`
	SHA256   string `json:"sha256"`
	MemoryMB int    `json:"memoryMB,omitempty"`
}

type Permission struct {
	Kind     string `json:"kind"`     // e.g. "filesystem", "network", "terminal", "secret"
	Resource string `json:"resource"` // canonical resource string, kind-scoped
	Mode     string `json:"mode"`     // "read" | "write"
}

type Contribution struct {
	Type string `json:"type"` // "command" | "setting" | "view"
	ID   string `json:"id"`
}

func Validate(m Manifest) error {
	if m.UI != nil {
		if err := ui.Validate(*m.UI); err != nil {
			return err
		}
	}
	if m.APIVersion != Version {
		return fmt.Errorf("%w: apiVersion %d, want %d", ErrInvalid, m.APIVersion, Version)
	}
	if !namePattern.MatchString(m.Name) {
		return fmt.Errorf("%w: name %q", ErrInvalid, m.Name)
	}
	if !versionPattern.MatchString(m.Version) {
		return fmt.Errorf("%w: version %q", ErrInvalid, m.Version)
	}
	if strings.TrimSpace(m.DisplayName) == "" {
		return fmt.Errorf("%w: displayName empty", ErrInvalid)
	}
	if !strings.HasSuffix(m.Entrypoint.WASM, ".wasm") || strings.Contains(m.Entrypoint.WASM, "..") {
		return fmt.Errorf("%w: entrypoint wasm %q", ErrInvalid, m.Entrypoint.WASM)
	}
	if len(m.Entrypoint.SHA256) != 64 {
		return fmt.Errorf("%w: entrypoint sha256 must be 64 hex chars", ErrInvalid)
	}
	if m.Entrypoint.MemoryMB != 0 && (m.Entrypoint.MemoryMB < 16 || m.Entrypoint.MemoryMB > 512) {
		return fmt.Errorf("%w: memoryMB %d out of 16..512", ErrInvalid, m.Entrypoint.MemoryMB)
	}
	if err := validatePermissions(m.Permissions); err != nil {
		return err
	}
	if err := validateContributions(m.Contributions); err != nil {
		return err
	}
	return nil
}

var validPermissionKinds = map[string]bool{
	"filesystem": true, "network": true, "terminal": true, "secret": true, "clipboard": true,
}

func validatePermissions(perms []Permission) error {
	seen := make(map[string]bool, len(perms))
	for _, perm := range perms {
		if !validPermissionKinds[perm.Kind] {
			return fmt.Errorf("%w: unknown permission kind %q", ErrInvalid, perm.Kind)
		}
		if strings.TrimSpace(perm.Resource) == "" {
			return fmt.Errorf("%w: permission %q has empty resource", ErrInvalid, perm.Kind)
		}
		if perm.Mode != "read" && perm.Mode != "write" {
			return fmt.Errorf("%w: permission %q mode %q", ErrInvalid, perm.Kind, perm.Mode)
		}
		key := perm.Kind + "|" + perm.Resource + "|" + perm.Mode
		if seen[key] {
			return fmt.Errorf("%w: duplicate permission %s", ErrInvalid, key)
		}
		seen[key] = true
	}
	return nil
}

func validateContributions(contribs []Contribution) error {
	seen := make(map[string]bool, len(contribs))
	for _, contribution := range contribs {
		switch contribution.Type {
		case "command", "setting", "view":
		default:
			return fmt.Errorf("%w: unknown contribution type %q", ErrInvalid, contribution.Type)
		}
		if !identifierPattern.MatchString(contribution.ID) {
			return fmt.Errorf("%w: contribution id %q", ErrInvalid, contribution.ID)
		}
		key := contribution.Type + "/" + contribution.ID
		if seen[key] {
			return fmt.Errorf("%w: duplicate contribution %s", ErrInvalid, key)
		}
		seen[key] = true
	}
	return nil
}

// RejectsV1Entrypoints documents the WV3-005 boundary: the v1 fields are not
// even representable in v2 manifests, so a v1 package cannot be upgraded by
// adding fields — it must be rebuilt for WASM.
const RejectsV1Entrypoints = "main.browser and main.node are not part of manifest v2 and cannot be added"
