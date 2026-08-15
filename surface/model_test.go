package surface

import (
	"reflect"
	"testing"
)

func TestControlCatalogCoversThePhysicalMCUSurface(t *testing.T) {
	definitions := ButtonDefinitions()
	byName := make(map[string]ButtonDefinition, len(definitions))
	for _, definition := range definitions {
		if _, exists := byName[definition.Name]; exists {
			t.Fatalf("duplicate control name %q", definition.Name)
		}
		byName[definition.Name] = definition
	}

	wantNotes := map[string]uint8{
		"channel.1.rec":         0x00,
		"channel.8.select":      0x1f,
		"encoder.1.press":       0x20,
		"assign.eq":             0x2c,
		"bank.right":            0x2f,
		"function.f8":           0x3d,
		"automation.touch":      0x4d,
		"transport.play":        0x5e,
		"navigation.scrub":      0x65,
		"footswitch.2":          0x67,
		"fader.master.touch":    0x70,
		"display.rude-solo-led": 0x73,
	}
	for name, note := range wantNotes {
		definition, ok := byName[name]
		if !ok {
			t.Errorf("control catalog is missing %q", name)
			continue
		}
		if definition.Note != note {
			t.Errorf("%s note = %#x, want %#x", name, definition.Note, note)
		}
	}
	if byName["encoder.1.press"].LED {
		t.Error("encoder presses are not illuminated on the X-Touch")
	}
	if byName["display.smpte-led"].Input {
		t.Error("SMPTE LED must be host-output only")
	}
}

func TestUserActionsEncodeDeviceToHostMessages(t *testing.T) {
	model := NewModel("V1.25")

	button, err := model.Button("transport.play", true)
	if err != nil {
		t.Fatal(err)
	}
	assertMessages(t, button.Outbound, [][]byte{{0x90, 0x5e, 0x7f}})

	release, err := model.Button("transport.play", false)
	if err != nil {
		t.Fatal(err)
	}
	assertMessages(t, release.Outbound, [][]byte{{0x90, 0x5e, 0x00}})

	fader, err := model.FaderMove(8, FaderMax)
	if err != nil {
		t.Fatal(err)
	}
	assertMessages(t, fader.Outbound, [][]byte{{0xe8, 0x7f, 0x7f}})

	touch, err := model.FaderTouch(0, true)
	if err != nil {
		t.Fatal(err)
	}
	assertMessages(t, touch.Outbound, [][]byte{{0x90, 0x68, 0x7f}})

	clockwise, err := model.EncoderTurn(7, 3)
	if err != nil {
		t.Fatal(err)
	}
	assertMessages(t, clockwise.Outbound, [][]byte{{0xb0, 0x17, 0x03}})

	counterClockwise, err := model.Jog(-2)
	if err != nil {
		t.Fatal(err)
	}
	assertMessages(t, counterClockwise.Outbound, [][]byte{{0xb0, 0x3c, 0x42}})

	expression, err := model.Expression(64)
	if err != nil {
		t.Fatal(err)
	}
	assertMessages(t, expression.Outbound, [][]byte{{0xb0, 0x2e, 0x40}})
	if model.Snapshot().Expression != 64 {
		t.Fatal("expression state was not retained")
	}
}

func TestHostMessagesUpdateEveryFeedbackFamily(t *testing.T) {
	model := NewModel("V1.25")
	messages := [][]byte{
		{0xe0, 0x00, 0x40},
		{0x90, 0x10, 0x7f},
		{0x90, 0x08, 0x01},
		{0xb0, 0x30, 0x66},
		{0xd0, 0x0a},
		{0xd0, 0x0e},
		{0xb0, 0x40, 0x71},
		{0xbf, 0x4b, 0x32},
		{0xf0, 0x00, 0x00, 0x66, 0x14, 0x12, 0x00, 'V', 'o', 'c', 'a', 'l', 's', ' ', 0xf7},
		{0xf0, 0x00, 0x00, 0x66, 0x14, 0x12, 0x38, '-', '6', '.', '0', 'd', 'B', ' ', 0xf7},
		{0xf0, 0x00, 0x00, 0x66, 0x14, 0x72, 6, 0, 0, 0, 0, 0, 0, 0, 0xf7},
	}
	for _, message := range messages {
		if _, err := model.ApplyHostMIDI(message); err != nil {
			t.Fatalf("ApplyHostMIDI(% x): %v", message, err)
		}
	}

	snapshot := model.Snapshot()
	if snapshot.Strips[0].Fader.Position != 8192 {
		t.Errorf("fader position = %d", snapshot.Strips[0].Fader.Position)
	}
	if snapshot.Buttons["channel.1.mute"].LED != LEDSolid {
		t.Errorf("mute LED = %q", snapshot.Buttons["channel.1.mute"].LED)
	}
	if snapshot.Buttons["channel.1.solo"].LED != LEDBlink {
		t.Errorf("solo LED = %q", snapshot.Buttons["channel.1.solo"].LED)
	}
	if snapshot.Strips[0].Ring.Mode != RingWrap || snapshot.Strips[0].Ring.Position != 6 || !snapshot.Strips[0].Ring.CenterLED {
		t.Errorf("ring = %#v", snapshot.Strips[0].Ring)
	}
	if snapshot.Strips[0].Meter.Level != 10 || !snapshot.Strips[0].Meter.Overload {
		t.Errorf("meter = %#v", snapshot.Strips[0].Meter)
	}
	if snapshot.Strips[0].LCDUpper != "Vocals " || snapshot.Strips[0].LCDLower != "-6.0dB " {
		t.Errorf("LCD = %q / %q", snapshot.Strips[0].LCDUpper, snapshot.Strips[0].LCDLower)
	}
	if snapshot.Strips[0].Color != ColorCyan {
		t.Errorf("color = %q", snapshot.Strips[0].Color)
	}
	if snapshot.Time[9].Character != "1" || !snapshot.Time[9].Dot {
		t.Errorf("rightmost time digit = %#v", snapshot.Time[9])
	}
	if snapshot.Assignment[0].Character != "2" {
		t.Errorf("left assignment digit = %#v", snapshot.Assignment[0])
	}
}

func TestTouchedFaderSuppressesUnsafeMotorFeedback(t *testing.T) {
	model := NewModel("V1.25")
	if _, err := model.FaderTouch(0, true); err != nil {
		t.Fatal(err)
	}
	result, err := model.ApplyHostMIDI([]byte{0xe0, 0x7f, 0x7f})
	if err != nil {
		t.Fatal(err)
	}
	if result.Warning == "" {
		t.Fatal("touched fader motor command did not produce a warning")
	}
	if model.Snapshot().Strips[0].Fader.Position != 0 {
		t.Fatal("touched fader moved in the simulator")
	}
}

func TestFirmwareIdentityAndConfigurationSysEx(t *testing.T) {
	model := NewModel("V1.25")
	identity, err := model.ApplyHostMIDI([]byte{0xf0, 0x00, 0x00, 0x66, 0x14, 0x13, 0x00, 0xf7})
	if err != nil {
		t.Fatal(err)
	}
	assertMessages(t, identity.Outbound, [][]byte{{0xf0, 0x00, 0x00, 0x66, 0x14, 0x14, 'V', '1', '.', '2', '5', 0xf7}})

	configuration := [][]byte{
		{0xf0, 0x00, 0x00, 0x66, 0x14, 0x0a, 0x00, 0xf7},
		{0xf0, 0x00, 0x00, 0x66, 0x14, 0x0b, 0x1e, 0xf7},
		{0xf0, 0x00, 0x00, 0x66, 0x14, 0x0c, 0x01, 0xf7},
		{0xf0, 0x00, 0x00, 0x66, 0x14, 0x0e, 0x08, 0x05, 0xf7},
		{0xf0, 0x00, 0x00, 0x66, 0x14, 0x20, 0x00, 0x07, 0xf7},
		{0xf0, 0x00, 0x00, 0x66, 0x14, 0x21, 0x01, 0xf7},
	}
	for _, message := range configuration {
		if _, err := model.ApplyHostMIDI(message); err != nil {
			t.Fatal(err)
		}
	}
	config := model.Snapshot().Config
	if config.TransportClick || config.BacklightSaverMinutes != 30 || !config.TouchlessFaders || config.FaderSensitivity[8] != 5 || config.MeterModes[0] != 7 || config.LCDMeterOrientation != "vertical" {
		t.Fatalf("configuration = %#v", config)
	}

	if _, err := model.ApplyHostMIDI([]byte{0xf0, 0x00, 0x00, 0x66, 0x14, 0x18, 0x00, 0xf7}); err == nil {
		t.Fatal("simulation accepted firmware-update SysEx")
	}
}

func TestMeterDecayRequiresSustainedLevelsToBeRefreshed(t *testing.T) {
	model := NewModel("V1.25")
	if _, err := model.ApplyHostMIDI([]byte{0xd0, 0x0a}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.Advance(599); err != nil {
		t.Fatal(err)
	}
	if got := model.Snapshot().Strips[0].Meter.Level; got != 9 {
		t.Fatalf("meter after 599 ms = %d, want 9", got)
	}

	// Re-sending an unchanged non-zero level refreshes the hardware decay timer.
	if _, err := model.ApplyHostMIDI([]byte{0xd0, 0x0a}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.Advance(299); err != nil {
		t.Fatal(err)
	}
	if got := model.Snapshot().Strips[0].Meter.Level; got != 10 {
		t.Fatalf("refreshed meter after 299 ms = %d, want 10", got)
	}
	if _, err := model.Advance(1); err != nil {
		t.Fatal(err)
	}
	if got := model.Snapshot().Strips[0].Meter.Level; got != 9 {
		t.Fatalf("refreshed meter after 300 ms = %d, want 9", got)
	}
	if model.Snapshot().ClockMS != 899 {
		t.Fatalf("clock = %d", model.Snapshot().ClockMS)
	}
}

func assertMessages(t *testing.T, got, want [][]byte) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = % x, want % x", got, want)
	}
}
