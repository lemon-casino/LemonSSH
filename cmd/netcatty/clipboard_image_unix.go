//go:build !windows

package main

import (
	"context"
	"os"
	"os/exec"
	"runtime"
)

func readNativeClipboardPNG(ctx context.Context) ([]byte, error) {
	if runtime.GOOS == "darwin" {
		return boundedClipboardOutput(exec.CommandContext(ctx, "osascript", "-l", "JavaScript", "-e", `ObjC.import('AppKit'); ObjC.import('Foundation'); var p=$.NSPasteboard.generalPasteboard; var d=p.dataForType('public.png'); if (!d) { var t=p.dataForType('public.tiff'); if(t) { var b=$.NSBitmapImageRep.imageRepWithData(t); if(b.pixelsWide*b.pixelsHigh > 16777216) throw Error('Image too large'); d=b.representationUsingTypeProperties($.NSPNGFileType,$({})); } } if(d) { $.NSFileHandle.fileHandleWithStandardOutput.writeData(d); }`), maxClipboardPNGBytes)
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return boundedClipboardOutput(exec.CommandContext(ctx, "wl-paste", "--no-newline", "--type", "image/png"), maxClipboardPNGBytes)
	}
	return boundedClipboardOutput(exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-t", "image/png", "-o"), maxClipboardPNGBytes)
}
