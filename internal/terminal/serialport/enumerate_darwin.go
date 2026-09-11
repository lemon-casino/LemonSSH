//go:build !js && darwin

package serialport

import "go.bug.st/serial"

// listPortInfos on macOS returns name-only entries: the detailed USB
// enumerator links IOKit through cgo, which the CGO_ENABLED=0 release packaging
// path cannot build. The renderer treats the empty USB fields as "unknown".
func listPortInfos() ([]Info, error) {
	native, err := serial.GetPortsList()
	if err != nil {
		return nil, err
	}
	infos := make([]Info, 0, len(native))
	for _, name := range native {
		infos = append(infos, Info{Name: name})
	}
	return infos, nil
}
