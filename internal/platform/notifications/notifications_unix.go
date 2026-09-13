//go:build !windows

package notifications

import (
	"context"
	"os/exec"
	"runtime"
	"time"
)

func Show(title, body string) error {
	title, body = sanitize(title, body)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if runtime.GOOS == "darwin" {
		return exec.CommandContext(ctx, "osascript", "-e", "on run argv\ndisplay notification (item 2 of argv) with title (item 1 of argv)\nend run", "--", title, body).Run()
	}
	return exec.CommandContext(ctx, "notify-send", "--app-name=Netcatty", "--", title, body).Run()
}
