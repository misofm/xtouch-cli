package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/misofm/xtouch-cli/internal/midi"
)

func TestSelectDestinationPrefersExplicitIndex(t *testing.T) {
	destinations := []midi.Destination{
		{Index: 0, UniqueID: 100, Name: "X-TOUCH"},
		{Index: 1, UniqueID: 101, Name: "X-TOUCH"},
	}
	destination, err := selectDestination(destinations, "1")
	if err != nil {
		t.Fatalf("selectDestination() error = %v", err)
	}
	if destination.UniqueID != 101 {
		t.Fatalf("selected unique ID %d", destination.UniqueID)
	}
}

func TestSelectDestinationRejectsAmbiguousName(t *testing.T) {
	destinations := []midi.Destination{
		{Index: 0, Name: "X-TOUCH"},
		{Index: 1, Name: "X-TOUCH"},
	}
	if _, err := selectDestination(destinations, "x-touch"); err == nil {
		t.Fatal("selectDestination() accepted an ambiguous name")
	}
}

func TestLooksLikeXTouchUsesAllMetadata(t *testing.T) {
	if !looksLikeXTouch(midi.Destination{Name: "Update"}) {
		t.Fatal("update endpoint was not recognized")
	}
	if !looksLikeXTouch(midi.Destination{Manufacturer: "Behringer", Model: "X-Touch"}) {
		t.Fatal("X-Touch model metadata was not recognized")
	}
	if looksLikeXTouch(midi.Destination{Name: "Network Session 1"}) {
		t.Fatal("unrelated endpoint was recognized as X-Touch")
	}
}

func TestFirmwareTrustedCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"firmware", "trusted"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "d17daae1e7973f9e2eca24fe57a0a4aa705ab121c6ff33ccd29548cc2ee68b8c") {
		t.Fatalf("trusted output did not contain the 1.25 hash: %s", stdout.String())
	}
}
