//go:build !windows

package filesystem

import "os"

func isHidden(os.FileInfo) bool { return false }
