package capability

import (
	"encoding/json"
	"sort"
	"strings"
)

// cattyCapabilityDenylist excludes implemented, CLI-only or meta
// capabilities from both agent kinds even when they have schemas.
var cattyCapabilityDenylist = map[string]bool{
	"meta.status":    true,
	"session.cancel": true,
	"session.resume": true,
	"session.get":    true,
}

// MCPToolName resolves the MCP tool name across the public then builtin
// surfaces, or "".
func (d *Definition) MCPToolName() string {
	if binding, ok := d.Surfaces[SurfacePublic]; ok && binding.MCPTool != "" {
		return binding.MCPTool
	}
	if binding, ok := d.Surfaces[SurfaceBuiltin]; ok {
		return binding.MCPTool
	}
	return ""
}

// CattyToolName resolves the sidebar tool name: renderer-local name first,
// then MCP name, then the dotted ID flattened.
func (d *Definition) CattyToolName() string {
	if binding, ok := d.Surfaces[SurfaceCatty]; ok && binding.ToolName != "" {
		return binding.ToolName
	}
	if mcp := d.MCPToolName(); mcp != "" {
		return mcp
	}
	return strings.ReplaceAll(d.ID, ".", "_")
}

// agentToolName resolves the tool name for one agent kind.
func (d *Definition) agentToolName(kind AgentKind) string {
	if kind == AgentKindGlobal {
		if binding, ok := d.Surfaces[SurfaceGlobalAgent]; ok && binding.ToolName != "" {
			return binding.ToolName
		}
		if mcp := d.MCPToolName(); mcp != "" {
			return mcp
		}
		return strings.ReplaceAll(d.ID, ".", "_")
	}
	return d.CattyToolName()
}

// AgentRPCMethod resolves the rpc method an agent dispatches to: builtin,
// then global, then public; "" when none.
func (d *Definition) AgentRPCMethod() string {
	for _, surface := range []Surface{SurfaceBuiltin, SurfaceGlobal, SurfacePublic} {
		if binding, ok := d.Surfaces[surface]; ok && binding.RPCMethod != "" {
			return binding.RPCMethod
		}
	}
	return ""
}

// ToolDescription returns the catalog description with the model hint
// appended when one exists.
func (d *Definition) ToolDescription() string {
	hint := ModelDescriptionHint(d.ID)
	if hint == "" {
		return d.Description
	}
	return d.Description + " " + hint
}

// IsAgentLocalOnlyCapability reports whether the capability runs only as a
// renderer-local tool for the kind (no RPC, no MCP exposure).
func (d *Definition) IsAgentLocalOnlyCapability(kind AgentKind) bool {
	local := false
	if kind == AgentKindGlobal {
		binding, ok := d.Surfaces[SurfaceGlobalAgent]
		local = ok && binding.ToolName != ""
	} else {
		binding, ok := d.Surfaces[SurfaceCatty]
		local = ok && binding.ToolName != ""
	}
	return local && d.AgentRPCMethod() == "" && d.MCPToolName() == ""
}

// ResolveAgentKinds returns the agents that may use the capability:
// explicit placement wins, then globalAgent-only, then sidebar-local-only,
// then the shared RPC/MCP default (both kinds).
func (d *Definition) ResolveAgentKinds() []AgentKind {
	if len(d.AgentKinds) > 0 {
		return d.AgentKinds
	}
	if _, ok := d.Surfaces[SurfaceGlobalAgent]; ok {
		return []AgentKind{AgentKindGlobal}
	}
	if d.IsAgentLocalOnlyCapability(AgentKindSidebar) {
		return []AgentKind{AgentKindSidebar}
	}
	if d.agentEligibleForKind(AgentKindSidebar, true) {
		return []AgentKind{AgentKindSidebar, AgentKindGlobal}
	}
	return nil
}

func (d *Definition) agentEligibleForKind(kind AgentKind, skipAgentKindCheck bool) bool {
	if d.Status != StatusImplemented {
		return false
	}
	if cattyCapabilityDenylist[d.ID] {
		return false
	}
	if !skipAgentKindCheck {
		placed := false
		for _, candidate := range d.ResolveAgentKinds() {
			if candidate == kind {
				placed = true
				break
			}
		}
		if !placed {
			return false
		}
	}
	if !HasToolInputFields(d.ID) {
		return false
	}
	if d.IsAgentLocalOnlyCapability(kind) {
		return true
	}
	if binding, ok := d.Surfaces[SurfaceBuiltin]; ok && binding.RPCMethod != "" {
		return true
	}
	if binding, ok := d.Surfaces[SurfaceGlobal]; ok && binding.RPCMethod != "" {
		return true
	}
	return d.MCPToolName() != ""
}

// IsAgentEligibleForKind reports whether the capability appears in the
// given agent's tool list.
func (d *Definition) IsAgentEligibleForKind(kind AgentKind) bool {
	return d.agentEligibleForKind(kind, false)
}

// AgentToolSpec is one projected agent tool; JSON tags and field order
// match the generated cattyToolSpecs.json / globalAgentToolSpecs.json.
type AgentToolSpec struct {
	CapabilityID   string                    `json:"capabilityId"`
	ToolName       string                    `json:"toolName"`
	RPCMethod      *string                   `json:"rpcMethod"`
	LocalExecution bool                      `json:"localExecution"`
	Description    string                    `json:"description"`
	InputShape     map[string]ToolInputField `json:"inputShape"`
	Policy         Policy                    `json:"policy"`
	AgentKind      AgentKind                 `json:"agentKind,omitempty"`
}

// ToolSchemaFor renders one capability's JSON Schema (2020-12 object with
// properties from the tool-input inventory; required from non-optional
// fields).
func ToolSchemaFor(def *Definition) json.RawMessage {
	fields, _ := ToolInputFields(def.ID)
	properties := make(map[string]any, len(fields))
	var required []string
	for name, field := range fields {
		property := map[string]any{"type": field.Type}
		if field.Description != "" {
			property["description"] = field.Description
		}
		properties[name] = property
		if !field.Optional {
			required = append(required, name)
		}
	}
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		sort.Strings(required)
		schema["required"] = required
	}
	raw, _ := json.Marshal(schema)
	return raw
}

// ListAgentToolSpecs projects the catalog into the tool list for one agent
// kind, in catalog order.
func (r *Registry) ListAgentToolSpecs(kind AgentKind) []AgentToolSpec {
	var specs []AgentToolSpec
	for _, def := range r.List(ListOptions{}) {
		if !def.IsAgentEligibleForKind(kind) {
			continue
		}
		fields, _ := ToolInputFields(def.ID)
		var rpcMethod *string
		if method := def.AgentRPCMethod(); method != "" {
			rpcMethod = &method
		}
		spec := AgentToolSpec{
			CapabilityID:   def.ID,
			ToolName:       def.agentToolName(kind),
			RPCMethod:      rpcMethod,
			LocalExecution: def.IsAgentLocalOnlyCapability(kind),
			Description:    def.ToolDescription(),
			InputShape:     fields,
			Policy:         def.Policy,
		}
		if kind != AgentKindSidebar {
			spec.AgentKind = kind
		}
		specs = append(specs, spec)
	}
	return specs
}

// ToolSurface is one capability's projection onto an RPC surface, matching
// listToolSurfaces in toolSurfaces.cjs.
type ToolSurface struct {
	CapabilityID    string                    `json:"capabilityId"`
	Domain          string                    `json:"domain"`
	ToolName        string                    `json:"toolName"`
	MCPTool         *string                   `json:"mcpTool"`
	RPCMethod       *string                   `json:"rpcMethod"`
	PublicRPCMethod *string                   `json:"publicRpcMethod"`
	Description     string                    `json:"description"`
	Policy          Policy                    `json:"policy"`
	InputShape      map[string]ToolInputField `json:"inputShape"`
	CattyEnabled    bool                      `json:"cattyEnabled"`
}

// ListToolSurfaces projects the catalog onto one surface. status filters to
// one baseline status; includeCatty mirrors the CJS flag of the same name.
func (r *Registry) ListToolSurfaces(surface Surface, status Status, includeCatty bool) []ToolSurface {
	var tools []ToolSurface
	for _, def := range r.List(ListOptions{}) {
		if def.Status != status {
			continue
		}
		binding, ok := def.Surfaces[surface]
		if !ok {
			binding, ok = def.Surfaces[SurfacePublic]
		}
		if !ok {
			binding, ok = def.Surfaces[SurfaceBuiltin]
		}
		if !ok {
			continue
		}

		mcpTool := def.MCPToolName()
		if !includeCatty && mcpTool == "" {
			continue
		}

		var rpcMethod *string
		builtinBinding, hasBuiltin := def.Surfaces[SurfaceBuiltin]
		switch {
		case hasBuiltin && builtinBinding.RPCMethod != "":
			rpcMethod = &builtinBinding.RPCMethod
		case binding.RPCMethod != "":
			rpcMethod = &binding.RPCMethod
		}

		var publicRPCMethod *string
		if publicBinding, ok := def.Surfaces[SurfacePublic]; ok && publicBinding.RPCMethod != "" {
			publicRPCMethod = &publicBinding.RPCMethod
		}

		fields, _ := ToolInputFields(def.ID)
		tools = append(tools, ToolSurface{
			CapabilityID:    def.ID,
			Domain:          def.Domain,
			ToolName:        def.CattyToolName(),
			MCPTool:         nullableString(mcpTool),
			RPCMethod:       rpcMethod,
			PublicRPCMethod: publicRPCMethod,
			Description:     def.ToolDescription(),
			Policy:          def.Policy,
			InputShape:      fields,
			CattyEnabled:    includeCatty && (HasToolInputFields(def.ID) || mcpTool != ""),
		})
	}
	return tools
}

// ListMcpTools returns implemented tools exposed over the public MCP
// surface.
func (r *Registry) ListMcpTools() []ToolSurface {
	var tools []ToolSurface
	for _, tool := range r.ListToolSurfaces(SurfacePublic, StatusImplemented, false) {
		if tool.MCPTool != nil {
			tools = append(tools, tool)
		}
	}
	return tools
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
