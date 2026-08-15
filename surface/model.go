package surface

import (
	"errors"
	"fmt"
	"strings"
)

type LEDState string

const (
	LEDOff   LEDState = "off"
	LEDBlink LEDState = "blink"
	LEDSolid LEDState = "solid"
)

type RingMode string

const (
	RingSingle   RingMode = "single"
	RingBoostCut RingMode = "boost-cut"
	RingWrap     RingMode = "wrap"
	RingSpread   RingMode = "spread"
)

type Color string

const (
	ColorBlack   Color = "black"
	ColorRed     Color = "red"
	ColorGreen   Color = "green"
	ColorYellow  Color = "yellow"
	ColorBlue    Color = "blue"
	ColorMagenta Color = "magenta"
	ColorCyan    Color = "cyan"
	ColorWhite   Color = "white"
)

var colors = [...]Color{
	ColorBlack, ColorRed, ColorGreen, ColorYellow,
	ColorBlue, ColorMagenta, ColorCyan, ColorWhite,
}

type FaderSnapshot struct {
	Position int  `json:"position"`
	Touched  bool `json:"touched"`
}

type RingSnapshot struct {
	Mode      RingMode `json:"mode"`
	Position  int      `json:"position"`
	CenterLED bool     `json:"centerLed"`
}

type MeterSnapshot struct {
	Level    int  `json:"level"`
	Overload bool `json:"overload"`
}

type DigitSnapshot struct {
	Code      int    `json:"code"`
	Character string `json:"character"`
	Dot       bool   `json:"dot"`
}

type ButtonSnapshot struct {
	Pressed bool     `json:"pressed"`
	LED     LEDState `json:"led,omitempty"`
}

type StripSnapshot struct {
	Number   int           `json:"number"`
	Fader    FaderSnapshot `json:"fader"`
	Ring     RingSnapshot  `json:"ring"`
	Meter    MeterSnapshot `json:"meter"`
	LCDUpper string        `json:"lcdUpper"`
	LCDLower string        `json:"lcdLower"`
	Color    Color         `json:"color"`
}

type ConfigurationSnapshot struct {
	TransportClick        bool   `json:"transportClick"`
	BacklightSaverMinutes int    `json:"backlightSaverMinutes"`
	TouchlessFaders       bool   `json:"touchlessFaders"`
	FaderSensitivity      []int  `json:"faderSensitivity"`
	MeterModes            []int  `json:"meterModes"`
	LCDMeterOrientation   string `json:"lcdMeterOrientation"`
}

type Snapshot struct {
	Firmware   string                    `json:"firmware"`
	ClockMS    int64                     `json:"clockMs"`
	Strips     []StripSnapshot           `json:"strips"`
	Master     FaderSnapshot             `json:"master"`
	Buttons    map[string]ButtonSnapshot `json:"buttons"`
	Time       []DigitSnapshot           `json:"time"`
	Assignment []DigitSnapshot           `json:"assignment"`
	Expression int                       `json:"expression"`
	Config     ConfigurationSnapshot     `json:"configuration"`
}

type ApplyResult struct {
	Changed  []string `json:"changed,omitempty"`
	Outbound [][]byte `json:"midi,omitempty"`
	Warning  string   `json:"warning,omitempty"`
}

type faderState struct {
	position int
	touched  bool
}

type ringState struct {
	mode      RingMode
	position  int
	centerLED bool
}

type meterState struct {
	level    int
	overload bool
}

type digitState struct {
	code int
	dot  bool
}

type Model struct {
	firmware            string
	faders              [FaderCount]faderState
	rings               [ChannelCount]ringState
	meters              [ChannelCount]meterState
	meterDecayRemainder [ChannelCount]int
	lcd                 [112]byte
	colors              [ChannelCount]Color
	pressed             [120]bool
	leds                [120]LEDState
	time                [10]digitState
	assign              [2]digitState
	expression          int

	transportClick      bool
	backlightSaver      int
	touchless           bool
	sensitivity         [FaderCount]int
	meterModes          [ChannelCount]int
	lcdMeterOrientation string
	clockMS             int64
}

func NewModel(firmware string) *Model {
	if strings.TrimSpace(firmware) == "" {
		firmware = "V1.25"
	}
	model := &Model{firmware: firmware}
	model.Reset()
	return model
}

func (model *Model) Reset() {
	firmware := model.firmware
	*model = Model{firmware: firmware, transportClick: true, backlightSaver: 15, lcdMeterOrientation: "horizontal"}
	for index := range model.lcd {
		model.lcd[index] = ' '
	}
	for index := range model.colors {
		model.colors[index] = ColorBlack
	}
	for index := range model.rings {
		model.rings[index].mode = RingSingle
	}
	for index := range model.leds {
		model.leds[index] = LEDOff
	}
	for index := range model.sensitivity {
		model.sensitivity[index] = 3
	}
}

func (model *Model) Snapshot() Snapshot {
	strips := make([]StripSnapshot, ChannelCount)
	for index := 0; index < ChannelCount; index++ {
		upper := string(model.lcd[index*7 : index*7+7])
		lowerOffset := 56 + index*7
		strips[index] = StripSnapshot{
			Number:   index + 1,
			Fader:    FaderSnapshot{Position: model.faders[index].position, Touched: model.faders[index].touched},
			Ring:     RingSnapshot{Mode: model.rings[index].mode, Position: model.rings[index].position, CenterLED: model.rings[index].centerLED},
			Meter:    MeterSnapshot{Level: model.meters[index].level, Overload: model.meters[index].overload},
			LCDUpper: upper, LCDLower: string(model.lcd[lowerOffset : lowerOffset+7]), Color: model.colors[index],
		}
	}
	buttons := make(map[string]ButtonSnapshot, len(buttonDefinitions))
	for _, definition := range buttonDefinitions {
		button := ButtonSnapshot{Pressed: model.pressed[definition.Note]}
		if definition.LED {
			button.LED = model.leds[definition.Note]
		}
		buttons[definition.Name] = button
	}
	timeDigits := make([]DigitSnapshot, len(model.time))
	for index, digit := range model.time {
		timeDigits[index] = snapshotDigit(digit)
	}
	assignmentDigits := make([]DigitSnapshot, len(model.assign))
	for index, digit := range model.assign {
		assignmentDigits[index] = snapshotDigit(digit)
	}
	sensitivity := make([]int, len(model.sensitivity))
	copy(sensitivity, model.sensitivity[:])
	meterModes := make([]int, len(model.meterModes))
	copy(meterModes, model.meterModes[:])
	return Snapshot{
		Firmware: model.firmware, ClockMS: model.clockMS, Strips: strips,
		Master:  FaderSnapshot{Position: model.faders[8].position, Touched: model.faders[8].touched},
		Buttons: buttons, Time: timeDigits, Assignment: assignmentDigits,
		Expression: model.expression,
		Config: ConfigurationSnapshot{
			TransportClick: model.transportClick, BacklightSaverMinutes: model.backlightSaver,
			TouchlessFaders: model.touchless, FaderSensitivity: sensitivity,
			MeterModes: meterModes, LCDMeterOrientation: model.lcdMeterOrientation,
		},
	}
}

func snapshotDigit(digit digitState) DigitSnapshot {
	character := " "
	if digit.code >= 0x20 && digit.code <= 0x5f {
		character = string(rune(digit.code))
	}
	return DigitSnapshot{Code: digit.code, Character: character, Dot: digit.dot}
}

func (model *Model) ApplyHostMIDI(message []byte) (ApplyResult, error) {
	if len(message) == 0 {
		return ApplyResult{}, errors.New("empty MIDI message")
	}
	if message[0] == 0xf0 {
		return model.applySysEx(message)
	}
	status := message[0]
	kind := status & 0xf0
	channel := status & 0x0f
	switch kind {
	case 0x80, 0x90:
		if len(message) != 3 {
			return ApplyResult{}, fmt.Errorf("note message requires 3 bytes, got %d", len(message))
		}
		note := message[1]
		definition, ok := buttonsByNote[note]
		if !ok || !definition.LED {
			return ApplyResult{Warning: fmt.Sprintf("note 0x%02x has no modeled X-Touch LED", note)}, nil
		}
		velocity := message[2]
		if kind == 0x80 {
			velocity = 0
		}
		model.leds[note] = ledState(velocity)
		return ApplyResult{Changed: []string{"buttons." + definition.Name + ".led"}}, nil
	case 0xb0:
		if len(message) != 3 {
			return ApplyResult{}, fmt.Errorf("control-change message requires 3 bytes, got %d", len(message))
		}
		cc, value := int(message[1]), int(message[2])
		if cc >= 0x30 && cc <= 0x37 && channel == 0 {
			index := cc - 0x30
			model.rings[index] = ringState{
				mode: ringMode((value >> 4) & 0x03), position: value & 0x0f, centerLED: value&0x40 != 0,
			}
			return ApplyResult{Changed: []string{fmt.Sprintf("strips.%d.ring", index+1)}}, nil
		}
		if (channel == 0 || channel == 15) && cc >= 0x40 && cc <= 0x49 {
			index := 9 - (cc - 0x40)
			model.time[index] = digitState{code: value & 0x3f, dot: value&0x40 != 0}
			return ApplyResult{Changed: []string{fmt.Sprintf("time.%d", index+1)}}, nil
		}
		if (channel == 0 || channel == 15) && cc >= 0x4a && cc <= 0x4b {
			index := 1 - (cc - 0x4a)
			model.assign[index] = digitState{code: value & 0x3f, dot: value&0x40 != 0}
			return ApplyResult{Changed: []string{fmt.Sprintf("assignment.%d", index+1)}}, nil
		}
		return ApplyResult{Warning: fmt.Sprintf("CC channel %d number 0x%02x is not a modeled host output", channel+1, cc)}, nil
	case 0xd0:
		if len(message) != 2 || channel != 0 {
			return ApplyResult{}, fmt.Errorf("meter channel-pressure message must be 2 bytes on channel 1")
		}
		strip, level := int(message[1]>>4), int(message[1]&0x0f)
		if strip >= ChannelCount {
			return ApplyResult{}, fmt.Errorf("meter strip %d is out of range", strip+1)
		}
		switch level {
		case 0x0e:
			model.meters[strip].overload = true
		case 0x0f:
			model.meters[strip].overload = false
		default:
			model.meters[strip].level = level
			model.meterDecayRemainder[strip] = 0
		}
		return ApplyResult{Changed: []string{fmt.Sprintf("strips.%d.meter", strip+1)}}, nil
	case 0xe0:
		if len(message) != 3 || channel >= FaderCount {
			return ApplyResult{}, fmt.Errorf("fader pitch-bend message is invalid")
		}
		position := int(message[1]) | int(message[2])<<7
		if model.faders[channel].touched {
			return ApplyResult{Warning: fmt.Sprintf("motor command for touched fader %d was suppressed", channel+1)}, nil
		}
		model.faders[channel].position = position
		return ApplyResult{Changed: []string{faderPath(int(channel))}}, nil
	default:
		return ApplyResult{Warning: fmt.Sprintf("MIDI status 0x%02x is not modeled as host output", status)}, nil
	}
}

// Advance moves the simulator's deterministic clock. X-Touch meters decay
// autonomously by approximately one division every 300 ms, so hosts must
// refresh sustained non-zero levels even when the quantized value is unchanged.
func (model *Model) Advance(milliseconds int) (ApplyResult, error) {
	if milliseconds < 0 {
		return ApplyResult{}, errors.New("advance duration cannot be negative")
	}
	model.clockMS += int64(milliseconds)
	changed := make([]string, 0, ChannelCount)
	for index := range model.meters {
		if model.meters[index].level == 0 {
			model.meterDecayRemainder[index] = 0
			continue
		}
		elapsed := model.meterDecayRemainder[index] + milliseconds
		steps := elapsed / 300
		model.meterDecayRemainder[index] = elapsed % 300
		if steps == 0 {
			continue
		}
		before := model.meters[index].level
		model.meters[index].level = max(0, before-steps)
		if model.meters[index].level != before {
			changed = append(changed, fmt.Sprintf("strips.%d.meter.level", index+1))
		}
	}
	return ApplyResult{Changed: changed}, nil
}

func (model *Model) applySysEx(message []byte) (ApplyResult, error) {
	if len(message) < 7 || message[len(message)-1] != 0xf7 {
		return ApplyResult{}, errors.New("malformed SysEx message")
	}
	if message[1] != 0x00 || message[2] != 0x00 || message[3] != 0x66 || message[4] != 0x14 {
		return ApplyResult{Warning: "SysEx is not addressed to an MCU/X-Touch main unit"}, nil
	}
	command := message[5]
	parameters := message[6 : len(message)-1]
	switch command {
	case 0x0a:
		if len(parameters) != 1 {
			return ApplyResult{}, errors.New("transport-click SysEx requires one parameter")
		}
		model.transportClick = parameters[0] != 0
		return ApplyResult{Changed: []string{"configuration.transportClick"}}, nil
	case 0x0b:
		if len(parameters) != 1 {
			return ApplyResult{}, errors.New("backlight-saver SysEx requires one parameter")
		}
		model.backlightSaver = int(parameters[0])
		return ApplyResult{Changed: []string{"configuration.backlightSaverMinutes"}}, nil
	case 0x0c:
		if len(parameters) != 1 {
			return ApplyResult{}, errors.New("touchless-fader SysEx requires one parameter")
		}
		model.touchless = parameters[0] != 0
		return ApplyResult{Changed: []string{"configuration.touchlessFaders"}}, nil
	case 0x0e:
		if len(parameters) != 2 || parameters[0] >= FaderCount || parameters[1] > 5 {
			return ApplyResult{}, errors.New("fader-sensitivity SysEx requires fader 0..8 and sensitivity 0..5")
		}
		model.sensitivity[parameters[0]] = int(parameters[1])
		return ApplyResult{Changed: []string{fmt.Sprintf("configuration.faderSensitivity.%d", parameters[0]+1)}}, nil
	case 0x12:
		if len(parameters) < 2 {
			return ApplyResult{}, errors.New("LCD SysEx requires an offset and at least one character")
		}
		offset := int(parameters[0])
		if offset >= len(model.lcd) || offset+len(parameters)-1 > len(model.lcd) {
			return ApplyResult{}, errors.New("LCD SysEx write is out of range")
		}
		copy(model.lcd[offset:], parameters[1:])
		return ApplyResult{Changed: []string{"lcd"}}, nil
	case 0x13:
		if len(parameters) != 1 || parameters[0] != 0 {
			return ApplyResult{}, errors.New("firmware-version request must contain parameter 0")
		}
		version := []byte(model.firmware)
		if len(version) > 5 {
			version = version[:5]
		}
		for len(version) < 5 {
			version = append(version, ' ')
		}
		reply := append([]byte{0xf0, 0x00, 0x00, 0x66, 0x14, 0x14}, version...)
		reply = append(reply, 0xf7)
		return ApplyResult{Outbound: [][]byte{reply}}, nil
	case 0x18:
		return ApplyResult{}, errors.New("firmware-update SysEx is intentionally rejected by simulation mode")
	case 0x20:
		if len(parameters) != 2 || parameters[0] >= ChannelCount {
			return ApplyResult{}, errors.New("meter-mode SysEx requires strip 0..7 and a mode byte")
		}
		model.meterModes[parameters[0]] = int(parameters[1] & 0x07)
		return ApplyResult{Changed: []string{fmt.Sprintf("configuration.meterModes.%d", parameters[0]+1)}}, nil
	case 0x21:
		if len(parameters) != 1 || parameters[0] > 1 {
			return ApplyResult{}, errors.New("LCD meter orientation must be 0 or 1")
		}
		if parameters[0] == 0 {
			model.lcdMeterOrientation = "horizontal"
		} else {
			model.lcdMeterOrientation = "vertical"
		}
		return ApplyResult{Changed: []string{"configuration.lcdMeterOrientation"}}, nil
	case 0x72:
		if len(parameters) != ChannelCount {
			return ApplyResult{}, errors.New("X-Touch color SysEx requires exactly eight colors")
		}
		for index, value := range parameters {
			if value > 7 {
				return ApplyResult{}, fmt.Errorf("strip %d color %d is out of range", index+1, value)
			}
			model.colors[index] = colors[value]
		}
		return ApplyResult{Changed: []string{"strips.colors"}}, nil
	default:
		return ApplyResult{Warning: fmt.Sprintf("MCU SysEx command 0x%02x is not modeled", command)}, nil
	}
}

func ledState(velocity byte) LEDState {
	switch velocity {
	case 0:
		return LEDOff
	case 1:
		return LEDBlink
	default:
		return LEDSolid
	}
}

func ringMode(value int) RingMode {
	switch value {
	case 1:
		return RingBoostCut
	case 2:
		return RingWrap
	case 3:
		return RingSpread
	default:
		return RingSingle
	}
}

func faderPath(index int) string {
	if index == FaderCount-1 {
		return "master.position"
	}
	return fmt.Sprintf("strips.%d.fader.position", index+1)
}
