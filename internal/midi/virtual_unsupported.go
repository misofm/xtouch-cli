//go:build !darwin

package midi

import (
	"fmt"
	"runtime"
)

func OpenVirtualDevice(name string) (VirtualDevice, error) {
	return nil, fmt.Errorf("virtual MIDI simulation is not implemented on %s", runtime.GOOS)
}
