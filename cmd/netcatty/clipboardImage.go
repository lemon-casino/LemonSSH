package main

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/binaricat/netcatty/internal/platform/filesystem"
)

const maxClipboardPNGBytes = 32 << 20
const maxClipboardPixels = 16 << 20

type ClipboardImageFile struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
}

// ReadClipboardImage uses the OS because Wails Clipboard only exposes text.
func (s *FilesystemService) ReadClipboardImage() (*ClipboardImageFile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return storeClipboardImage(ctx, s.temp, readNativeClipboardPNG)
}

func storeClipboardImage(ctx context.Context, temp *filesystem.TempService, read func(context.Context) ([]byte, error)) (*ClipboardImageFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := read(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	if len(data) > maxClipboardPNGBytes {
		return nil, fmt.Errorf("clipboard image exceeds size limit")
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("clipboard PNG: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxClipboardPixels {
		return nil, fmt.Errorf("clipboard image exceeds pixel limit")
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if temp == nil {
		return nil, fmt.Errorf("managed temp unavailable")
	}
	target, err := temp.ReserveFilePath("clipboard.png")
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = temp.Remove(filepath.Base(target))
		}
	}()
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	err = png.Encode(f, decoded)
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	success = true
	return &ClipboardImageFile{Path: target, Name: filepath.Base(target), MediaType: "image/png", Size: info.Size()}, nil
}
