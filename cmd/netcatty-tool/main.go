// Command netcatty-tool is the native first-party CLI (W07, P7-02, AI-02).
// It resolves catalog CLI commands via internal/capability, authenticates
// to the running Netcatty host over internal/rpc using the discovery file
// left by the desktop shell, and prints the host result as JSON. When the
// app is not running it fails with a typed unavailable message, never a
// stack trace.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/binaricat/netcatty/internal/capability"
	"github.com/binaricat/netcatty/internal/rpc"
)

const (
	exitOK    = 0
	exitCall  = 1
	exitUsage = 2
)

type errorPayload struct {
	OK    bool         `json:"ok"`
	Error payloadError `json:"error"`
}

type payloadError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func main() {
	os.Exit(run())
}

func run() int {
	positionals, opts := parseArgs(os.Args[1:])
	if _, ok := opts["scopedSessionIds"]; !ok {
		opts["scopedSessionIds"] = []string{}
	}

	if len(positionals) == 0 || positionals[0] == "help" || positionals[0] == "--help" || positionals[0] == "-h" {
		fmt.Println(usageText())
		return exitOK
	}
	if positionals[0] == "capabilities" {
		printCapabilities()
		return exitOK
	}

	// Connection precedes command resolution so an app-down condition
	// surfaces as UNAVAILABLE regardless of arguments (CJS precedence).
	discoveryPath := os.Getenv("NETCATTY_TOOL_CLI_DISCOVERY_FILE")
	if discoveryPath == "" {
		emitError("UNAVAILABLE", "NETCATTY_TOOL_CLI_DISCOVERY_FILE is not set; launch via Netcatty.")
		return exitCall
	}
	client, err := rpc.Dial(discoveryPath)
	if err != nil {
		emitError("UNAVAILABLE", err.Error())
		return exitCall
	}
	defer client.Close()

	registry := capability.Default()
	def := registry.GetByCLICommand(positionals)
	if def == nil {
		emitError("INVALID_ARGUMENT", fmt.Sprintf("Unknown command: %s", strings.Join(positionals, " ")))
		return exitCall
	}

	// Catalog policy gate: chat-scoped commands refuse to run without the
	// chat scope reference, matching the CJS CLI fallback wording.
	if def.Policy.RequiresChatSession && opts["chatSessionId"] == nil {
		emitError("INVALID_ARGUMENT", fmt.Sprintf("Missing required --chat-session <id> for %s.", strings.Join(positionals, " ")))
		return exitCall
	}

	params, err := capability.BuildCatalogCLIParams(def.ID, opts)
	if err != nil {
		var argErr *capability.CLIArgError
		if errors.As(err, &argErr) {
			emitError(argErr.Code, argErr.Message)
			return exitCall
		}
		emitError("INVALID_ARGUMENT", err.Error())
		return exitCall
	}
	for _, scopeField := range []string{"chatSessionId", "scopedSessionIds"} {
		if value, ok := opts[scopeField]; ok && value != nil {
			params[scopeField] = value
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	result, err := client.Call(ctx, def.ResolveCLIRPCMethod(), params)
	if err != nil {
		var rpcErr *rpc.RPCError
		if errors.As(err, &rpcErr) {
			emitError(rpcErr.Code, rpcErr.Message)
		} else {
			emitError("UNKNOWN_ERROR", err.Error())
		}
		return exitCall
	}

	if opts["json"] == true {
		payload := map[string]any{"ok": true}
		var decoded map[string]any
		if err := json.Unmarshal(result, &decoded); err == nil {
			for key, value := range decoded {
				payload[key] = value
			}
		} else {
			payload["result"] = result
		}
		printJSON(payload)
	} else {
		printJSON(result)
	}
	return exitOK
}

// parseArgs splits argv into positional command parts and flag options.
// Flags take `--flag value` or `--flag=value`; a bare `--` collects the
// remaining words into the `command` option (terminal exec form).
func parseArgs(args []string) ([]string, map[string]any) {
	var positionals []string
	opts := map[string]any{}
	flagsByOptKey := optKeyIndex()

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			opts["command"] = append([]string(nil), args[i+1:]...)
			break
		}
		if arg == "--json" {
			opts["json"] = true
			continue
		}
		if arg == "--scope-session" {
			if i+1 < len(args) {
				i++
				if value := args[i]; value != "" {
					scoped := opts["scopedSessionIds"].([]string)
					opts["scopedSessionIds"] = append(scoped, value)
				}
			}
			continue
		}
		if strings.HasPrefix(arg, "--") {
			name := strings.TrimPrefix(arg, "--")
			var value string
			if eq := strings.Index(name, "="); eq >= 0 {
				value = name[eq+1:]
				name = name[:eq]
			} else if i+1 < len(args) {
				i++
				value = args[i]
			}
			optKey := name
			if mapped, ok := flagsByOptKey[name]; ok {
				optKey = mapped
			}
			opts[optKey] = value
			continue
		}
		positionals = append(positionals, arg)
	}
	return positionals, opts
}

// optKeyIndex maps long flag spellings to the options keys the catalog
// parameter builder expects (from CLIFieldBindings flags).
func optKeyIndex() map[string]string {
	index := map[string]string{
		"chat-session": "chatSessionId",
	}
	for _, binding := range capability.CLIFieldBindings {
		if binding.Flag == "--" {
			continue
		}
		index[strings.TrimPrefix(binding.Flag, "--")] = binding.OptKey
	}
	return index
}

func usageText() string {
	registry := capability.Default()
	lines := append([]string{
		"Netcatty tool CLI",
		"",
		"Usage: netcatty-tool <command> [subcommands] [--flags]",
		"",
		"Commands:",
	}, registry.FormatCLIHelpLines()...)
	return strings.Join(lines, "\n")
}

func printCapabilities() {
	registry := capability.Default()
	entries, err := json.MarshalIndent(registry.ListCLICapabilities(), "", "  ")
	if err != nil {
		emitError("UNKNOWN_ERROR", err.Error())
		return
	}
	fmt.Println(string(entries))
}

func printJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		emitError("UNKNOWN_ERROR", err.Error())
	}
}

func emitError(code, message string) {
	payload := errorPayload{OK: false, Error: payloadError{Code: code, Message: message}}
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", code, message)
		return
	}
	fmt.Fprintln(os.Stderr, string(out))
}
