package main

import (
	"reflect"
	"testing"
)

func TestParseScopeFlagsWithoutPreinitialization(t *testing.T) {
	command, options := parseArgs([]string{"exec", "--chat-session", "chat", "--scope-session", "one", "--scope-session", "two", "--", "echo hi"})
	if !reflect.DeepEqual(command, []string{"exec"}) || !reflect.DeepEqual(options["scopedSessionIds"], []string{"one", "two"}) {
		t.Fatalf("%v %v", command, options)
	}
	if options["chatSessionId"] != "chat" {
		t.Fatal(options)
	}
}
