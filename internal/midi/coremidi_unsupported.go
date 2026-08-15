//go:build !darwin

package midi

import (
	"errors"
	"runtime"
)

func Destinations() ([]Destination, error) {
	return nil, errors.New("MIDI output is not implemented on " + runtime.GOOS + "; use macOS or contribute a backend")
}

func OpenOutput(destination Destination) (Output, error) {
	return nil, errors.New("MIDI output is not implemented on " + runtime.GOOS + "; use macOS or contribute a backend")
}
