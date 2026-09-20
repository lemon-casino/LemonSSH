package terminaluse

import (
	"math"

	"github.com/binaricat/netcatty/internal/platform/monitoring"
	"strings"
	"testing"
)

func rowFloat(t *testing.T, value any) float64 {
	t.Helper()
	switch number := value.(type) {
	case *float64:
		if number == nil {
			t.Fatal("unexpected nil number")
		}
		return *number
	case float64:
		return number
	default:
		t.Fatalf("unexpected number type %T", value)
		return 0
	}
}

func TestParseAscendInfoTableCANN24(t *testing.T) {
	result := parseAcceleratorSnapshot(`
__NC_ACCEL_BEGIN__
__NC_NPU_BEGIN__
__NC_NPU_INFO__
| npu-smi 24.1.rc2                       Version: 24.1.rc2                                       |
| NPU   Name                | Health          | Power(W)    Temp(C)           Hugepages-Usage(page)|
| Chip                      | Bus-Id          | AICore(%)   Memory-Usage(MB)  HBM-Usage(MB)        |
| 6     910B1               | OK              | 100.8       33                0    / 0             |
| 0                         | 0000:01:00.0     | 0           0    / 0          3384 / 65536         |
__NC_NPU_PROCS__
__NC_NPU_END__
__NC_ACCEL_END__`)
	devices, ok := result.Devices.([]monitoring.Row)
	if !result.Success || !ok || len(devices) != 1 {
		t.Fatalf("unexpected snapshot: %+v", result)
	}
	device := devices[0]
	if device["vendor"] != "ascend" || device["index"] != 6 || device["name"] != "910B1" {
		t.Fatalf("unexpected device identity: %#v", device)
	}
	if rowFloat(t, device["memoryUsedMb"]) != 3384 || rowFloat(t, device["memoryTotalMb"]) != 65536 {
		t.Fatalf("unexpected HBM values: %#v", device)
	}
	if rowFloat(t, device["powerDrawW"]) != 100.8 || rowFloat(t, device["temperatureC"]) != 33 {
		t.Fatalf("unexpected telemetry: %#v", device)
	}
	if device["driverVersion"] != "24.1.rc2" {
		t.Fatalf("unexpected driver: %#v", device["driverVersion"])
	}
}

func TestParseAscendInfoTableAggregatesMultiChipRows(t *testing.T) {
	devices := parseAscendInfoTable(`
| npu-smi 25.2.0                   Version: 25.2.0                                               |
| NPU   Name                | Health        | Power(W)    Temp(C)           Hugepages-Usage(page)|
| Chip  Phy-ID              | Bus-Id        | AICore(%)   Memory-Usage(MB)  HBM-Usage(MB)        |
| 0     Ascend910           | OK            | 162.8       37                0    / 0             |
| 0     0                   | 0000:9C:00.0  | 0           0    / 0          3133 / 65536         |
| 0     Ascend910           | OK            | -           37                0    / 0             |
| 1     1                   | 0000:9E:00.0  | 0           0    / 0          2876 / 65536         |`)
	if len(devices) != 1 {
		t.Fatalf("unexpected devices: %#v", devices)
	}
	if devices[0].memoryUsedMB == nil || devices[0].memoryTotalMB == nil || *devices[0].memoryUsedMB != 6009 || *devices[0].memoryTotalMB != 131072 {
		t.Fatalf("multi-chip memory was not aggregated: %#v", devices[0])
	}
}

func TestParseAscendProcessesFromInfoTable(t *testing.T) {
	rows := parseAscendProcesses(`
| NPU Chip | Process id | Process name | Process memory(MB) |
| 1       0 | 3277562 | mindie_llm_back | 14513 |
| 1 0 3277562 mindie_llm_back 14513 |`)
	if len(rows) != 1 {
		t.Fatalf("duplicate process rows were not collapsed: %#v", rows)
	}
	if rows[0]["gpuIndex"] != 1 || rows[0]["pid"] != 3277562 || math.Abs(rowFloat(t, rows[0]["memoryUsedMb"])-14513) > 0.001 {
		t.Fatalf("unexpected process row: %#v", rows[0])
	}
}

func TestAcceleratorCollectorQueriesBothVendors(t *testing.T) {
	for _, required := range []string{"nvidia-smi --query-gpu", "npu-smi info -l", "npu-smi info -m", "npu-smi info -t proc-mem", "__NC_NPU_INFO__"} {
		if !strings.Contains(acceleratorCollectCommand, required) {
			t.Fatalf("collector is missing %q", required)
		}
	}
}
