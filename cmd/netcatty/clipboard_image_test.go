package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/binaricat/netcatty/internal/platform/filesystem"
)

func TestClipboardImageManagedPNG(t *testing.T) {
	temp, err := filesystem.NewTempService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	result, err := storeClipboardImage(context.Background(), temp, func(context.Context) ([]byte, error) { return data.Bytes(), nil })
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.MediaType != "image/png" || result.Size == 0 {
		t.Fatalf("bad result: %+v", result)
	}
	f, err := os.Open(result.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	config, err := png.DecodeConfig(f)
	if err != nil || config.Width != 2 || config.Height != 3 {
		t.Fatalf("invalid image: %+v %v", config, err)
	}
}

func TestClipboardImageAbsenceFailureAndCancellation(t *testing.T) {
	temp, _ := filesystem.NewTempService(t.TempDir())
	for _, tc := range []struct {
		name      string
		data      []byte
		err       error
		cancelled bool
		wantErr   bool
	}{
		{name: "absent"}, {name: "invalid", data: []byte("not PNG"), wantErr: true}, {name: "failure", err: errors.New("unavailable"), wantErr: true}, {name: "cancelled", cancelled: true, wantErr: true}, {name: "oversized", data: make([]byte, maxClipboardPNGBytes+1), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			result, err := storeClipboardImage(ctx, temp, func(context.Context) ([]byte, error) { return tc.data, tc.err })
			if (err != nil) != tc.wantErr || result != nil {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			entries, _ := os.ReadDir(temp.Root())
			if len(entries) != 0 {
				t.Fatal("failed read left temporary file")
			}
		})
	}
}

func TestSFTPOpenForTerminalRejectsMissingAndNonSSH(t *testing.T) {
	s := NewSFTPService(nil, nil)
	s.setTerminalService(&TerminalService{sessions: map[string]*terminalSession{"local": {}}})
	for _, id := range []string{"missing", "local"} {
		if _, err := s.OpenForTerminal(id); err == nil {
			t.Fatalf("accepted %s", id)
		}
	}
}
