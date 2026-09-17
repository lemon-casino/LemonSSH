// Command netcatty-capability-codegen writes the TS agent tool spec
// projections from the Go capability authority (W05). Until W22 retires the
// Node generator (scripts/generate-capability-tools.cjs), use it to verify
// or preview the Go-owned projection:
//
//	go run ./cmd/netcatty-capability-codegen            # write both files to stdout-dir? see --out
//	go run ./cmd/netcatty-capability-codegen --out dir  # write cattyToolSpecs.json + globalAgentToolSpecs.json into dir
//
// Content is proven equal to the CJS projection by the fixture tests in
// internal/capability; object key order inside inputShape is sorted here,
// so byte-compare against the Node-generated files is not meaningful.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/binaricat/netcatty/internal/capability"
)

func main() {
	outDir := flag.String("out", "", "directory to write cattyToolSpecs.json and globalAgentToolSpecs.json into (default: stdout)")
	flag.Parse()

	registry := capability.Default()
	specs := map[string][]capability.AgentToolSpec{
		"cattyToolSpecs.json":       registry.ListAgentToolSpecs(capability.AgentKindSidebar),
		"globalAgentToolSpecs.json": registry.ListAgentToolSpecs(capability.AgentKindGlobal),
	}

	if *outDir == "" {
		for name, entries := range specs {
			fmt.Fprintf(os.Stderr, "## %s (%d specs)\n", name, len(entries))
			if err := writeJSON(os.Stdout, entries); err != nil {
				fail(name, err)
			}
		}
		return
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail(*outDir, err)
	}
	for name, entries := range specs {
		path := filepath.Join(*outDir, name)
		file, err := os.Create(path)
		if err != nil {
			fail(path, err)
		}
		if err := writeJSON(file, entries); err != nil {
			file.Close()
			fail(path, err)
		}
		if err := file.Close(); err != nil {
			fail(path, err)
		}
		fmt.Fprintf(os.Stderr, "wrote %d specs to %s\n", len(entries), path)
	}
}

func writeJSON(file *os.File, value any) error {
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func fail(target string, err error) {
	fmt.Fprintf(os.Stderr, "netcatty-capability-codegen: %s: %v\n", target, err)
	os.Exit(1)
}
