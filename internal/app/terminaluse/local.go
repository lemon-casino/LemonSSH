package terminaluse

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/binaricat/netcatty/internal/terminal/pty"
)

// LocalStartRequest is the shell-facing local PTY payload.
type LocalStartRequest struct {
	Shell     string            `json:"shell"`
	ShellArgs []string          `json:"shellArgs"`
	CWD       string            `json:"cwd"`
	Env       map[string]string `json:"env"`
	Cols      uint16            `json:"cols"`
	Rows      uint16            `json:"rows"`
}

// DiscoveredShell describes a locally available shell.
type DiscoveredShell struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Icon      string   `json:"icon"`
	IsDefault bool     `json:"isDefault"`
}

// PathValidation reports stat-derived facts about a candidate binary path.
type PathValidation struct {
	Exists       bool `json:"exists"`
	IsFile       bool `json:"isFile"`
	IsDirectory  bool `json:"isDirectory"`
	IsExecutable bool `json:"isExecutable"`
}

func localStartConfig(id string, request LocalStartRequest) pty.Config {
	keys := make([]string, 0, len(request.Env))
	for key := range request.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+request.Env[key])
	}
	return pty.BuildConfig(id, request.Shell, request.CWD, request.ShellArgs, env, request.Cols, request.Rows)
}

// StartLocal launches a local PTY and streams it on the same data plane as SSH.
func (s *Service) StartLocal(shell, cwd string, cols, rows uint16) (string, error) {
	return s.StartLocalWithOptions(LocalStartRequest{Shell: shell, CWD: cwd, Cols: cols, Rows: rows})
}

func (s *Service) StartLocalWithOptions(request LocalStartRequest) (string, error) {
	cols, rows := request.Cols, request.Rows
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	s.mu.Lock()
	s.counter++
	sessionID := fmt.Sprintf("local-%d", s.counter)
	s.mu.Unlock()

	request.Cols, request.Rows = cols, rows
	config := localStartConfig(sessionID, request)
	local := pty.NewSession(config)
	if err := local.Start(context.Background(), pty.NewPlatformBackend()); err != nil {
		return "", fmt.Errorf("local pty: %w", err)
	}
	bootstrap, err := s.controller.Open(sessionID)
	if err != nil {
		_ = local.Close()
		return "", fmt.Errorf("open route: %w", err)
	}

	s.mu.Lock()
	s.sessions[sessionID] = &terminalSession{local: local, localConfig: config, bootstrap: bootstrap}
	s.mu.Unlock()

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := local.ReadOnce(buf)
			if n > 0 {
				if !s.publishOutput(sessionID, buf[:n]) {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()
	// ConPTY output handles can remain open after the child exits. Observe the
	// process independently; EOF is never evidence of a successful exit.
	go func() {
		_ = s.closeWithStatus(sessionID, terminalWaitExit(sessionID, local.Wait()))
	}()
	return sessionID, nil
}

// DefaultShell reports the platform default shell.
func DefaultShell() string { return pty.DefaultShell("") }

// ValidatePath stats a candidate path for local shell selection.
func ValidatePath(path, kind string) PathValidation {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimLeft(strings.TrimPrefix(path, "~"), `/\`))
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return PathValidation{}
	}
	result := PathValidation{Exists: true, IsFile: info.Mode().IsRegular(), IsDirectory: info.IsDir()}
	if result.IsFile {
		result.IsExecutable = info.Mode().Perm()&0111 != 0
		if runtime.GOOS == "windows" {
			ext := strings.ToLower(filepath.Ext(path))
			result.IsExecutable = ext == ".exe" || ext == ".com" || ext == ".cmd" || ext == ".bat"
		}
	}
	return result
}

// DiscoverShells enumerates locally installed shells with the platform default first.
func DiscoverShells() []DiscoveredShell {
	defaultShell := DefaultShell()
	shells := []DiscoveredShell{}
	seen := map[string]bool{}
	add := func(command string, args []string) {
		if command == "" {
			return
		}
		resolved, err := exec.LookPath(command)
		if err != nil {
			return
		}
		key := resolved
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			return
		}
		seen[key] = true
		shells = append(shells, DiscoveredShell{ID: key, Name: strings.TrimSuffix(filepath.Base(resolved), filepath.Ext(resolved)), Command: resolved, Args: args, Icon: "terminal", IsDefault: strings.EqualFold(resolved, defaultShell)})
	}
	add(defaultShell, []string{})
	if runtime.GOOS == "windows" {
		add("pwsh.exe", []string{"-NoLogo"})
		add("powershell.exe", []string{"-NoLogo"})
		add("cmd.exe", []string{})
		add("wsl.exe", []string{})
		for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs")} {
			if root != "" {
				add(filepath.Join(root, "Git", "bin", "bash.exe"), []string{"--login", "-i"})
			}
		}
	} else {
		if data, err := os.ReadFile("/etc/shells"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "/") {
					add(line, []string{})
				}
			}
		}
		for _, command := range []string{"bash", "zsh", "fish", "sh"} {
			add(command, []string{})
		}
	}
	return shells
}
