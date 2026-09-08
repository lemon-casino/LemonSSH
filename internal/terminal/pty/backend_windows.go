//go:build windows

package pty

import "context"

type conptyBackend struct{}

// NewPlatformBackend is deliberately explicit until the native ConPTY adapter
// is added in the next P3-02 child slice. We do not silently fall back to a
// pipes-only process because that would violate terminal resize/interrupt
// semantics.
func NewPlatformBackend() Backend { return &conptyBackend{} }

func (*conptyBackend) Start(context.Context, Config) (Process, error) {
	return nil, ErrUnsupported
}
