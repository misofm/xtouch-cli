package main

import (
	"bytes"
	"encoding/json"
	"errors"
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

func TestSimulateDescribePublishesTheControlCatalog(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"simulate", "describe"}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	output := stdout.String()
	for _, expected := range []string{"xtouch.surface-description/v1", "transport.play", "fader.master", "display.rude-solo-led"} {
		if !strings.Contains(output, expected) {
			t.Errorf("description did not contain %q", expected)
		}
	}
}

func TestSimulationRunsAnNDJSONScenario(t *testing.T) {
	pressed := true
	_ = pressed
	script := strings.Join([]string{
		`{"id":"display","type":"host.midi","bytes":[240,0,0,102,20,18,0,86,111,99,97,108,115,32,247]}`,
		`{"id":"play","type":"user.button","control":"transport.play","pressed":true}`,
		`{"id":"fader","type":"user.fader","fader":"master","position":16383}`,
		`{"id":"state","type":"snapshot"}`,
	}, "\n")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run([]string{"simulate", "run"}, strings.NewReader(script), &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v\nstderr: %s", err, stderr.String())
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d responses: %s", len(lines), stdout.String())
	}
	var play struct {
		MIDI [][]int `json:"midi"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &play); err != nil {
		t.Fatal(err)
	}
	if len(play.MIDI) != 1 || len(play.MIDI[0]) != 3 || play.MIDI[0][1] != 0x5e {
		t.Fatalf("play MIDI = %#v", play.MIDI)
	}
	var snapshot struct {
		State struct {
			Master struct {
				Position int `json:"position"`
			} `json:"master"`
			Strips []struct {
				LCDUpper string `json:"lcdUpper"`
			} `json:"strips"`
		} `json:"state"`
	}
	if err := json.Unmarshal([]byte(lines[3]), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.State.Master.Position != 16383 || snapshot.State.Strips[0].LCDUpper != "Vocals " {
		t.Fatalf("snapshot = %#v", snapshot.State)
	}
}

func TestSimulationErrorResponsePreservesIDAndContext(t *testing.T) {
	response := newSimulationErrorResponse(
		json.RawMessage(`"bad-button"`),
		7,
		"user.button",
		errors.New("unknown control"),
	)
	if response.Type != "error" || string(response.ID) != `"bad-button"` {
		t.Fatalf("response = %#v", response)
	}
	if !strings.Contains(response.Error, "simulation line 7 (user.button)") {
		t.Fatalf("error = %q", response.Error)
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
