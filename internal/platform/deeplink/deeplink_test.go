package deeplink

import (
	"sync"
	"testing"
)

func TestParseAcceptsKnownSchemes(t *testing.T) {
	cases := []struct {
		raw, kind, host, port, user string
	}{
		// The current app scheme and the legacy pre-rename scheme must parse
		// identically: OS registrations of netcatty:// stay functional.
		{"lemonssh://ssh/user@example.com:2222", "ssh", "example.com", "2222", "user"},
		{"netcatty://ssh/user@example.com:2222", "ssh", "example.com", "2222", "user"},
		{"lemonssh://telnet/router.local:23", "telnet", "router.local", "23", ""},
		{"netcatty://telnet/router.local:23", "telnet", "router.local", "23", ""},
		{"ssh://example.com", "ssh", "example.com", "", ""},
		{"telnet://router.local:23", "telnet", "router.local", "23", ""},
		{"jms://jump.example.com/token", "jms", "", "", ""},
	}
	for _, testCase := range cases {
		action, err := Parse(testCase.raw)
		if err != nil {
			t.Fatalf("%s: %v", testCase.raw, err)
		}
		if testCase.kind == "jms" {
			if action.Kind != "jms" || action.URL != testCase.raw {
				t.Fatalf("%s: %+v", testCase.raw, action)
			}
			continue
		}
		if action.Kind != testCase.kind || action.Host != testCase.host || action.Port != testCase.port || action.Username != testCase.user {
			t.Fatalf("%s: %+v", testCase.raw, action)
		}
	}
}

func TestParseRejectsMalformedAndUnsafe(t *testing.T) {
	bad := []string{
		"",
		"  ",
		"http://example.com",
		"lemonssh://",
		"netcatty://",
		"ssh://user@example.com?password=secret",
		"ssh://us er@example.com",
	}
	for _, raw := range bad {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("malformed intent accepted: %q", raw)
		}
	}
}

func TestQueueBuffersAndFlushesInOrder(t *testing.T) {
	queue := NewQueue()
	if err := queue.Enqueue("lemonssh://ssh/first.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := queue.Enqueue("lemonssh://ssh/second.example.com"); err != nil {
		t.Fatal(err)
	}
	if queue.Pending() != 2 {
		t.Fatalf("pending mismatch: %d", queue.Pending())
	}
	var delivered []string
	queue.Ready(func(action *Action) { delivered = append(delivered, action.Host) })
	if len(delivered) != 2 || delivered[0] != "first.example.com" || delivered[1] != "second.example.com" {
		t.Fatalf("delivery mismatch: %v", delivered)
	}
	if queue.Pending() != 0 {
		t.Fatal("queue must drain")
	}
}

func TestQueueDeduplicatesIdenticalIntents(t *testing.T) {
	queue := NewQueue()
	_ = queue.Enqueue("lemonssh://ssh/dup.example.com")
	_ = queue.Enqueue("lemonssh://ssh/dup.example.com")
	queue.Ready(func(*Action) {})
	count := 0
	queue.Ready(func(*Action) { count++ })
	if count != 0 {
		t.Fatalf("post-ready enqueue expected, got %d", count)
	}
	if err := queue.Enqueue("lemonssh://ssh/dup.example.com"); err != nil {
		t.Fatal(err)
	}
	if queue.Pending() != 0 {
		t.Fatal("dedupe must suppress identical pre-ready duplicates")
	}
}

func TestConcurrentEnqueueIsSafe(t *testing.T) {
	queue := NewQueue()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = queue.Enqueue("lemonssh://ssh/host" + string(rune('a'+n)) + ".example.com")
		}(i)
	}
	wg.Wait()
	if queue.Pending() != 16 {
		t.Fatalf("expected 16 pending, got %d", queue.Pending())
	}
}
