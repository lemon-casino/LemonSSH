package pty

import "fmt"

type ExitError struct{ Code int }

func (e ExitError) Error() string { return fmt.Sprintf("process exited with code %d", e.Code) }
func (e ExitError) ExitCode() int { return e.Code }
