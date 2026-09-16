package terminaluse

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/binaricat/netcatty/internal/terminal/dataplane"
)

func TestTerminalCwdProbeSelectsOnlyExactForegroundShell(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	cases := []struct{ name, rows, want string }{
		{"foreground nested shell", "100 10 pts/1 S bash\n200 100 pts/1 S sudo\n201 200 pts/1 S+ zsh\n300 100 pts/1 S bash\n400 99 pts/2 S+ bash", "/srv/a b'\u76ee\u5f55\n"},
		{"ambiguous sibling", "100 10 pts/1 S+ bash\n101 10 pts/2 S+ bash", ""},
		{"foreign connection", "400 99 pts/2 S+ bash", ""},
		{"foreground TUI", "100 10 pts/1 S bash\n200 100 pts/1 R+ vim", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
			// Replace OS edges, but execute the entire production probe and its awk
			// selection. The fake readlink rejects every PID except the active shell.
			setup := fmt.Sprintf("ps() { printf '%%s 10 ? S sh\\n' \"$$\"; printf '%%s\\n' %s; }; readlink() { [ \"$1\" = /proc/201/cwd ] || return 1; printf '%%s\\n' %s; };\n", quote(tc.rows), quote("/srv/a b'\u76ee\u5f55"))
			out, err := exec.Command(shell, "-c", setup+terminalCwdProbe).Output()
			if string(out) != tc.want {
				t.Fatalf("cwd = %q; want %q (%v)", out, tc.want, err)
			}
			if tc.want != "" && err != nil {
				t.Fatal(err)
			}
			if tc.want == "" && err == nil {
				t.Fatal("ambiguous/unavailable directory must fail")
			}
		})
	}
}

func TestTerminalCwdSplitOSCAndIsolation(t *testing.T) {
	c := dataplane.NewRouteController()
	s := New(c, dataplane.NewServer(c, "127.0.0.1:0"), nil)
	for _, id := range []string{"a", "b"} {
		boot, _ := c.Open(id)
		s.sessions[id] = &terminalSession{bootstrap: boot}
	}
	wire := []byte("prompt\x1b]7;file://host/srv/a%20b%27%E7%9B%AE%E5%BD%95\x1b\\")
	for _, b := range wire {
		s.publishOutput("a", []byte{b})
	}
	got := s.GetSessionPwd("a", TerminalPwdOptions{})
	if !got.Success || got.Cwd != "/srv/a b'\u76ee\u5f55" {
		t.Fatalf("cwd = %+v", got)
	}
	if s.GetSessionPwd("b", TerminalPwdOptions{}).Success {
		t.Fatal("cwd leaked to another session")
	}
	s.publishOutput("a", []byte("\x1b]7;/new path\a"))
	if got := s.GetSessionPwd("a", TerminalPwdOptions{}); got.Cwd != "/new path" {
		t.Fatalf("cwd = %+v", got)
	}
	_ = s.Close("a")
	if s.GetSessionPwd("a", TerminalPwdOptions{}).Success {
		t.Fatal("closed session retained cwd")
	}
}
