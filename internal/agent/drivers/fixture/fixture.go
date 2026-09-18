// Package fixture provides a deterministic, provider-free turn driver for
// development and test runs of the W12 minimal chain. It is NOT a product
// provider: the composition root may wire it only behind an explicit dev
// flag, and release builds must fail with UNAVAILABLE instead of silently
// answering with fixture output.
package fixture

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/binaricat/netcatty/internal/agent/runtime"
)

// Driver echoes a deterministic reasoning-free turn: one user-echo text
// event, one tool-call-free finish. Events go through the runtime-owned
// session sink so sequencing stays with the TurnManager.
type Driver struct {
	// Texts are emitted in order; empty means the single default line.
	Texts []string
}

// New builds the default fixture driver.
func New() *Driver {
	return &Driver{Texts: []string{"[fixture] turn accepted"}}
}

// Stream implements runtime.TurnDriver.
func (d *Driver) Stream(ctx context.Context, session *runtime.DriverSession) error {
	texts := d.Texts
	if len(texts) == 0 {
		texts = []string{"[fixture] turn accepted"}
	}
	for _, text := range texts {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		payload, err := json.Marshal(map[string]string{"text": text})
		if err != nil {
			return fmt.Errorf("fixture: %w", err)
		}
		if err := session.Emit("text_delta", payload); err != nil {
			return err
		}
	}
	return nil
}
