package main

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
)

func configureFileOpenProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

func openSystemFile(filePath string) error {
	file, err := windows.UTF16PtrFromString(filePath)
	if err != nil {
		return err
	}
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}
