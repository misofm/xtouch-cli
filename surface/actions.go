package surface

import "fmt"

func (model *Model) Button(name string, pressed bool) (ApplyResult, error) {
	definition, ok := buttonsByName[name]
	if !ok {
		return ApplyResult{}, fmt.Errorf("unknown X-Touch button %q", name)
	}
	if !definition.Input {
		return ApplyResult{}, fmt.Errorf("control %q is host-output only", name)
	}
	model.pressed[definition.Note] = pressed
	velocity := byte(0)
	if pressed {
		velocity = 0x7f
	}
	return ApplyResult{
		Changed:  []string{"buttons." + name + ".pressed"},
		Outbound: [][]byte{{0x90, definition.Note, velocity}},
	}, nil
}

func (model *Model) FaderMove(index, position int) (ApplyResult, error) {
	if index < 0 || index >= FaderCount {
		return ApplyResult{}, fmt.Errorf("fader index must be 0..8")
	}
	if position < 0 || position > FaderMax {
		return ApplyResult{}, fmt.Errorf("fader position must be 0..%d", FaderMax)
	}
	model.faders[index].position = position
	return ApplyResult{
		Changed:  []string{faderPath(index)},
		Outbound: [][]byte{{0xe0 | byte(index), byte(position & 0x7f), byte((position >> 7) & 0x7f)}},
	}, nil
}

func (model *Model) FaderTouch(index int, touched bool) (ApplyResult, error) {
	if index < 0 || index >= FaderCount {
		return ApplyResult{}, fmt.Errorf("fader index must be 0..8")
	}
	model.faders[index].touched = touched
	model.pressed[0x68+index] = touched
	velocity := byte(0)
	if touched {
		velocity = 0x7f
	}
	path := fmt.Sprintf("strips.%d.fader.touched", index+1)
	if index == FaderCount-1 {
		path = "master.touched"
	}
	return ApplyResult{
		Changed:  []string{path},
		Outbound: [][]byte{{0x90, byte(0x68 + index), velocity}},
	}, nil
}

func (model *Model) EncoderTurn(index, delta int) (ApplyResult, error) {
	if index < 0 || index >= ChannelCount {
		return ApplyResult{}, fmt.Errorf("encoder index must be 0..7")
	}
	value, err := relativeValue(delta)
	if err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Outbound: [][]byte{{0xb0, byte(0x10 + index), value}}}, nil
}

func (model *Model) Jog(delta int) (ApplyResult, error) {
	value, err := relativeValue(delta)
	if err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Outbound: [][]byte{{0xb0, 0x3c, value}}}, nil
}

func (model *Model) Expression(value int) (ApplyResult, error) {
	if value < 0 || value > 127 {
		return ApplyResult{}, fmt.Errorf("expression value must be 0..127")
	}
	model.expression = value
	return ApplyResult{
		Changed:  []string{"expression"},
		Outbound: [][]byte{{0xb0, 0x2e, byte(value)}},
	}, nil
}

func relativeValue(delta int) (byte, error) {
	if delta == 0 || delta < -63 || delta > 63 {
		return 0, fmt.Errorf("relative delta must be -63..-1 or 1..63")
	}
	if delta < 0 {
		return byte(-delta) | 0x40, nil
	}
	return byte(delta), nil
}
