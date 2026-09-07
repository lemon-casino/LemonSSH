package main

import (
	"fmt"
	"os"
	"time"

	probe "netcatty.local/profile-secret-migration"
)

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "probe failed: invalid arguments")
		os.Exit(2)
	}
	_, err := probe.Run(os.Stdin, os.Stdout, probe.NewPlatformProvider(), 30*time.Second)
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe failed")
		os.Exit(1)
	}
}
