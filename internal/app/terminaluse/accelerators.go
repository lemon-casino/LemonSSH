package terminaluse

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/binaricat/netcatty/internal/platform/monitoring"
)

const acceleratorCollectCommand = `export LC_ALL=C
printf '%s\n' '__NC_ACCEL_BEGIN__'
if command -v nvidia-smi >/dev/null 2>&1; then
  printf '%s\n' '__NC_NVIDIA_DEVICES__'
  nvidia-smi --query-gpu=index,uuid,name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw,power.limit,fan.speed,driver_version --format=csv,noheader,nounits 2>/dev/null || true
  printf '%s\n' '__NC_NVIDIA_PROCESSES__'
  nvidia-smi --query-compute-apps=gpu_uuid,pid,process_name,used_gpu_memory --format=csv,noheader,nounits 2>/dev/null || true
fi
if command -v npu-smi >/dev/null 2>&1; then
  printf '%s\n' '__NC_NPU_BEGIN__'
  ids=$(npu-smi info -l 2>/dev/null | sed -n 's/^[[:space:]]*NPU ID[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p')
  if [ -z "$ids" ]; then
    ids=$(npu-smi info -m 2>/dev/null | awk 'NR>1 && $1 ~ /^[0-9]+$/ {print $1}' | sort -nu)
  fi
  if [ -n "$ids" ]; then
    for id in $ids; do
      printf '%s\n' "__NC_NPU_DEVICE__=$id"
      npu-smi info -t board -i "$id" 2>/dev/null || true
      npu-smi info -t common -i "$id" 2>/dev/null || true
      npu-smi info -t usages -i "$id" 2>/dev/null || true
      npu-smi info -t memory -i "$id" 2>/dev/null || true
    done
  fi
  printf '%s\n' '__NC_NPU_INFO__'
  npu-smi info 2>/dev/null || true
  printf '%s\n' '__NC_NPU_PROCS__'
  npu-smi info -t proc-mem 2>/dev/null || true
  printf '%s\n' '__NC_NPU_END__'
fi
printf '%s\n' '__NC_ACCEL_END__'`

type acceleratorDevice struct {
	vendor             string
	index              int
	uuid               string
	name               string
	utilizationPercent *float64
	memoryUsedMB       *float64
	memoryTotalMB      *float64
	temperatureC       *float64
	powerDrawW         *float64
	powerLimitW        *float64
	fanPercent         *float64
	driverVersion      string
	health             string
	chipCount          int
}

func (d acceleratorDevice) row() monitoring.Row {
	return monitoring.Row{
		"vendor": d.vendor, "index": d.index, "uuid": d.uuid, "name": d.name,
		"utilizationPercent": d.utilizationPercent, "memoryUsedMb": d.memoryUsedMB,
		"memoryTotalMb": d.memoryTotalMB, "temperatureC": d.temperatureC,
		"powerDrawW": d.powerDrawW, "powerLimitW": d.powerLimitW,
		"fanPercent": d.fanPercent, "driverVersion": nullableString(d.driverVersion),
		"health": nullableString(d.health),
	}
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func acceleratorNumber(value string) *float64 {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, ",", ""), "%", ""))
	if value == "" || value == "-" || strings.EqualFold(value, "N/A") || strings.EqualFold(value, "[N/A]") || strings.EqualFold(value, "not supported") {
		return nil
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	return &number
}

func maxAcceleratorNumber(current, next *float64) *float64 {
	if next == nil {
		return current
	}
	if current == nil || *next > *current {
		value := *next
		return &value
	}
	return current
}

func parseNvidiaAccelerators(section string) ([]acceleratorDevice, []monitoring.Row, string) {
	devices := []acceleratorDevice{}
	for _, rawLine := range strings.Split(section, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "__NC_") {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 11 {
			continue
		}
		for index := range parts {
			parts[index] = strings.TrimSpace(parts[index])
		}
		deviceIndex, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		devices = append(devices, acceleratorDevice{
			vendor: "nvidia", index: deviceIndex, uuid: parts[1], name: parts[2],
			utilizationPercent: acceleratorNumber(parts[3]), memoryUsedMB: acceleratorNumber(parts[4]),
			memoryTotalMB: acceleratorNumber(parts[5]), temperatureC: acceleratorNumber(parts[6]),
			powerDrawW: acceleratorNumber(parts[7]), powerLimitW: acceleratorNumber(parts[8]),
			fanPercent: acceleratorNumber(parts[9]), driverVersion: parts[10],
		})
	}
	rows := make([]monitoring.Row, 0, len(devices))
	driver := ""
	for _, device := range devices {
		rows = append(rows, device.row())
		if driver == "" {
			driver = device.driverVersion
		}
	}
	return devices, rows, driver
}

func parseNvidiaAcceleratorProcesses(section string, devices []acceleratorDevice) []monitoring.Row {
	byUUID := map[string]int{}
	for _, device := range devices {
		byUUID[device.uuid] = device.index
	}
	result := []monitoring.Row{}
	for _, rawLine := range strings.Split(section, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "__NC_") {
			continue
		}
		parts := strings.SplitN(line, ",", 4)
		if len(parts) < 4 {
			continue
		}
		for index := range parts {
			parts[index] = strings.TrimSpace(parts[index])
		}
		pid, err := strconv.Atoi(parts[1])
		if err != nil || pid <= 0 {
			continue
		}
		result = append(result, monitoring.Row{"vendor": "nvidia", "gpuIndex": byUUID[parts[0]], "pid": pid, "processName": parts[2], "memoryUsedMb": acceleratorNumber(parts[3])})
	}
	return result
}

func extractAcceleratorNumber(block string, labels ...string) *float64 {
	for _, label := range labels {
		match := regexp.MustCompile(`(?i)` + label + `\s*[:=]\s*([0-9]+(?:\.[0-9]+)?)`).FindStringSubmatch(block)
		if len(match) == 2 {
			return acceleratorNumber(match[1])
		}
	}
	return nil
}

func extractAcceleratorText(block string, labels ...string) string {
	for _, label := range labels {
		match := regexp.MustCompile(`(?i)` + label + `\s*[:=]\s*(.+)`).FindStringSubmatch(block)
		if len(match) == 2 {
			value := strings.TrimSpace(regexp.MustCompile(`\s{2,}.*$`).ReplaceAllString(match[1], ""))
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func parseAscendDeviceBlock(index int, block string) acceleratorDevice {
	device := acceleratorDevice{
		vendor: "ascend", index: index, name: fmt.Sprintf("Ascend NPU %d", index),
	}
	if name := extractAcceleratorText(block, "Product Name", "NPU Name", "Chip Name", "Model", "Board Name"); name != "" {
		device.name = name
	}
	device.utilizationPercent = extractAcceleratorNumber(block, `Aicore Usage Rate\(%\)`, `AICore Usage Rate\(%\)`, "Aicore Usage Rate", "AI Core Usage")
	device.memoryUsedMB = extractAcceleratorNumber(block, `HBM Used Memory\(MB\)`, `HBM Memory Usage\(MB\)`, `Used HBM Memory\(MB\)`, "Used HBM Memory")
	device.memoryTotalMB = extractAcceleratorNumber(block, `HBM Total Memory\(MB\)`, `HBM Capacity\(MB\)`, `Total HBM Memory\(MB\)`, "Total HBM Memory")
	usageRate := extractAcceleratorNumber(block, `HBM Usage Rate\(%\)`, "HBM Usage Rate")
	pairPattern := regexp.MustCompile(`(?i)HBM[^\n]*?(?:Memory|Usage)\([^\n]*?([0-9]+(?:\.[0-9]+)?)\s*/\s*([0-9]+(?:\.[0-9]+)?)`)
	pair := pairPattern.FindStringSubmatch(block)
	if len(pair) != 3 {
		pair = regexp.MustCompile(`(?i)HBM Used Memory[^\n]*?([0-9]+(?:\.[0-9]+)?)\s*/\s*([0-9]+(?:\.[0-9]+)?)`).FindStringSubmatch(block)
	}
	if len(pair) == 3 {
		device.memoryUsedMB, device.memoryTotalMB = acceleratorNumber(pair[1]), acceleratorNumber(pair[2])
	} else if device.memoryUsedMB == nil && usageRate != nil && device.memoryTotalMB != nil && *device.memoryTotalMB > 0 {
		value := (*usageRate / 100) * *device.memoryTotalMB
		device.memoryUsedMB = &value
	}
	device.temperatureC = extractAcceleratorNumber(block, `Temperature\(C\)`, `Temp\(C\)`, "Temperature")
	device.powerDrawW = extractAcceleratorNumber(block, `NPU Real-time Power\(W\)`, `Power Dissipation\(W\)`, `Power\(W\)`, "Power")
	device.health = extractAcceleratorText(block, "Health", "Health Status")
	return device
}

var (
	ascendVersionPattern    = regexp.MustCompile(`(?i)\|\s*npu-smi\s+(\S+)\s+.*?Version:\s*(\S+)`)
	ascendSummaryPattern    = regexp.MustCompile(`^\|\s*(\d+)\s+(\S+)\s+\|\s*(\S+)\s+\|\s*(\S+)\s+(\d+(?:\.\d+)?)\b`)
	ascendBusChipPattern    = regexp.MustCompile(`^\|\s*(\d+)\s*(\d*)\s*\|\s*([0-9A-Fa-f:.]+|NA)\s*\|\s*(\d+(?:\.\d+)?)\b`)
	ascendLegacyChipPattern = regexp.MustCompile(`^\|\s*(\d+)\s+\d+\s+\d+\s+(\d+(?:\.\d+)?)\s+(\d+(?:\.\d+)?)\s*/\s*(\d+(?:\.\d+)?)\s+(\d+(?:\.\d+)?)\s*/\s*(\d+(?:\.\d+)?)`)
	memoryPairPattern       = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*/\s*([0-9]+(?:\.[0-9]+)?)`)
)

func extractAscendDriverVersion(text string) string {
	match := ascendVersionPattern.FindStringSubmatch(text)
	if len(match) != 3 {
		return ""
	}
	version := strings.Trim(strings.TrimSpace(match[2]), "|")
	if version == "" {
		version = strings.Trim(strings.TrimSpace(match[1]), "|")
	}
	return version
}

func applyAcceleratorMemory(device *acceleratorDevice, used, total string, aggregate bool) {
	usedNumber, totalNumber := acceleratorNumber(used), acceleratorNumber(total)
	if usedNumber == nil && totalNumber == nil {
		return
	}
	if aggregate && device.memoryUsedMB != nil && usedNumber != nil {
		usedValue := *device.memoryUsedMB + *usedNumber
		totalValue := 0.0
		if device.memoryTotalMB != nil {
			totalValue += *device.memoryTotalMB
		}
		if totalNumber != nil {
			totalValue += *totalNumber
		}
		device.memoryUsedMB, device.memoryTotalMB = &usedValue, &totalValue
		return
	}
	if usedNumber != nil {
		device.memoryUsedMB = usedNumber
	}
	if totalNumber != nil {
		device.memoryTotalMB = totalNumber
	}
}

func applyAscendChipLine(device *acceleratorDevice, line string, utilization string, aggregate bool) {
	device.utilizationPercent = maxAcceleratorNumber(device.utilizationPercent, acceleratorNumber(utilization))
	pairs := memoryPairPattern.FindAllStringSubmatch(line, -1)
	if len(pairs) > 0 {
		pair := pairs[len(pairs)-1]
		applyAcceleratorMemory(device, pair[1], pair[2], aggregate)
	}
	device.chipCount++
}

func parseAscendInfoTable(text string) []acceleratorDevice {
	devices := []acceleratorDevice{}
	byIndex := map[int]int{}
	lines := strings.Split(text, "\n")
	driver := extractAscendDriverVersion(text)
	lastIndex := -1
	for lineIndex := 0; lineIndex < len(lines); lineIndex++ {
		line := strings.TrimSpace(lines[lineIndex])
		if !strings.HasPrefix(line, "|") {
			continue
		}
		if match := ascendSummaryPattern.FindStringSubmatch(line); len(match) == 6 {
			deviceIndex, _ := strconv.Atoi(match[1])
			name := strings.TrimSpace(match[2])
			if name == "" || regexp.MustCompile(`(?i)^(NPU|Chip|Name)$`).MatchString(name) {
				continue
			}
			position, exists := byIndex[deviceIndex]
			isNew := !exists
			if !exists {
				devices = append(devices, acceleratorDevice{vendor: "ascend", index: deviceIndex, name: name, temperatureC: acceleratorNumber(match[5]), powerDrawW: acceleratorNumber(match[4]), driverVersion: driver, health: strings.TrimSpace(match[3])})
				position = len(devices) - 1
				byIndex[deviceIndex] = position
			}
			device := &devices[position]
			if exists {
				device.temperatureC = maxAcceleratorNumber(device.temperatureC, acceleratorNumber(match[5]))
				if device.powerDrawW == nil {
					device.powerDrawW = acceleratorNumber(match[4])
				}
				if device.health == "" || strings.EqualFold(device.health, "ok") {
					device.health = strings.TrimSpace(match[3])
				}
			}
			lastIndex = position
			if lineIndex+1 < len(lines) {
				next := strings.TrimSpace(lines[lineIndex+1])
				if chip := ascendBusChipPattern.FindStringSubmatch(next); len(chip) == 5 {
					applyAscendChipLine(device, next, chip[4], !isNew && device.chipCount > 0)
					lineIndex++
					continue
				}
				if chip := ascendLegacyChipPattern.FindStringSubmatch(next); len(chip) == 7 {
					applyAcceleratorMemory(device, chip[5], chip[6], !isNew && device.chipCount > 0)
					device.utilizationPercent = maxAcceleratorNumber(device.utilizationPercent, acceleratorNumber(chip[2]))
					device.chipCount++
					lineIndex++
				}
			}
			continue
		}
		if chip := ascendLegacyChipPattern.FindStringSubmatch(line); len(chip) == 7 {
			deviceIndex, _ := strconv.Atoi(chip[1])
			position, exists := byIndex[deviceIndex]
			if !exists {
				position = lastIndex
			}
			if position >= 0 {
				device := &devices[position]
				applyAcceleratorMemory(device, chip[5], chip[6], device.chipCount > 0)
				device.utilizationPercent = maxAcceleratorNumber(device.utilizationPercent, acceleratorNumber(chip[2]))
				device.chipCount++
			}
			continue
		}
		if chip := ascendBusChipPattern.FindStringSubmatch(line); len(chip) == 5 && lastIndex >= 0 {
			device := &devices[lastIndex]
			applyAscendChipLine(device, line, chip[4], device.chipCount > 0)
		}
	}
	return devices
}

func parseAscendTypedSections(text string) []acceleratorDevice {
	chunks := strings.Split(text, "__NC_NPU_DEVICE__=")
	devices := []acceleratorDevice{}
	for _, chunk := range chunks[1:] {
		chunk = strings.TrimSpace(chunk)
		lineEnd := strings.IndexByte(chunk, '\n')
		idText, body := chunk, ""
		if lineEnd >= 0 {
			idText, body = chunk[:lineEnd], chunk[lineEnd+1:]
		}
		index, err := strconv.Atoi(strings.TrimSpace(idText))
		if err == nil {
			devices = append(devices, parseAscendDeviceBlock(index, body))
		}
	}
	return devices
}

var (
	ascendNamedProcessPattern      = regexp.MustCompile(`(?i)(?:NPU|Device)?\s*I?D?\s*[:=]?\s*(\d+).*?(?:PID|Pid)\s*[:=]?\s*(\d+).*?(?:Name|Process)\s*[:=]?\s*(\S+).*?(?:Memory|Mem)\s*[:=]?\s*(\d+(?:\.\d+)?)`)
	ascendInfoProcessPattern       = regexp.MustCompile(`^\|\s*(\d+)\s+(\d+)\s+\|\s+(\d+)\s+\|\s*([^|]+?)\s*\|\s*(\d+(?:\.\d+)?)`)
	ascendPipeProcessPattern       = regexp.MustCompile(`^\|\s*(\d+)\s*\|\s*\d+\s*\|\s*(\d+)\s*\|\s*([^|]+?)\s*\|\s*(\d+(?:\.\d+)?)`)
	ascendWhitespaceProcessPattern = regexp.MustCompile(`^\|\s*(\d+)\s+(\d+)\s+(\d+)\s+(\S+)\s+(\d+(?:\.\d+)?)\s*\|?\s*$`)
)

func parseAscendProcesses(text string) []monitoring.Row {
	result := []monitoring.Row{}
	seen := map[string]bool{}
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.Contains(strings.ToLower(line), "no running processes") {
			continue
		}
		var npu, pid, name, memory string
		switch {
		case len(ascendNamedProcessPattern.FindStringSubmatch(line)) == 5:
			match := ascendNamedProcessPattern.FindStringSubmatch(line)
			npu, pid, name, memory = match[1], match[2], match[3], match[4]
		case len(ascendInfoProcessPattern.FindStringSubmatch(line)) == 6:
			match := ascendInfoProcessPattern.FindStringSubmatch(line)
			npu, pid, name, memory = match[1], match[3], strings.TrimSpace(match[4]), match[5]
		case len(ascendPipeProcessPattern.FindStringSubmatch(line)) == 5:
			match := ascendPipeProcessPattern.FindStringSubmatch(line)
			npu, pid, name, memory = match[1], match[2], strings.TrimSpace(match[3]), match[4]
		case len(ascendWhitespaceProcessPattern.FindStringSubmatch(line)) == 6:
			match := ascendWhitespaceProcessPattern.FindStringSubmatch(line)
			npu, pid, name, memory = match[1], match[3], match[4], match[5]
		default:
			continue
		}
		npuIndex, npuErr := strconv.Atoi(npu)
		processID, pidErr := strconv.Atoi(pid)
		if npuErr != nil || pidErr != nil || processID <= 0 {
			continue
		}
		key := fmt.Sprintf("%d:%d:%s", npuIndex, processID, name)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, monitoring.Row{"vendor": "ascend", "gpuIndex": npuIndex, "pid": processID, "processName": name, "memoryUsedMb": acceleratorNumber(memory)})
	}
	return result
}

func markedSection(text, begin string, ends ...string) string {
	start := strings.Index(text, begin)
	if start < 0 {
		return ""
	}
	from := start + len(begin)
	end := len(text)
	for _, marker := range ends {
		if index := strings.Index(text[from:], marker); index >= 0 && from+index < end {
			end = from + index
		}
	}
	return text[from:end]
}

func mergeAscendDevices(typed, table []acceleratorDevice) []acceleratorDevice {
	if len(typed) == 0 {
		return table
	}
	byIndex := map[int]int{}
	for index := range typed {
		byIndex[typed[index].index] = index
	}
	for _, tableDevice := range table {
		index, ok := byIndex[tableDevice.index]
		if !ok {
			typed = append(typed, tableDevice)
			byIndex[tableDevice.index] = len(typed) - 1
			continue
		}
		device := &typed[index]
		if device.name == "" || strings.HasPrefix(device.name, "Ascend NPU ") {
			device.name = tableDevice.name
		}
		if device.utilizationPercent == nil {
			device.utilizationPercent = tableDevice.utilizationPercent
		}
		if device.memoryUsedMB == nil {
			device.memoryUsedMB = tableDevice.memoryUsedMB
		}
		if device.memoryTotalMB == nil {
			device.memoryTotalMB = tableDevice.memoryTotalMB
		}
		if device.temperatureC == nil {
			device.temperatureC = tableDevice.temperatureC
		}
		if device.powerDrawW == nil {
			device.powerDrawW = tableDevice.powerDrawW
		}
		if device.health == "" {
			device.health = tableDevice.health
		}
		if device.driverVersion == "" {
			device.driverVersion = tableDevice.driverVersion
		}
	}
	return typed
}

func parseAcceleratorSnapshot(text string) MonitoringResult {
	nvidiaDeviceText := markedSection(text, "__NC_NVIDIA_DEVICES__", "__NC_NVIDIA_PROCESSES__", "__NC_NPU_BEGIN__", "__NC_ACCEL_END__")
	nvidiaProcessText := markedSection(text, "__NC_NVIDIA_PROCESSES__", "__NC_NPU_BEGIN__", "__NC_ACCEL_END__")
	npuSection := markedSection(text, "__NC_NPU_BEGIN__", "__NC_NPU_END__", "__NC_ACCEL_END__")
	nvidiaDevices, deviceRows, driver := parseNvidiaAccelerators(nvidiaDeviceText)
	processRows := parseNvidiaAcceleratorProcesses(nvidiaProcessText, nvidiaDevices)
	infoDump := markedSection(npuSection, "__NC_NPU_INFO__", "__NC_NPU_PROCS__", "__NC_NPU_END__")
	ascendDevices := mergeAscendDevices(parseAscendTypedSections(npuSection), parseAscendInfoTable(firstNonEmpty(infoDump, npuSection)))
	ascendDriver := extractAscendDriverVersion(firstNonEmpty(infoDump, npuSection))
	for index := range ascendDevices {
		if ascendDevices[index].driverVersion == "" {
			ascendDevices[index].driverVersion = ascendDriver
		}
		deviceRows = append(deviceRows, ascendDevices[index].row())
	}
	ascendProcessText := markedSection(npuSection, "__NC_NPU_PROCS__", "__NC_NPU_END__")
	processRows = append(processRows, parseAscendProcesses(ascendProcessText)...)
	processRows = append(processRows, parseAscendProcesses(firstNonEmpty(infoDump, npuSection))...)
	processRows = dedupeAcceleratorRows(processRows)
	sort.Slice(deviceRows, func(i, j int) bool {
		left, right := fmt.Sprint(deviceRows[i]["vendor"]), fmt.Sprint(deviceRows[j]["vendor"])
		if left != right {
			return left < right
		}
		return deviceRows[i]["index"].(int) < deviceRows[j]["index"].(int)
	})
	return MonitoringResult{Success: true, Devices: deviceRows, Processes: processRows, NvidiaDriverVersion: driver, ProbedAt: time.Now().UnixMilli()}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func dedupeAcceleratorRows(rows []monitoring.Row) []monitoring.Row {
	seen := map[string]bool{}
	result := make([]monitoring.Row, 0, len(rows))
	for _, row := range rows {
		key := fmt.Sprintf("%v:%v:%v:%v", row["vendor"], row["gpuIndex"], row["pid"], row["processName"])
		if !seen[key] {
			seen[key] = true
			result = append(result, row)
		}
	}
	return result
}

func (s *Service) ListAccelerators(ctx context.Context, sessionID string) MonitoringResult {
	queryContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	text, err := s.monitoringExec(queryContext, sessionID, acceleratorCollectCommand)
	if err != nil {
		return monitoringFailure(err)
	}
	return parseAcceleratorSnapshot(text)
}
