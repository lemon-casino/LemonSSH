package monitoring

import "testing"

func TestDockerJSON(t *testing.T) {
	rows, err := ParseDocker("containers", "{\"ID\":\"abc\",\"Names\":\"web\",\"Image\":\"nginx\",\"Status\":\"Up\",\"State\":\"running\",\"Ports\":\"80/tcp\",\"CreatedAt\":\"today\"}\n")
	if err != nil || len(rows) != 1 || rows[0]["name"] != "web" {
		t.Fatalf("%v %v", rows, err)
	}
	for _, bad := range []string{"not json", "{}", "null", "{\"ID\":4}"} {
		if _, err := ParseDocker("containers", bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	if rows, err := ParseDocker("images", ""); err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	rows, err = ParseDocker("stats", `{"ID":"abc","Name":"web","CPUPerc":"12.5%","MemPerc":"2%","MemUsage":"2MiB / 1GiB","NetIO":"1kB / 2kB","BlockIO":"0B / 0B","PIDs":"3"}`)
	if err != nil || rows[0]["cpuPercent"] != 12.5 || rows[0]["pids"] != float64(3) {
		t.Fatal(rows, err)
	}
}
func TestProcessesAndTmux(t *testing.T) {
	rows, err := ParseProcesses(" 12 1 root S 1.5 2.0 1024 2048 01:02 /bin/a --hello world\n")
	if err != nil || rows[0]["command"] != "/bin/a --hello world" || rows[0]["pid"] != float64(12) {
		t.Fatal(rows, err)
	}
	if _, err := ParseProcesses("broken"); err == nil {
		t.Fatal("accepted malformed ps")
	}
	rows, err = ParseTmux("work\t2\t1\t1700000000\n")
	if err != nil || rows[0]["attached"] != true {
		t.Fatal(rows, err)
	}
	if _, err := ParseTmux("bad\tx\t0\t0"); err == nil {
		t.Fatal("accepted malformed tmux")
	}
}
func TestCapabilities(t *testing.T) {
	c, err := ParseCapabilities("Linux\nproc=1\nps=1\ntmux=0\ndocker=0\n")
	if err != nil || c["hasDocker"] != false || c["targetOs"] != "linux" {
		t.Fatal(c, err)
	}
	if _, err := ParseCapabilities("garbage"); err == nil {
		t.Fatal("accepted invalid probe")
	}
}
func TestStatsSamples(t *testing.T) {
	before := "cpu 10 0 10 80 0 0 0 0\ncpu0 10 0 10 80 0 0 0 0\n"
	after := "cpu 20 0 20 160 0 0 0 0\ncpu0 20 0 20 160 0 0 0 0\n"
	data := "===cpu1===\n" + before + "===net1===\neth0: 100 0 0 0 0 0 0 0 200 0 0 0 0 0 0 0\n===time1===\n10.0 0\n===cpu2===\n" + after + "===net2===\neth0: 300 0 0 0 0 0 0 0 600 0 0 0 0 0 0 0\n===time2===\n10.5 0\n===mem===\nMemTotal: 2048 kB\nMemAvailable: 1024 kB\nMemFree: 512 kB\nBuffers: 128 kB\nCached: 256 kB\nSwapTotal: 1024 kB\nSwapFree: 512 kB\n"
	s, err := ParseStats(data)
	if err != nil || s["cpu"] != float64(20) || s["memUsed"] != float64(1) || s["netRxSpeed"] != float64(400) {
		t.Fatal(s, err)
	}
	if _, err := ParseStats("garbage"); err == nil {
		t.Fatal("accepted invalid stats")
	}
}
