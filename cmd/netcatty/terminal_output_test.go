package main

import (
	"github.com/binaricat/netcatty/internal/terminal/dataplane"
	"testing"
)

func TestReconnectKeepsNativeSessionAndRotatesRoute(t *testing.T) {
	controller := dataplane.NewRouteController()
	dp := dataplane.NewServer(controller, "127.0.0.1:0")
	s := NewTerminalService(controller, dp, nil)
	old, _ := controller.Open("mosh")
	term := &terminalSession{bootstrap: old}
	s.sessions["mosh"] = term
	fresh, err := s.Reconnect("mosh")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Generation <= old.Generation || fresh.DataToken == old.DataToken {
		t.Fatal("route credentials not rotated")
	}
	got, _ := s.lookup("mosh")
	if got != term {
		t.Fatal("native session was replaced")
	}
}

func TestTerminalPublishFailureReportsAndCloses(t *testing.T) {
	controller := dataplane.NewRouteController()
	dp := dataplane.NewServer(controller, "127.0.0.1:0")
	service := NewTerminalService(controller, dp, nil)
	bootstrap, err := controller.Open("bounded")
	if err != nil {
		t.Fatal(err)
	}
	service.sessions["bounded"] = &terminalSession{bootstrap: bootstrap}
	reported := false
	service.setEventEmitter(func(name string, payload any) {
		if name == "terminal:error" {
			reported = true
		}
	})
	if !service.publishOutput("bounded", make([]byte, dataplane.ReceiveWindowBytes)) {
		t.Fatal("initial output rejected")
	}
	if service.publishOutput("bounded", []byte("overflow")) {
		t.Fatal("overflow accepted")
	}
	if !reported {
		t.Fatal("output rejection was not reported")
	}
	if _, ok := service.lookup("bounded"); ok {
		t.Fatal("failed session still live")
	}
}
