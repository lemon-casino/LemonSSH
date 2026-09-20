package capability

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// CLIArgError is a typed CLI argument failure; Code matches the CJS
// INVALID_ARGUMENT code so first-party launchers can branch on it.
type CLIArgError struct {
	Code    string
	Message string
}

func (e *CLIArgError) Error() string { return e.Code + ": " + e.Message }

// CLIFieldBinding maps a tool-input field to its CLI flag and options key.
type CLIFieldBinding struct {
	Flag   string
	OptKey string
}

// CLIFieldBindings mirrors CLI_FIELD_BINDINGS in cliAdapter.cjs.
var CLIFieldBindings = map[string]CLIFieldBinding{
	"hostId":           {Flag: "--host-id", OptKey: "hostId"},
	"filename":         {Flag: "--filename", OptKey: "filename"},
	"filePath":         {Flag: "--file-path", OptKey: "filePath"},
	"snippetId":        {Flag: "--snippet-id", OptKey: "snippetId"},
	"scriptId":         {Flag: "--script-id", OptKey: "scriptId"},
	"runId":            {Flag: "--run-id", OptKey: "runId"},
	"ruleId":           {Flag: "--rule-id", OptKey: "ruleId"},
	"notes":            {Flag: "--notes", OptKey: "notes"},
	"sessionId":        {Flag: "--session", OptKey: "sessionId"},
	"variables":        {Flag: "--variables", OptKey: "variables"},
	"wait":             {Flag: "--wait", OptKey: "wait"},
	"scriptIds":        {Flag: "--script-ids", OptKey: "scriptIds"},
	"label":            {Flag: "--label", OptKey: "label"},
	"kind":             {Flag: "--kind", OptKey: "kind"},
	"trigger":          {Flag: "--trigger", OptKey: "trigger"},
	"triggerPattern":   {Flag: "--trigger-pattern", OptKey: "triggerPattern"},
	"targets":          {Flag: "--targets", OptKey: "targets"},
	"targetGroups":     {Flag: "--target-groups", OptKey: "targetGroups"},
	"targetsAllHosts":  {Flag: "--targets-all-hosts", OptKey: "targetsAllHosts"},
	"description":      {Flag: "--description", OptKey: "description"},
	"language":         {Flag: "--language", OptKey: "language"},
	"package":          {Flag: "--package", OptKey: "package"},
	"shortkey":         {Flag: "--shortkey", OptKey: "shortkey"},
	"noAutoRun":        {Flag: "--no-auto-run", OptKey: "noAutoRun"},
	"multiLineRunMode": {Flag: "--multi-line-run-mode", OptKey: "multiLineRunMode"},
	"path":             {Flag: "--remote-path", OptKey: "remotePath"},
	"remotePath":       {Flag: "--remote-path", OptKey: "remotePath"},
	"localPath":        {Flag: "--local-path", OptKey: "localPath"},
	"oldPath":          {Flag: "--old-remote-path", OptKey: "oldRemotePath"},
	"newPath":          {Flag: "--new-remote-path", OptKey: "newRemotePath"},
	"content":          {Flag: "--content", OptKey: "content"},
	"mode":             {Flag: "--mode", OptKey: "mode"},
	"command":          {Flag: "--", OptKey: "command"},
	"jobId":            {Flag: "--job", OptKey: "jobId"},
	"offset":           {Flag: "--offset", OptKey: "offset"},
}

// CLIFieldBindingFor exposes every schema field, including optional fields
// added after the original CLI flag table was written.
func CLIFieldBindingFor(field string) CLIFieldBinding {
	if binding, ok := CLIFieldBindings[field]; ok {
		return binding
	}
	var flag strings.Builder
	flag.WriteString("--")
	for _, ch := range field {
		if unicode.IsUpper(ch) {
			flag.WriteByte('-')
			ch = unicode.ToLower(ch)
		}
		flag.WriteRune(ch)
	}
	return CLIFieldBinding{Flag: flag.String(), OptKey: field}
}

// ResolveCLIRPCMethod returns the method a CLI command dispatches to:
// builtin, then global, then public; "" when none.
func (d *Definition) ResolveCLIRPCMethod() string {
	for _, surface := range []Surface{SurfaceBuiltin, SurfaceGlobal, SurfacePublic} {
		if binding, ok := d.Surfaces[surface]; ok && binding.RPCMethod != "" {
			return binding.RPCMethod
		}
	}
	return ""
}

// CLICapabilityEntry is one row of the CLI help/listing projection.
type CLICapabilityEntry struct {
	ID          string   `json:"id"`
	Domain      string   `json:"domain"`
	Status      Status   `json:"status"`
	Description string   `json:"description"`
	Command     []string `json:"command"`
	RPCMethod   string   `json:"rpcMethod"`
	Policy      Policy   `json:"policy"`
}

// ListCLICapabilities lists implemented capabilities with a CLI command
// binding, in catalog order.
func (r *Registry) ListCLICapabilities() []CLICapabilityEntry {
	var entries []CLICapabilityEntry
	for _, def := range r.List(ListOptions{Status: StatusImplemented, Surface: SurfaceCLI}) {
		binding := def.Surfaces[SurfaceCLI]
		if len(binding.Command) == 0 {
			continue
		}
		entries = append(entries, CLICapabilityEntry{
			ID:          def.ID,
			Domain:      def.Domain,
			Status:      def.Status,
			Description: def.Description,
			Command:     binding.Command,
			RPCMethod:   def.ResolveCLIRPCMethod(),
			Policy:      def.Policy,
		})
	}
	return entries
}

// FormatCLIHelpLines renders the help listing with the CJS prefix.
func (r *Registry) FormatCLIHelpLines() []string {
	var lines []string
	for _, entry := range r.ListCLICapabilities() {
		suffix := ""
		if entry.Status == StatusPlanned {
			suffix = " (planned)"
		}
		lines = append(lines, fmt.Sprintf("  netcatty-tool %s%s", strings.Join(entry.Command, " "), suffix))
	}
	return lines
}

// BuildCatalogCLIParams converts parsed CLI options into request params
// for one capability, mirroring buildCatalogCliParams: flags bind through
// CLIFieldBindings, `--variables` must parse as JSON, `--offset` coerces to
// a number, and missing required fields fail with INVALID_ARGUMENT.
func BuildCatalogCLIParams(capabilityID string, opts map[string]any) (map[string]any, error) {
	fields, ok := ToolInputFields(capabilityID)
	if !ok {
		return map[string]any{}, nil
	}

	// Deterministic iteration for stable error ordering.
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)

	params := map[string]any{}
	for _, fieldName := range names {
		fieldDef := fields[fieldName]
		binding := CLIFieldBindingFor(fieldName)

		value := opts[binding.OptKey]
		if fieldName == "command" {
			if array, isArray := value.([]string); isArray {
				if len(array) == 1 {
					value = array[0]
				} else {
					value = nil
				}
			}
		}
		if fieldName == "variables" {
			if text, isText := value.(string); isText && strings.TrimSpace(text) != "" {
				var parsed any
				if err := json.Unmarshal([]byte(text), &parsed); err != nil {
					return nil, &CLIArgError{Code: "INVALID_ARGUMENT", Message: fmt.Sprintf("--variables must be valid JSON for %s.", capabilityID)}
				}
				value = parsed
			}
		}
		if fieldName == "offset" {
			switch typed := value.(type) {
			case nil:
			case float64:
			case int:
				value = float64(typed)
			case string:
				if typed == "" {
					value = nil
				} else {
					parsed, err := strconv.ParseFloat(typed, 64)
					if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed < 0 || math.Trunc(parsed) != parsed {
						return nil, &CLIArgError{Code: "INVALID_ARGUMENT", Message: fmt.Sprintf("--offset must be a number for %s.", capabilityID)}
					}
					value = parsed
				}
			}
		}

		allowEmpty := fieldName == "content" || fieldName == "notes"
		empty := value == nil || (value == "" && !allowEmpty)
		if empty {
			if !fieldDef.Optional {
				return nil, &CLIArgError{Code: "INVALID_ARGUMENT", Message: fmt.Sprintf("Missing required %s for %s.", binding.Flag, capabilityID)}
			}
			continue
		}

		params[fieldName] = value
	}

	return params, nil
}
