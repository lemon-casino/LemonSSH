package filesystem

import (
	"os"
	"syscall"
)

func isHidden(info os.FileInfo) bool {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && data.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
}
