package mosh

import (
	"reflect"
	"testing"
)

func TestNativeClientLaunchKeepsKeyOutOfArgv(t *testing.T) {
	args, env, err := ClientLaunch("mosh", "server", "user", 22, Connect{Port: 60001, Key: "session-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args, []string{"server", "60001"}) || env["MOSH_KEY"] != "session-secret" {
		t.Fatalf("launch %v %v", args, env)
	}
}
func TestETLaunchUsesOwnProtocol(t *testing.T) {
	args, env, err := ClientLaunch("et", "server", "user", 2222, Connect{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args, []string{"user@server", "--ssh-port", "2222"}) || len(env) != 0 {
		t.Fatalf("et launch %v %v", args, env)
	}
}
