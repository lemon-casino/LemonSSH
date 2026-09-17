package capability

import (
	"sort"
	"strings"
	"sync"
)

// Registry indexes the catalog exactly like electron/capabilities/registry.cjs:
// surface-scoped rpc/mcp keys, joined CLI commands, tool names and domains.
// Where one surface binds the same rpcMethod twice, the later catalog row
// wins (insertion-order assignment), matching the CJS baseline.
type Registry struct {
	all          []Definition
	byID         map[string]*Definition
	byRPCMethod  map[string]*Definition
	byMCPTool    map[string]*Definition
	byCLICommand map[string]*Definition
	byToolName   map[string]*Definition
	byDomain     map[string][]*Definition
}

// NewRegistry builds indexes over defs. The slice order is preserved for
// listings; callers must not mutate defs afterwards.
func NewRegistry(defs []Definition) *Registry {
	registry := &Registry{
		all:          defs,
		byID:         make(map[string]*Definition, len(defs)),
		byRPCMethod:  make(map[string]*Definition),
		byMCPTool:    make(map[string]*Definition),
		byCLICommand: make(map[string]*Definition),
		byToolName:   make(map[string]*Definition),
		byDomain:     make(map[string][]*Definition),
	}
	for i := range defs {
		def := &defs[i]
		registry.byID[def.ID] = def
		registry.byDomain[def.Domain] = append(registry.byDomain[def.Domain], def)
		for surface, binding := range def.Surfaces {
			if binding.RPCMethod != "" {
				registry.byRPCMethod[string(surface)+":"+binding.RPCMethod] = def
			}
			if binding.MCPTool != "" {
				registry.byMCPTool[string(surface)+":"+binding.MCPTool] = def
			}
			if len(binding.Command) > 0 {
				registry.byCLICommand[strings.Join(binding.Command, " ")] = def
			}
			if binding.ToolName != "" {
				registry.byToolName[binding.ToolName] = def
			}
		}
	}
	return registry
}

var defaultRegistry = sync.OnceValue(func() *Registry { return NewRegistry(Catalog) })

// Default returns the process-wide registry over Catalog.
func Default() *Registry { return defaultRegistry() }

// ListOptions filters List results. Zero-value options return every row.
type ListOptions struct {
	Status  Status
	Domain  string
	Surface Surface
}

// List returns catalog rows matching the filter, in catalog order.
func (r *Registry) List(opts ListOptions) []*Definition {
	var out []*Definition
	for i := range r.all {
		def := &r.all[i]
		if opts.Status != "" && def.Status != opts.Status {
			continue
		}
		if opts.Domain != "" && def.Domain != opts.Domain {
			continue
		}
		if opts.Surface != "" {
			if _, ok := def.Surfaces[opts.Surface]; !ok {
				continue
			}
		}
		out = append(out, def)
	}
	return out
}

// GetByID returns the row with the given stable ID, or nil.
func (r *Registry) GetByID(id string) *Definition { return r.byID[id] }

// GetByRPCMethod resolves one surface-scoped method; surface defaults do not
// exist here — callers pass SurfaceBuiltin explicitly like the CJS default.
func (r *Registry) GetByRPCMethod(method string, surface Surface) *Definition {
	return r.byRPCMethod[string(surface)+":"+method]
}

// GetByMCPTool resolves one surface-scoped MCP tool name, or nil.
func (r *Registry) GetByMCPTool(tool string, surface Surface) *Definition {
	return r.byMCPTool[string(surface)+":"+tool]
}

// GetByCLICommand resolves a CLI command by its argv parts, or nil.
func (r *Registry) GetByCLICommand(parts []string) *Definition {
	return r.byCLICommand[strings.Join(parts, " ")]
}

// GetByToolName resolves a renderer-local tool name (catty/globalAgent), or nil.
func (r *Registry) GetByToolName(name string) *Definition { return r.byToolName[name] }

// RPCFilter narrows RPCMethodsForSurface. Fields left false are not filtered.
type RPCFilter struct {
	Status      Status
	Write       bool
	LongRunning bool
}

// RPCMethodsForSurface lists the distinct rpc methods bound on one surface.
// The result is sorted for deterministic output.
func (r *Registry) RPCMethodsForSurface(surface Surface, filter RPCFilter) []string {
	var methods []string
	seen := make(map[string]bool)
	for i := range r.all {
		def := &r.all[i]
		binding, ok := def.Surfaces[surface]
		if !ok || binding.RPCMethod == "" {
			continue
		}
		if filter.Status != "" && def.Status != filter.Status {
			continue
		}
		if filter.Write && !def.Policy.Write {
			continue
		}
		if filter.LongRunning && !def.Policy.LongRunning {
			continue
		}
		if !seen[binding.RPCMethod] {
			seen[binding.RPCMethod] = true
			methods = append(methods, binding.RPCMethod)
		}
	}
	sort.Strings(methods)
	return methods
}
