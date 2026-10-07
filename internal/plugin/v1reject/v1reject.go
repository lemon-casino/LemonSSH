// Package v1reject implements the P5-08 v1 plugin rejection boundary: v1
// packages (main.browser/main.node) are structurally rejected at install time
// with a clear incompatibility result. No shim, no adapter, no fallback.
package v1reject

import (
	"encoding/json"
	"errors"
	"fmt"
)

var ErrV1NotSupported = errors.New("plugin v1 packages are not supported by this runtime")

// DetectV1 checks a raw manifest JSON for v1-only entrypoint fields. Returns
// (true, nil) when the manifest is v1 (must be rejected), (false, nil) when
// it is not, or an error on malformed JSON.
func DetectV1(manifestJSON []byte) (bool, error) {
	var probe struct {
		Main *json.RawMessage `json:"main"`
	}
	if err := json.Unmarshal(manifestJSON, &probe); err != nil {
		return false, fmt.Errorf("%w: %v", ErrV1NotSupported, err)
	}
	return probe.Main != nil, nil
}

// Reject produces the user-facing incompatibility message for a v1 package.
func Reject(pluginID string) error {
	return fmt.Errorf("%w: plugin %q uses the legacy JavaScript/Node runtime "+
		"which is not supported. Rebuild the plugin for WASM (manifest v2)", ErrV1NotSupported, pluginID)
}
