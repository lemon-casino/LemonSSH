// Test-only JSONL transport to the real durable Go store. No Wails/webview is
// needed; the TypeScript integration test drives the same ProfileClient API.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"github.com/binaricat/netcatty/internal/profile/store"
	"os"
)

func main() {
	s, err := store.Open(os.Args[1], nil)
	if err != nil {
		panic(err)
	}
	defer s.Close()
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			Method    string
			Domain    string
			Key       string
			Revision  uint64
			Mutations []store.Mutation
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			panic(err)
		}
		var value any
		var callErr error
		switch request.Method {
		case "revision":
			value, callErr = s.Revision()
		case "keys":
			value, callErr = s.DomainKeys(request.Domain)
		case "get":
			value, callErr = s.GetRaw(request.Domain, request.Key)
			if errors.Is(callErr, store.ErrNoSuchKey) {
				value = nil
				callErr = nil
			}
		case "write":
			value, callErr = s.Write(store.WriteRequest{ExpectedRevision: request.Revision, Mutations: request.Mutations})
		case "close":
			callErr = s.Close()
		}
		response := map[string]any{"value": value}
		if callErr != nil {
			response["error"] = callErr.Error()
		}
		if err := encoder.Encode(response); err != nil {
			panic(err)
		}
	}
}
