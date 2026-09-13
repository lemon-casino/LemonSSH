package main

import (
	"github.com/binaricat/netcatty/internal/terminal/pty"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type LocalStartRequest struct {
	Shell     string            `json:"shell"`
	ShellArgs []string          `json:"shellArgs"`
	CWD       string            `json:"cwd"`
	Env       map[string]string `json:"env"`
	Cols      uint16            `json:"cols"`
	Rows      uint16            `json:"rows"`
}

type DiscoveredShell struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Icon      string   `json:"icon"`
	IsDefault bool     `json:"isDefault"`
}

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

func (s *TerminalService) GetDefaultShell() string { return pty.DefaultShell("") }

func (s *TerminalService) ValidatePath(path, kind string) PathValidation {
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

func (s *TerminalService) DiscoverShells() []DiscoveredShell {
	defaultShell := s.GetDefaultShell()
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
