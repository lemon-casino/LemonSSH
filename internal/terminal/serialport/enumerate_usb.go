//go:build !js && !darwin

package serialport

import (
	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// listPortInfos uses the detailed USB enumerator, which is cgo-free on Linux
// and Windows. macOS needs cgo for IOKit, so it uses the name-only fallback.
func listPortInfos() ([]Info, error) {
	detailed, err := enumerator.GetDetailedPortsList()
	if err != nil {
		// Some drivers/OS builds cannot report details; the plain list still
		// gives a usable port picker, so degrade rather than fail the panel.
		return listPortNames()
	}
	infos := make([]Info, 0, len(detailed))
	for _, port := range detailed {
		if port == nil {
			continue
		}
		infos = append(infos, Info{
			Name:         port.Name,
			Manufacturer: port.Product,
			SerialNumber: port.SerialNumber,
			VendorID:     port.VID,
			ProductID:    port.PID,
			PNPID:        port.Name,
		})
	}
	return infos, nil
}

func listPortNames() ([]Info, error) {
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
