// Package surface models the MIDI-facing state and physical interactions of a
// full-size Behringer X-Touch running in Mackie Control (MC) mode.
package surface

import "fmt"

const (
	ChannelCount = 8
	FaderCount   = 9
	FaderMax     = 0x3fff
)

type Confidence string

const (
	ConfidenceHigh          Confidence = "high"
	ConfidenceNeedsHardware Confidence = "needs-hardware-validation"
)

type ButtonDefinition struct {
	Name       string     `json:"name"`
	Note       uint8      `json:"note"`
	Input      bool       `json:"input"`
	LED        bool       `json:"led"`
	Confidence Confidence `json:"confidence"`
}

type ControlDefinition struct {
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Direction  string     `json:"direction"`
	Channel    int        `json:"channel,omitempty"`
	Number     int        `json:"number,omitempty"`
	Minimum    int        `json:"minimum,omitempty"`
	Maximum    int        `json:"maximum,omitempty"`
	Confidence Confidence `json:"confidence"`
}

var buttonDefinitions = buildButtonDefinitions()
var buttonsByName = indexButtonsByName(buttonDefinitions)
var buttonsByNote = indexButtonsByNote(buttonDefinitions)

func buildButtonDefinitions() []ButtonDefinition {
	buttons := make([]ButtonDefinition, 0, 108)
	add := func(name string, note uint8, input, led bool, confidence Confidence) {
		buttons = append(buttons, ButtonDefinition{
			Name: name, Note: note, Input: input, LED: led, Confidence: confidence,
		})
	}

	for channel := 1; channel <= ChannelCount; channel++ {
		add(fmt.Sprintf("channel.%d.rec", channel), uint8(channel-1), true, true, ConfidenceHigh)
		add(fmt.Sprintf("channel.%d.solo", channel), uint8(0x08+channel-1), true, true, ConfidenceHigh)
		add(fmt.Sprintf("channel.%d.mute", channel), uint8(0x10+channel-1), true, true, ConfidenceHigh)
		add(fmt.Sprintf("channel.%d.select", channel), uint8(0x18+channel-1), true, true, ConfidenceHigh)
		add(fmt.Sprintf("encoder.%d.press", channel), uint8(0x20+channel-1), true, false, ConfidenceHigh)
	}

	functionButtons := []struct {
		name string
		note uint8
	}{
		{"assign.track", 0x28}, {"assign.send", 0x29}, {"assign.pan-surround", 0x2a},
		{"assign.plugin", 0x2b}, {"assign.eq", 0x2c}, {"assign.instrument", 0x2d},
		{"bank.left", 0x2e}, {"bank.right", 0x2f}, {"channel.left", 0x30},
		{"channel.right", 0x31}, {"flip", 0x32}, {"global", 0x33},
		{"display.name-value", 0x34}, {"display.smpte-beats", 0x35},
		{"function.f1", 0x36}, {"function.f2", 0x37}, {"function.f3", 0x38},
		{"function.f4", 0x39}, {"function.f5", 0x3a}, {"function.f6", 0x3b},
		{"function.f7", 0x3c}, {"function.f8", 0x3d},
		{"view.midi-tracks", 0x3e}, {"view.inputs", 0x3f}, {"view.audio-tracks", 0x40},
		{"view.audio-instruments", 0x41}, {"view.aux", 0x42}, {"view.buses", 0x43},
		{"view.outputs", 0x44}, {"view.user", 0x45},
		{"modifier.shift", 0x46}, {"modifier.option", 0x47},
		{"modifier.control", 0x48}, {"modifier.alt", 0x49},
		{"automation.read-off", 0x4a}, {"automation.write", 0x4b},
		{"automation.trim", 0x4c}, {"automation.touch", 0x4d},
		{"automation.latch", 0x4e}, {"automation.group", 0x4f},
		{"utility.save", 0x50}, {"utility.undo", 0x51},
		{"utility.cancel", 0x52}, {"utility.enter", 0x53},
		{"navigation.markers", 0x54}, {"navigation.nudge", 0x55},
		{"transport.cycle", 0x56}, {"transport.drop", 0x57},
		{"transport.replace", 0x58}, {"transport.click", 0x59},
		{"transport.solo", 0x5a}, {"transport.rewind", 0x5b},
		{"transport.fast-forward", 0x5c}, {"transport.stop", 0x5d},
		{"transport.play", 0x5e}, {"transport.record", 0x5f},
		{"cursor.up", 0x60}, {"cursor.down", 0x61},
		{"cursor.left", 0x62}, {"cursor.right", 0x63},
		{"navigation.zoom", 0x64}, {"navigation.scrub", 0x65},
	}
	for _, button := range functionButtons {
		add(button.name, button.note, true, true, ConfidenceHigh)
	}

	add("footswitch.1", 0x66, true, false, ConfidenceNeedsHardware)
	add("footswitch.2", 0x67, true, false, ConfidenceNeedsHardware)
	for fader := 1; fader <= FaderCount; fader++ {
		name := fmt.Sprintf("fader.%d.touch", fader)
		if fader == FaderCount {
			name = "fader.master.touch"
		}
		add(name, uint8(0x68+fader-1), true, false, ConfidenceHigh)
	}

	add("display.smpte-led", 0x71, false, true, ConfidenceHigh)
	add("display.beats-led", 0x72, false, true, ConfidenceHigh)
	add("display.rude-solo-led", 0x73, false, true, ConfidenceHigh)
	add("relay.click", 0x76, false, true, ConfidenceNeedsHardware)
	return buttons
}

func indexButtonsByName(definitions []ButtonDefinition) map[string]ButtonDefinition {
	indexed := make(map[string]ButtonDefinition, len(definitions))
	for _, definition := range definitions {
		indexed[definition.Name] = definition
	}
	return indexed
}

func indexButtonsByNote(definitions []ButtonDefinition) map[uint8]ButtonDefinition {
	indexed := make(map[uint8]ButtonDefinition, len(definitions))
	for _, definition := range definitions {
		indexed[definition.Note] = definition
	}
	return indexed
}

func ButtonDefinitions() []ButtonDefinition {
	return append([]ButtonDefinition(nil), buttonDefinitions...)
}

func ControlDefinitions() []ControlDefinition {
	controls := make([]ControlDefinition, 0, len(buttonDefinitions)+64)
	for _, button := range buttonDefinitions {
		direction := "host-to-device"
		if button.Input && button.LED {
			direction = "bidirectional"
		} else if button.Input {
			direction = "device-to-host"
		}
		controls = append(controls, ControlDefinition{
			Name: button.Name, Kind: "button", Direction: direction,
			Channel: 1, Number: int(button.Note), Maximum: 127, Confidence: button.Confidence,
		})
	}
	for index := 0; index < FaderCount; index++ {
		name := fmt.Sprintf("fader.%d", index+1)
		if index == FaderCount-1 {
			name = "fader.master"
		}
		controls = append(controls, ControlDefinition{
			Name: name, Kind: "pitch-bend", Direction: "bidirectional",
			Channel: index + 1, Maximum: FaderMax, Confidence: ConfidenceHigh,
		})
	}
	for index := 0; index < ChannelCount; index++ {
		controls = append(controls,
			ControlDefinition{Name: fmt.Sprintf("encoder.%d.turn", index+1), Kind: "relative-cc", Direction: "device-to-host", Channel: 1, Number: 0x10 + index, Minimum: -63, Maximum: 63, Confidence: ConfidenceHigh},
			ControlDefinition{Name: fmt.Sprintf("encoder.%d.ring", index+1), Kind: "encoder-ring", Direction: "host-to-device", Channel: 1, Number: 0x30 + index, Maximum: 0x7f, Confidence: ConfidenceHigh},
			ControlDefinition{Name: fmt.Sprintf("meter.%d", index+1), Kind: "channel-pressure", Direction: "host-to-device", Channel: 1, Maximum: 0x0f, Confidence: ConfidenceHigh},
			ControlDefinition{Name: fmt.Sprintf("lcd.%d", index+1), Kind: "sysex-text", Direction: "host-to-device", Channel: 1, Maximum: 14, Confidence: ConfidenceHigh},
			ControlDefinition{Name: fmt.Sprintf("lcd.%d.color", index+1), Kind: "x-touch-sysex-color", Direction: "host-to-device", Channel: 1, Maximum: 7, Confidence: ConfidenceNeedsHardware},
		)
	}
	controls = append(controls,
		ControlDefinition{Name: "jog", Kind: "relative-cc", Direction: "device-to-host", Channel: 1, Number: 0x3c, Minimum: -63, Maximum: 63, Confidence: ConfidenceHigh},
		ControlDefinition{Name: "expression", Kind: "absolute-cc", Direction: "device-to-host", Channel: 1, Number: 0x2e, Maximum: 127, Confidence: ConfidenceNeedsHardware},
		ControlDefinition{Name: "time-display", Kind: "cc-digits", Direction: "host-to-device", Channel: 1, Number: 0x40, Maximum: 10, Confidence: ConfidenceHigh},
		ControlDefinition{Name: "assignment-display", Kind: "cc-digits", Direction: "host-to-device", Channel: 1, Number: 0x4a, Maximum: 2, Confidence: ConfidenceHigh},
	)
	return controls
}
