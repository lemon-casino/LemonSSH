package terminaluse

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/binaricat/netcatty/internal/platform/monitoring"
)

// Extended monitoring request DTOs are kept in the application layer so the
// Wails facade and any later RPC facade share the same validation rules.
type ProcessSignalOptions struct {
	SessionID string `json:"sessionId"`
	PID       int    `json:"pid"`
	Signal    string `json:"signal,omitempty"`
	Nice      *int   `json:"nice,omitempty"`
}

type TmuxSessionRequest struct {
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	Command   string `json:"command,omitempty"`
}

type TmuxTargetRequest struct {
	SessionID   string `json:"sessionId"`
	SessionName string `json:"sessionName"`
	WindowIndex int    `json:"windowIndex,omitempty"`
}

type TmuxActionRequest struct {
	SessionID   string `json:"sessionId"`
	Action      string `json:"action"`
	SessionName string `json:"sessionName,omitempty"`
	NewName     string `json:"newName,omitempty"`
	WindowName  string `json:"windowName,omitempty"`
	WindowIndex int    `json:"windowIndex,omitempty"`
	PaneIndex   int    `json:"paneIndex,omitempty"`
	Direction   string `json:"direction,omitempty"`
	Keys        string `json:"keys,omitempty"`
	Enter       bool   `json:"enter,omitempty"`
}

type SystemServiceActionRequest struct {
	SessionID string `json:"sessionId"`
	UnitName  string `json:"unitName"`
	Action    string `json:"action"`
	Scope     string `json:"scope,omitempty"`
}

type DockerInspectRequest struct {
	SessionID   string `json:"sessionId"`
	ContainerID string `json:"containerId,omitempty"`
	ImageID     string `json:"imageId,omitempty"`
}

type DockerActionRequest struct {
	SessionID   string `json:"sessionId"`
	ContainerID string `json:"containerId"`
	Action      string `json:"action"`
	NewName     string `json:"newName,omitempty"`
}

type DockerImageActionRequest struct {
	SessionID  string `json:"sessionId"`
	Action     string `json:"action"`
	ImageRef   string `json:"imageRef,omitempty"`
	ImageID    string `json:"imageId,omitempty"`
	Force      bool   `json:"force,omitempty"`
	All        bool   `json:"all,omitempty"`
	Repository string `json:"repository,omitempty"`
	Tag        string `json:"tag,omitempty"`
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func successResult() MonitoringResult { return MonitoringResult{Success: true} }

func (s *Service) SignalSystemProcess(ctx context.Context, options ProcessSignalOptions) MonitoringResult {
	if options.PID <= 1 {
		return monitoringFailure(fmt.Errorf("invalid process id"))
	}
	var command string
	if options.Nice != nil {
		if *options.Nice < -20 || *options.Nice > 19 {
			return monitoringFailure(fmt.Errorf("nice value must be between -20 and 19"))
		}
		command = fmt.Sprintf("renice -n %d -p %d", *options.Nice, options.PID)
	} else {
		signal := strings.ToUpper(strings.TrimSpace(options.Signal))
		if signal == "" {
			signal = "TERM"
		}
		allowed := map[string]bool{"TERM": true, "KILL": true, "INT": true, "HUP": true, "STOP": true, "CONT": true}
		if !allowed[signal] {
			return monitoringFailure(fmt.Errorf("unsupported process signal"))
		}
		command = fmt.Sprintf("kill -%s -- %d", signal, options.PID)
	}
	if _, err := s.monitoringExec(ctx, options.SessionID, command); err != nil {
		return monitoringFailure(err)
	}
	return successResult()
}

func (s *Service) SetupOSC7Tracking(ctx context.Context, sessionID, command string) MonitoringResult {
	if strings.TrimSpace(command) == "" {
		return monitoringFailure(fmt.Errorf("OSC 7 setup command is required"))
	}
	output, err := s.monitoringExec(ctx, sessionID, command)
	if err != nil {
		return monitoringFailure(err)
	}
	return MonitoringResult{Success: true, Output: output}
}

func (s *Service) CreateTmuxSession(ctx context.Context, request TmuxSessionRequest) MonitoringResult {
	if strings.TrimSpace(request.Name) == "" {
		return monitoringFailure(fmt.Errorf("tmux session name is required"))
	}
	command := "tmux new-session -d -s " + shellQuote(request.Name)
	if request.Command != "" {
		command += " " + shellQuote(request.Command)
	}
	if _, err := s.monitoringExec(ctx, request.SessionID, command); err != nil {
		return monitoringFailure(err)
	}
	return successResult()
}

func parseInt(value string) int {
	number, _ := strconv.Atoi(strings.TrimSpace(value))
	return number
}

func (s *Service) ListTmuxWindows(ctx context.Context, request TmuxTargetRequest) MonitoringResult {
	format := "#{window_index}\\t#{window_name}\\t#{window_panes}\\t#{window_active}\\t#{window_layout}"
	command := "tmux list-windows -t " + shellQuote(request.SessionName) + " -F " + shellQuote(format)
	text, err := s.monitoringExec(ctx, request.SessionID, command)
	if err != nil {
		return monitoringFailure(err)
	}
	rows := []monitoring.Row{}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			return monitoringFailure(fmt.Errorf("invalid tmux window row"))
		}
		rows = append(rows, monitoring.Row{"index": parseInt(parts[0]), "name": parts[1], "panes": parseInt(parts[2]), "active": parts[3] == "1", "layout": parts[4]})
	}
	return MonitoringResult{Success: true, Windows: rows}
}

func (s *Service) ListTmuxPanes(ctx context.Context, request TmuxTargetRequest) MonitoringResult {
	target := fmt.Sprintf("%s:%d", request.SessionName, request.WindowIndex)
	format := "#{pane_index}\\t#{pane_title}\\t#{pane_current_command}\\t#{pane_active}\\t#{pane_pid}\\t#{pane_width}\\t#{pane_height}"
	command := "tmux list-panes -t " + shellQuote(target) + " -F " + shellQuote(format)
	text, err := s.monitoringExec(ctx, request.SessionID, command)
	if err != nil {
		return monitoringFailure(err)
	}
	rows := []monitoring.Row{}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 7 {
			return monitoringFailure(fmt.Errorf("invalid tmux pane row"))
		}
		rows = append(rows, monitoring.Row{"index": parseInt(parts[0]), "title": parts[1], "command": parts[2], "active": parts[3] == "1", "pid": parseInt(parts[4]), "width": parseInt(parts[5]), "height": parseInt(parts[6])})
	}
	return MonitoringResult{Success: true, Panes: rows}
}

func (s *Service) ListTmuxClients(ctx context.Context, request TmuxTargetRequest) MonitoringResult {
	format := "#{client_name}\\t#{client_tty}\\t#{client_activity}\\t#{client_session}"
	command := "tmux list-clients -F " + shellQuote(format)
	text, err := s.monitoringExec(ctx, request.SessionID, command)
	if err != nil {
		// No attached clients is a valid state on several tmux versions.
		if strings.Contains(strings.ToLower(err.Error()), "no server running") {
			return MonitoringResult{Success: true, Clients: []monitoring.Row{}}
		}
		return monitoringFailure(err)
	}
	rows := []monitoring.Row{}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			return monitoringFailure(fmt.Errorf("invalid tmux client row"))
		}
		if request.SessionName != "" && parts[3] != request.SessionName {
			continue
		}
		rows = append(rows, monitoring.Row{"name": parts[0], "tty": parts[1], "activity": parts[2], "session": parts[3]})
	}
	return MonitoringResult{Success: true, Clients: rows}
}

func (s *Service) TmuxAction(ctx context.Context, request TmuxActionRequest) MonitoringResult {
	targetWindow := fmt.Sprintf("%s:%d", request.SessionName, request.WindowIndex)
	targetPane := fmt.Sprintf("%s.%d", targetWindow, request.PaneIndex)
	var command string
	switch request.Action {
	case "killSession":
		command = "tmux kill-session -t " + shellQuote(request.SessionName)
	case "renameSession":
		command = "tmux rename-session -t " + shellQuote(request.SessionName) + " " + shellQuote(request.NewName)
	case "detachSession":
		command = "tmux detach-client -s " + shellQuote(request.SessionName)
	case "createWindow":
		command = "tmux new-window -t " + shellQuote(request.SessionName)
		if request.WindowName != "" {
			command += " -n " + shellQuote(request.WindowName)
		}
	case "killWindow":
		command = "tmux kill-window -t " + shellQuote(targetWindow)
	case "renameWindow":
		command = "tmux rename-window -t " + shellQuote(targetWindow) + " " + shellQuote(request.NewName)
	case "killPane":
		command = "tmux kill-pane -t " + shellQuote(targetPane)
	case "splitPane":
		flag := "-v"
		if request.Direction == "horizontal" {
			flag = "-h"
		}
		command = "tmux split-window " + flag + " -t " + shellQuote(targetPane)
	case "sendKeys":
		command = "tmux send-keys -t " + shellQuote(targetPane) + " -l " + shellQuote(request.Keys)
		if request.Enter {
			command += " && tmux send-keys -t " + shellQuote(targetPane) + " Enter"
		}
	case "selectWindow":
		command = "tmux select-window -t " + shellQuote(targetWindow)
	case "killServer":
		command = "tmux kill-server"
	default:
		return monitoringFailure(fmt.Errorf("unsupported tmux action"))
	}
	if _, err := s.monitoringExec(ctx, request.SessionID, command); err != nil {
		return monitoringFailure(err)
	}
	return successResult()
}

var ssProcessPattern = regexp.MustCompile(`pid=(\d+).*?\"([^\"]+)\"`)
var netstatProcessPattern = regexp.MustCompile(`^(\d+)/(.*)$`)
var systemdUnitPattern = regexp.MustCompile(`^[A-Za-z0-9_.@:-]+$`)

func splitEndpoint(value string) (string, int) {
	value = strings.TrimSpace(value)
	index := strings.LastIndex(value, ":")
	if index < 0 {
		return value, 0
	}
	port, _ := strconv.Atoi(strings.Trim(value[index+1:], "*"))
	address := strings.Trim(value[:index], "[]")
	if address == "" {
		address = "*"
	}
	return address, port
}

func parseListeningPorts(text string) ([]monitoring.Row, error) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 {
		return []monitoring.Row{}, nil
	}
	mode := strings.TrimSpace(lines[0])
	rows := []monitoring.Row{}
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		protocol := strings.ToLower(fields[0])
		localIndex := 4
		processText := ""
		if mode == "NETSTAT" {
			localIndex = 3
		}
		if localIndex >= len(fields) {
			continue
		}
		address, port := splitEndpoint(fields[localIndex])
		pid := 0
		processName := ""
		if mode == "SS" {
			processText = strings.Join(fields[localIndex+2:], " ")
			if match := ssProcessPattern.FindStringSubmatch(processText); len(match) == 3 {
				pid = parseInt(match[1])
				processName = match[2]
			}
		} else if len(fields) > 6 {
			if match := netstatProcessPattern.FindStringSubmatch(fields[len(fields)-1]); len(match) == 3 {
				pid = parseInt(match[1])
				processName = match[2]
			}
		}
		isIPv6 := strings.Contains(address, ":")
		if strings.HasPrefix(protocol, "tcp") && isIPv6 {
			protocol = "tcp6"
		}
		if strings.HasPrefix(protocol, "udp") && isIPv6 {
			protocol = "udp6"
		}
		if protocol != "tcp" && protocol != "tcp6" && protocol != "udp" && protocol != "udp6" {
			protocol = "unknown"
		}
		var pidValue any
		if pid > 0 {
			pidValue = pid
		}
		rows = append(rows, monitoring.Row{"protocol": protocol, "address": address, "port": port, "pid": pidValue, "processName": processName, "id": fmt.Sprintf("%s|%s|%d|%d", protocol, address, port, pid)})
	}
	return rows, nil
}

func (s *Service) ListListeningPorts(ctx context.Context, sessionID string) MonitoringResult {
	command := `export LC_ALL=C; if command -v ss >/dev/null 2>&1; then echo SS; ss -H -lntup; elif command -v netstat >/dev/null 2>&1; then echo NETSTAT; netstat -lntup; else echo 'port collector unavailable' >&2; exit 127; fi`
	text, err := s.monitoringExec(ctx, sessionID, command)
	if err != nil {
		return monitoringFailure(err)
	}
	rows, err := parseListeningPorts(text)
	if err != nil {
		return monitoringFailure(err)
	}
	return MonitoringResult{Success: true, Ports: rows}
}

func parseSystemdUnits(text string) ([]monitoring.Row, error) {
	rows := []monitoring.Row{}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 6)
		if len(parts) < 6 {
			return nil, fmt.Errorf("invalid systemd unit row")
		}
		rows = append(rows, monitoring.Row{"scope": parts[0], "name": parts[1], "loadState": parts[2], "activeState": parts[3], "subState": parts[4], "description": parts[5]})
	}
	return rows, nil
}

func (s *Service) ListSystemServices(ctx context.Context, sessionID string) MonitoringResult {
	format := `awk '{ scope=$1; $1=""; sub(/^[ ]+/, ""); split($0,a," "); name=a[1]; load=a[2]; active=a[3]; substate=a[4]; desc=$0; sub(/^[^ ]+ +[^ ]+ +[^ ]+ +[^ ]+ +/,"",desc); printf "%s\t%s\t%s\t%s\t%s\t%s\n",scope,name,load,active,substate,desc }'`
	command := `export LC_ALL=C; command -v systemctl >/dev/null 2>&1 || { echo 'systemctl unavailable' >&2; exit 127; }; { systemctl list-units --type=service --all --no-legend --no-pager --plain 2>/dev/null | sed 's/^/system /'; systemctl --user list-units --type=service --all --no-legend --no-pager --plain 2>/dev/null | sed 's/^/user /'; } | ` + format
	text, err := s.monitoringExec(ctx, sessionID, command)
	if err != nil {
		return monitoringFailure(err)
	}
	rows, err := parseSystemdUnits(text)
	if err != nil {
		return monitoringFailure(err)
	}
	return MonitoringResult{Success: true, Units: rows}
}

func (s *Service) SystemServiceAction(ctx context.Context, request SystemServiceActionRequest) MonitoringResult {
	allowed := map[string]bool{"start": true, "stop": true, "restart": true, "enable": true, "disable": true, "reload": true}
	if !allowed[request.Action] {
		return monitoringFailure(fmt.Errorf("unsupported service action"))
	}
	if !systemdUnitPattern.MatchString(request.UnitName) {
		return monitoringFailure(fmt.Errorf("invalid service name"))
	}
	command := "systemctl "
	if request.Scope == "user" {
		command += "--user "
	}
	command += request.Action + " " + shellQuote(request.UnitName)
	if _, err := s.monitoringExec(ctx, request.SessionID, command); err != nil {
		return monitoringFailure(err)
	}
	return successResult()
}

func parseInspect(text string) (map[string]any, error) {
	var rows []map[string]any
	if err := json.Unmarshal([]byte(text), &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("inspect returned no rows")
	}
	return rows[0], nil
}

func (s *Service) DockerInspect(ctx context.Context, request DockerInspectRequest) MonitoringResult {
	if strings.TrimSpace(request.ContainerID) == "" {
		return monitoringFailure(fmt.Errorf("container id is required"))
	}
	text, err := s.monitoringExec(ctx, request.SessionID, "docker inspect --type container "+shellQuote(request.ContainerID))
	if err != nil {
		return monitoringFailure(err)
	}
	inspect, err := parseInspect(text)
	if err != nil {
		return monitoringFailure(err)
	}
	return MonitoringResult{Success: true, Inspect: inspect}
}

func (s *Service) DockerImageInspect(ctx context.Context, request DockerInspectRequest) MonitoringResult {
	if strings.TrimSpace(request.ImageID) == "" {
		return monitoringFailure(fmt.Errorf("image id is required"))
	}
	text, err := s.monitoringExec(ctx, request.SessionID, "docker image inspect "+shellQuote(request.ImageID))
	if err != nil {
		return monitoringFailure(err)
	}
	inspect, err := parseInspect(text)
	if err != nil {
		return monitoringFailure(err)
	}
	return MonitoringResult{Success: true, Inspect: inspect}
}

func (s *Service) DockerAction(ctx context.Context, request DockerActionRequest) MonitoringResult {
	if strings.TrimSpace(request.ContainerID) == "" {
		return monitoringFailure(fmt.Errorf("container id is required"))
	}
	allowed := map[string]bool{"start": true, "stop": true, "restart": true, "rm": true, "pause": true, "unpause": true, "kill": true}
	var command string
	if request.Action == "rename" {
		if request.NewName == "" {
			return monitoringFailure(fmt.Errorf("new container name is required"))
		}
		command = "docker rename " + shellQuote(request.ContainerID) + " " + shellQuote(request.NewName)
	} else if allowed[request.Action] {
		command = "docker " + request.Action + " " + shellQuote(request.ContainerID)
	} else {
		return monitoringFailure(fmt.Errorf("unsupported docker action"))
	}
	if _, err := s.monitoringExec(ctx, request.SessionID, command); err != nil {
		return monitoringFailure(err)
	}
	return successResult()
}

func (s *Service) DockerImageAction(ctx context.Context, request DockerImageActionRequest) MonitoringResult {
	var command string
	actionContext := ctx
	cancel := func() {}
	switch request.Action {
	case "pull":
		if strings.TrimSpace(request.ImageRef) == "" {
			return monitoringFailure(fmt.Errorf("image reference is required"))
		}
		command = "docker pull " + shellQuote(request.ImageRef)
		actionContext, cancel = context.WithTimeout(ctx, 10*time.Minute)
	case "rm":
		if strings.TrimSpace(request.ImageID) == "" {
			return monitoringFailure(fmt.Errorf("image id is required"))
		}
		command = "docker image rm "
		if request.Force {
			command += "--force "
		}
		command += shellQuote(request.ImageID)
	case "prune":
		command = "docker image prune -f"
		if request.All {
			command += " -a"
		}
	case "tag":
		if strings.TrimSpace(request.ImageID) == "" || strings.TrimSpace(request.Repository) == "" {
			return monitoringFailure(fmt.Errorf("image id and repository are required"))
		}
		tag := request.Tag
		if tag == "" {
			tag = "latest"
		}
		command = "docker tag " + shellQuote(request.ImageID) + " " + shellQuote(request.Repository+":"+tag)
	default:
		return monitoringFailure(fmt.Errorf("unsupported docker image action"))
	}
	defer cancel()
	output, err := s.monitoringExec(actionContext, request.SessionID, command)
	if err != nil {
		return monitoringFailure(err)
	}
	return MonitoringResult{Success: true, Output: output}
}
