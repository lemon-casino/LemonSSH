// Package capability is the Go authority for the Netcatty capability
// catalog (W05, P7-01, AI-01). It mirrors electron/capabilities on stable
// IDs, policies and surface bindings; catalog_electron_test.go pins the
// parity per ID against a fixture dumped from the CJS source. Dispatchers
// built on this package must reject capabilities that are unknown,
// non-implemented or lacking an injected host handler; nothing here widens
// access beyond what the catalog declares.
package capability

// Surface names one exposure channel of a capability.
type Surface string

const (
	SurfaceBuiltin     Surface = "builtin"
	SurfacePublic      Surface = "public"
	SurfaceCLI         Surface = "cli"
	SurfaceGlobal      Surface = "global"
	SurfaceCatty       Surface = "catty"       // renderer-local sidebar tools, no MCP/CLI exposure
	SurfaceGlobalAgent Surface = "globalAgent" // renderer-local global agent tools, no MCP/CLI exposure
)

// Status is the baseline build state of a capability. Runtime availability
// is tracked separately by consumers; status alone never grants access.
type Status string

const (
	StatusImplemented Status = "implemented"
	StatusPlanned     Status = "planned"
)

// PermissionMode is the agent safety mode a request executes under.
type PermissionMode string

const (
	ModeObserver PermissionMode = "observer"
	ModeConfirm  PermissionMode = "confirm"
	ModeAuto     PermissionMode = "auto"
)

// AgentKind names where an agent runs, orthogonal to RPC/MCP/CLI surfaces.
type AgentKind string

const (
	AgentKindSidebar AgentKind = "sidebar"
	AgentKindGlobal  AgentKind = "global"
)

// Policy is the static decision input set of one capability.
type Policy struct {
	Write                 bool
	SensitiveRead         bool
	LongRunning           bool
	RequiresChatSession   bool
	BypassesObserverBlock bool
	BypassesApproval      bool
	BypassesChatCancel    bool
}

// SurfaceBinding is one capability's registration on one surface. Absent
// fields stay nil/empty; ConfirmInConfirmMode is a tri-state: nil means
// "inherit from policy", non-nil overrides it.
type SurfaceBinding struct {
	RPCMethod            string
	MCPTool              string
	ToolName             string
	Command              []string
	ConfirmInConfirmMode *bool
}

// Definition is one catalog row. AgentKinds records explicit placement only;
// when empty, placement is inferred by the projection layer.
type Definition struct {
	ID          string
	Domain      string
	Status      Status
	Description string
	Policy      Policy
	Surfaces    map[Surface]SurfaceBinding
	AgentKinds  []AgentKind
}
