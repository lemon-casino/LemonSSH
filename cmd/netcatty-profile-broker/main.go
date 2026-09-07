// Command netcatty-profile-broker is the minimal stdio lease broker that lets
// the Electron shell and the Wails shell share one writer-lease authority
// (P2-03). One JSON request per line on stdin; one JSON response per line on
// stdout. Requests: {"op":"acquire","holder":"...","ttlMs":30000},
// {"op":"renew","ttlMs":...}, {"op":"release"}, {"op":"status"}.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/binaricat/netcatty/internal/profile/coordination"
)

type request struct {
	Op     string `json:"op"`
	Holder string `json:"holder"`
	TTLMS  int64  `json:"ttlMs"`
}

type response struct {
	OK    bool                `json:"ok"`
	Lease *coordination.Lease `json:"lease,omitempty"`
	Error string              `json:"error,omitempty"`
}

func main() {
	profilePath := os.Getenv("NETCATTY_PROFILE_PATH")
	if profilePath == "" {
		writeLine(response{OK: false, Error: "NETCATTY_PROFILE_PATH is required"})
		os.Exit(2)
	}
	if _, err := os.Stat(profilePath); err != nil {
		_ = os.MkdirAll(filepath.Dir(profilePath), 0o700)
	}

	scanner := bufio.NewScanner(os.Stdin)
	var manager *coordination.Manager
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			writeLine(response{OK: false, Error: fmt.Sprintf("bad request: %v", err)})
			continue
		}
		switch req.Op {
		case "acquire":
			manager = coordination.NewManager(profilePath, req.Holder)
			lease, err := manager.Acquire(time.Duration(req.TTLMS) * time.Millisecond)
			reply(lease, err)
		case "renew":
			if manager == nil {
				writeLine(response{OK: false, Error: "not acquired"})
				continue
			}
			lease, err := manager.Renew(time.Duration(req.TTLMS) * time.Millisecond)
			reply(lease, err)
		case "release":
			if manager == nil {
				writeLine(response{OK: false, Error: "not acquired"})
				continue
			}
			err := manager.Release()
			writeLine(response{OK: err == nil, Error: errorText(err)})
		case "status":
			if manager == nil {
				manager = coordination.NewManager(profilePath, req.Holder)
			}
			lease, ok := manager.Current()
			if !ok {
				writeLine(response{OK: true})
				continue
			}
			writeLine(response{OK: true, Lease: &lease})
		default:
			writeLine(response{OK: false, Error: "unknown op"})
		}
	}
}

func reply(lease coordination.Lease, err error) {
	if err != nil {
		writeLine(response{OK: false, Error: err.Error()})
		return
	}
	writeLine(response{OK: true, Lease: &lease})
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func writeLine(value response) {
	data, _ := json.Marshal(value)
	fmt.Println(string(data))
}
