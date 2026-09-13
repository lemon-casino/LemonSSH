// Package monitoring parses bounded, read-only host command output.
package monitoring

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

type Row = map[string]any

func number(s string) (float64, error) {
	n, e := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
	if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return 0, fmt.Errorf("invalid metric %q", s)
	}
	return n, nil
}
func lines(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(strings.TrimSpace(s), "\n")
}
func ParseDocker(kind, text string) ([]Row, error) {
	rows := []Row{}
	for _, line := range lines(text) {
		var raw map[string]string
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, fmt.Errorf("invalid docker JSON: %w", err)
		}
		if raw["ID"] == "" {
			return nil, fmt.Errorf("docker row missing ID")
		}
		row := Row{"id": raw["ID"]}
		var fields map[string]string
		switch kind {
		case "containers":
			fields = map[string]string{"name": "Names", "image": "Image", "status": "Status", "state": "State", "ports": "Ports", "createdAt": "CreatedAt"}
		case "images":
			fields = map[string]string{"repository": "Repository", "tag": "Tag", "size": "Size", "createdAt": "CreatedAt", "digest": "Digest"}
			row["name"] = raw["Repository"] + ":" + raw["Tag"]
		case "stats":
			fields = map[string]string{"name": "Name", "memUsage": "MemUsage", "netIO": "NetIO", "blockIO": "BlockIO"}
			for dst, src := range map[string]string{"cpuPercent": "CPUPerc", "memPercent": "MemPerc", "pids": "PIDs"} {
				n, err := number(raw[src])
				if err != nil {
					return nil, err
				}
				row[dst] = n
			}
		default:
			return nil, fmt.Errorf("unknown docker collection")
		}
		for dst, src := range fields {
			value, ok := raw[src]
			if !ok && dst != "digest" {
				return nil, fmt.Errorf("docker row missing %s", src)
			}
			row[dst] = value
		}
		rows = append(rows, row)
	}
	return rows, nil
}
func ParseProcesses(text string) ([]Row, error) {
	rows := []Row{}
	for _, line := range lines(text) {
		f := strings.Fields(line)
		if len(f) < 10 {
			return nil, fmt.Errorf("invalid ps row")
		}
		r := Row{"user": f[2], "stat": f[3], "elapsed": f[8], "command": strings.Join(f[9:], " ")}
		for i, k := range []string{"pid", "ppid", "", "", "cpuPercent", "memPercent", "rssKb", "vszKb"} {
			if k != "" {
				n, e := number(f[i])
				if e != nil {
					return nil, e
				}
				r[k] = n
			}
		}
		rows = append(rows, r)
	}
	return rows, nil
}
func ParseTmux(text string) ([]Row, error) {
	rows := []Row{}
	for _, line := range lines(text) {
		f := strings.Split(line, "\t")
		if len(f) != 4 || f[0] == "" {
			return nil, fmt.Errorf("invalid tmux row")
		}
		w, e := number(f[1])
		if e != nil {
			return nil, e
		}
		a, e := number(f[2])
		if e != nil {
			return nil, e
		}
		c, e := number(f[3])
		if e != nil {
			return nil, e
		}
		rows = append(rows, Row{"name": f[0], "windows": w, "attached": a > 0, "created": c})
	}
	return rows, nil
}
func ParseCapabilities(text string) (Row, error) {
	f := lines(text)
	if len(f) < 2 {
		return nil, fmt.Errorf("invalid capability probe")
	}
	os := map[string]string{"Linux": "linux", "Darwin": "darwin"}[f[0]]
	if os == "" {
		os = "unknown"
	}
	r := Row{"targetOs": os, "hasTmux": false, "hasDocker": false, "hasNvidiaSmi": false, "hasNpuSmi": false, "probedAt": time.Now().UnixMilli()}
	for _, line := range f[1:] {
		p := strings.Split(line, "=")
		if len(p) != 2 || (p[1] != "0" && p[1] != "1") {
			return nil, fmt.Errorf("invalid capability value")
		}
		if key := map[string]string{"tmux": "hasTmux", "docker": "hasDocker"}[p[0]]; key != "" {
			r[key] = p[1] == "1"
		}
	}
	return r, nil
}
func sections(text string) map[string]string {
	r := map[string]string{}
	key := ""
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "===") && strings.HasSuffix(line, "===") {
			key = strings.Trim(line, "=")
		} else if key != "" {
			r[key] += line + "\n"
		}
	}
	return r
}
func cpus(text string) (map[string][2]float64, error) {
	r := map[string][2]float64{}
	for _, l := range lines(text) {
		f := strings.Fields(l)
		if !strings.HasPrefix(f[0], "cpu") {
			continue
		}
		if len(f) < 5 {
			return nil, fmt.Errorf("invalid proc stat")
		}
		var total, idle float64
		for i := 1; i < len(f) && i <= 8; i++ {
			n, e := number(f[i])
			if e != nil {
				return nil, e
			}
			total += n
			if i == 4 || i == 5 {
				idle += n
			}
		}
		r[f[0]] = [2]float64{total, idle}
	}
	if len(r) == 0 {
		return nil, fmt.Errorf("CPU metrics unavailable")
	}
	return r, nil
}
func netCounters(text string) (map[string][2]float64, error) {
	r := map[string][2]float64{}
	for _, l := range lines(text) {
		p := strings.SplitN(l, ":", 2)
		if len(p) != 2 {
			continue
		}
		f := strings.Fields(p[1])
		if len(f) < 16 {
			return nil, fmt.Errorf("invalid network counters")
		}
		rx, e := number(f[0])
		if e != nil {
			return nil, e
		}
		tx, e := number(f[8])
		if e != nil {
			return nil, e
		}
		r[strings.TrimSpace(p[0])] = [2]float64{rx, tx}
	}
	return r, nil
}
func ParseStats(text string) (Row, error) {
	s := sections(text)
	a, e := cpus(s["cpu1"])
	if e != nil {
		return nil, e
	}
	b, e := cpus(s["cpu2"])
	if e != nil {
		return nil, e
	}
	r := Row{"cpu": nil, "cpuCores": len(b) - 1, "cpuPerCore": []float64{}, "topProcesses": []Row{}, "disks": []Row{}, "netInterfaces": []Row{}, "netRxSpeed": float64(0), "netTxSpeed": float64(0), "latencyMs": nil, "diskPercent": nil, "diskUsed": nil, "diskTotal": nil, "loadAverage": []float64{}, "uptimeSeconds": nil, "lastUpdated": time.Now().UnixMilli()}
	usage := func(k string) (float64, error) {
		x, ok := a[k]
		y := b[k]
		d := y[0] - x[0]
		idle := y[1] - x[1]
		if !ok || d <= 0 || idle < 0 || idle > d {
			return 0, fmt.Errorf("invalid CPU delta")
		}
		return 100 * (d - idle) / d, nil
	}
	cpu, e := usage("cpu")
	if e != nil {
		return nil, e
	}
	r["cpu"] = cpu
	cores := []float64{}
	for i := 0; i < len(b)-1; i++ {
		n, e := usage(fmt.Sprintf("cpu%d", i))
		if e != nil {
			return nil, e
		}
		cores = append(cores, n)
	}
	r["cpuPerCore"] = cores
	mem := map[string]float64{}
	for _, l := range lines(s["mem"]) {
		f := strings.Fields(l)
		if len(f) < 2 {
			return nil, fmt.Errorf("invalid meminfo")
		}
		n, e := number(f[1])
		if e != nil {
			return nil, e
		}
		mem[strings.TrimSuffix(f[0], ":")] = n / 1024
	}
	for dst, src := range map[string]string{"memTotal": "MemTotal", "memFree": "MemFree", "memBuffers": "Buffers", "memCached": "Cached", "swapTotal": "SwapTotal"} {
		if n, ok := mem[src]; ok {
			r[dst] = n
		} else {
			r[dst] = nil
		}
	}
	r["memUsed"] = nil
	r["swapUsed"] = nil
	if t, ok := mem["MemTotal"]; ok {
		if av, ok := mem["MemAvailable"]; ok && av <= t {
			r["memUsed"] = t - av
		}
	}
	if t, ok := mem["SwapTotal"]; ok {
		if f, ok := mem["SwapFree"]; ok && f <= t {
			r["swapUsed"] = t - f
		}
	}
	t1 := strings.Fields(s["time1"])
	t2 := strings.Fields(s["time2"])
	if len(t1) == 0 || len(t2) == 0 {
		return nil, fmt.Errorf("uptime unavailable")
	}
	start, e := number(t1[0])
	if e != nil {
		return nil, e
	}
	end, e := number(t2[0])
	if e != nil || end <= start {
		return nil, fmt.Errorf("invalid sampling interval")
	}
	r["uptimeSeconds"] = end
	n1, e := netCounters(s["net1"])
	if e != nil {
		return nil, e
	}
	n2, e := netCounters(s["net2"])
	if e != nil {
		return nil, e
	}
	nets := []Row{}
	var rx, tx float64
	for name, now := range n2 {
		old, ok := n1[name]
		if !ok || name == "lo" {
			continue
		}
		if now[0] < old[0] || now[1] < old[1] {
			continue
		}
		rs, ts := (now[0]-old[0])/(end-start), (now[1]-old[1])/(end-start)
		rx += rs
		tx += ts
		nets = append(nets, Row{"name": name, "rxBytes": now[0], "txBytes": now[1], "rxSpeed": rs, "txSpeed": ts})
	}
	r["netInterfaces"] = nets
	r["netRxSpeed"] = rx
	r["netTxSpeed"] = tx
	for key, section := range map[string]string{"hostname": "hostname", "kernelRelease": "kernel", "osName": "os"} {
		if v := strings.TrimSpace(s[section]); v != "" {
			r[key] = v
		}
	}
	if f := strings.Fields(s["load"]); len(f) >= 3 {
		load := []float64{}
		for _, v := range f[:3] {
			n, e := number(v)
			if e != nil {
				return nil, e
			}
			load = append(load, n)
		}
		r["loadAverage"] = load
	}
	if strings.TrimSpace(s["ps"]) != "" {
		ps, e := ParseProcesses(s["ps"])
		if e != nil {
			return nil, e
		}
		top := []Row{}
		for i, p := range ps {
			if i == 10 {
				break
			}
			top = append(top, Row{"pid": fmt.Sprint(p["pid"]), "memPercent": p["memPercent"], "command": p["command"]})
		}
		r["topProcesses"] = top
	}
	disks := []Row{}
	for i, l := range lines(s["df"]) {
		if i == 0 {
			continue
		}
		f := strings.Fields(l)
		if len(f) < 7 {
			return nil, fmt.Errorf("invalid df row")
		}
		total, e := number(f[2])
		if e != nil {
			return nil, e
		}
		used, e := number(f[3])
		if e != nil {
			return nil, e
		}
		pct, e := number(f[5])
		if e != nil {
			return nil, e
		}
		mount := strings.Join(f[6:], " ")
		disk := Row{"capacityKey": f[0], "filesystemType": f[1], "mountPoint": mount, "used": used / 1048576, "total": total / 1048576, "percent": pct}
		disks = append(disks, disk)
		if mount == "/" {
			r["diskPercent"] = pct
			r["diskUsed"] = used / 1048576
			r["diskTotal"] = total / 1048576
		}
	}
	r["disks"] = disks
	return r, nil
}
