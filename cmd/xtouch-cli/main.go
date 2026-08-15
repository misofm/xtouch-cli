package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/misofm/xtouch-cli/internal/firmware"
	"github.com/misofm/xtouch-cli/internal/midi"
	"github.com/misofm/xtouch-cli/surface"
)

var version = "0.1.0-dev"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "xtouch-cli: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stdout)
		return nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	case "version", "--version":
		fmt.Fprintf(stdout, "xtouch-cli %s\n", version)
		return nil
	case "devices":
		return runDevices(args[1:], stdout)
	case "simulate":
		return runSimulate(args[1:], stdin, stdout, stderr)
	case "firmware":
		return runFirmware(args[1:], stdin, stdout, stderr)
	default:
		return fmt.Errorf("unknown command %q; run xtouch-cli help", args[0])
	}
}

func printUsage(output io.Writer) {
	fmt.Fprintln(output, `xtouch-cli — inspect and maintain a full-size Behringer X-Touch

Usage:
  xtouch-cli devices
	  xtouch-cli simulate describe
	  xtouch-cli simulate run [--script FILE]
	  xtouch-cli simulate serve
	  xtouch-cli firmware inspect FILE
  xtouch-cli firmware trusted
  xtouch-cli firmware send --destination INDEX_OR_NAME FILE
  xtouch-cli version

Simulation is deterministic and never opens a MIDI device.
Firmware sending is safety-gated, rate-limited, and never happens implicitly.`)
}

type simulationRequest struct {
	ID           json.RawMessage `json:"id,omitempty"`
	Type         string          `json:"type"`
	Bytes        []int           `json:"bytes,omitempty"`
	Control      string          `json:"control,omitempty"`
	Pressed      *bool           `json:"pressed,omitempty"`
	Fader        json.RawMessage `json:"fader,omitempty"`
	Position     int             `json:"position,omitempty"`
	Encoder      int             `json:"encoder,omitempty"`
	Delta        int             `json:"delta,omitempty"`
	Value        int             `json:"value,omitempty"`
	Milliseconds int             `json:"milliseconds,omitempty"`
}

type simulationResponse struct {
	Schema        string            `json:"schema"`
	Type          string            `json:"type"`
	ID            json.RawMessage   `json:"id,omitempty"`
	Error         string            `json:"error,omitempty"`
	Changed       []string          `json:"changed,omitempty"`
	MIDI          [][]int           `json:"midi,omitempty"`
	MIDIDirection string            `json:"midiDirection,omitempty"`
	Warning       string            `json:"warning,omitempty"`
	State         *surface.Snapshot `json:"state,omitempty"`
	Device        string            `json:"device,omitempty"`
}

func runSimulate(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "run" {
		if len(args) > 0 {
			args = args[1:]
		}
		return runSimulation(args, stdin, stdout, stderr)
	}
	if args[0] == "describe" {
		if len(args) != 1 {
			return errors.New("usage: xtouch-cli simulate describe")
		}
		description := struct {
			Schema     string                      `json:"schema"`
			Device     string                      `json:"device"`
			Mode       string                      `json:"mode"`
			Controls   []surface.ControlDefinition `json:"controls"`
			References []string                    `json:"references"`
			Caveats    []string                    `json:"caveats"`
		}{
			Schema:   "xtouch.surface-description/v1",
			Device:   "Behringer X-Touch (full-size)",
			Mode:     "Mackie Control (MC)",
			Controls: surface.ControlDefinitions(),
			References: []string{
				"https://cdn-media.empowertribe.com/5f4ebaa5746d48b39c2bc317641de448/QSG_BE_0808-AAD_X-TOUCH_WW.pdf",
				"https://github.com/NicoG60/TouchMCU/blob/main/doc/mackie_control_protocol.md",
			},
			Caveats: []string{
				"The X-Touch manual defines physical controls; host applications define their semantics.",
				"Controls marked needs-hardware-validation are modeled from corroborated reverse engineering but are not yet verified on this firmware-1.25 unit.",
			},
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(description)
	}
	if args[0] == "serve" {
		return runSimulationServe(args[1:], stdin, stdout, stderr)
	}
	return fmt.Errorf("unknown simulate command %q", args[0])
}

func runSimulationServe(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("simulate serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	name := flags.String("name", "X-Touch Simulator INT", "CoreMIDI input/output endpoint name")
	firmwareVersion := flags.String("firmware", "V1.25", "firmware version returned by identity requests")
	stayOpen := flags.Bool("stay-open", false, "keep endpoints alive after stdin reaches EOF")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: xtouch-cli simulate serve [--name NAME] [--stay-open]")
	}

	device, err := midi.OpenVirtualDevice(*name)
	if err != nil {
		return err
	}
	defer device.Close()
	model := surface.NewModel(*firmwareVersion)
	encoder := json.NewEncoder(stdout)
	if err := encoder.Encode(simulationResponse{
		Schema: "xtouch.sim/v1", Type: "ready", Device: device.Name(),
	}); err != nil {
		return err
	}

	lines := make(chan string)
	scanErrors := make(chan error, 1)
	go scanLines(stdin, lines, scanErrors)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	lineNumber := 0
	for {
		select {
		case <-signals:
			return nil
		case scanErr := <-scanErrors:
			if scanErr != nil {
				return fmt.Errorf("read simulation commands: %w", scanErr)
			}
			scanErrors = nil
		case line, ok := <-lines:
			if !ok {
				lines = nil
				if !*stayOpen {
					return nil
				}
				continue
			}
			lineNumber++
			if strings.TrimSpace(line) == "" {
				continue
			}
			var request simulationRequest
			if err := json.Unmarshal([]byte(line), &request); err != nil {
				if err := encoder.Encode(newSimulationErrorResponse(nil, lineNumber, "", fmt.Errorf("decode JSON: %w", err))); err != nil {
					return err
				}
				continue
			}
			response, err := applySimulationRequest(model, request)
			if err != nil {
				if err := encoder.Encode(newSimulationErrorResponse(request.ID, lineNumber, request.Type, err)); err != nil {
					return err
				}
				continue
			}
			for _, message := range response.MIDI {
				bytes, conversionErr := midiBytes(message)
				if conversionErr != nil {
					return conversionErr
				}
				if err := device.Send(bytes); err != nil {
					return err
				}
			}
			if err := encoder.Encode(response); err != nil {
				return err
			}
		case message, ok := <-device.Received():
			if !ok {
				return errors.New("virtual MIDI endpoint closed unexpectedly")
			}
			result, applyErr := model.ApplyHostMIDI(message)
			response := simulationResponse{
				Schema: "xtouch.sim/v1", Type: "host.midi", MIDI: integerMessages([][]byte{message}),
				MIDIDirection: "host-to-device", Changed: result.Changed, Warning: result.Warning,
			}
			if applyErr != nil {
				response.Type = "error"
				response.Warning = applyErr.Error()
			}
			for _, reply := range result.Outbound {
				if err := device.Send(reply); err != nil {
					return err
				}
			}
			if err := encoder.Encode(response); err != nil {
				return err
			}
		}
	}
}

func newSimulationErrorResponse(id json.RawMessage, lineNumber int, requestType string, err error) simulationResponse {
	context := fmt.Sprintf("simulation line %d", lineNumber)
	if requestType != "" {
		context += fmt.Sprintf(" (%s)", requestType)
	}
	return simulationResponse{
		Schema: "xtouch.sim/v1",
		Type:   "error",
		ID:     id,
		Error:  fmt.Sprintf("%s: %v", context, err),
	}
}

func scanLines(input io.Reader, lines chan<- string, scanErrors chan<- error) {
	defer close(lines)
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines <- scanner.Text()
	}
	scanErrors <- scanner.Err()
}

func runSimulation(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("simulate run", flag.ContinueOnError)
	flags.SetOutput(stderr)
	scriptPath := flags.String("script", "", "read NDJSON commands from FILE instead of stdin")
	firmwareVersion := flags.String("firmware", "V1.25", "firmware version returned by identity requests")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: xtouch-cli simulate run [--script FILE]")
	}

	input := stdin
	var file *os.File
	if *scriptPath != "" {
		opened, err := os.Open(*scriptPath)
		if err != nil {
			return fmt.Errorf("open simulation script: %w", err)
		}
		file = opened
		defer file.Close()
		input = file
	}

	model := surface.NewModel(*firmwareVersion)
	encoder := json.NewEncoder(stdout)
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var request simulationRequest
		if err := json.Unmarshal([]byte(line), &request); err != nil {
			return fmt.Errorf("simulation line %d: decode JSON: %w", lineNumber, err)
		}
		response, err := applySimulationRequest(model, request)
		if err != nil {
			return fmt.Errorf("simulation line %d (%s): %w", lineNumber, request.Type, err)
		}
		if err := encoder.Encode(response); err != nil {
			return fmt.Errorf("write simulation response: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read simulation commands: %w", err)
	}
	return nil
}

func applySimulationRequest(model *surface.Model, request simulationRequest) (simulationResponse, error) {
	response := simulationResponse{
		Schema: "xtouch.sim/v1", Type: "result", ID: request.ID,
	}
	var result surface.ApplyResult
	var err error
	switch request.Type {
	case "host.midi":
		message, conversionErr := midiBytes(request.Bytes)
		if conversionErr != nil {
			return response, conversionErr
		}
		result, err = model.ApplyHostMIDI(message)
	case "user.button":
		if request.Pressed == nil {
			return response, errors.New("user.button requires pressed=true or false")
		}
		result, err = model.Button(request.Control, *request.Pressed)
	case "user.fader":
		var index int
		index, err = simulationFaderIndex(request.Fader)
		if err == nil {
			result, err = model.FaderMove(index, request.Position)
		}
	case "user.fader-touch":
		if request.Pressed == nil {
			return response, errors.New("user.fader-touch requires pressed=true or false")
		}
		var index int
		index, err = simulationFaderIndex(request.Fader)
		if err == nil {
			result, err = model.FaderTouch(index, *request.Pressed)
		}
	case "user.encoder-turn":
		result, err = model.EncoderTurn(request.Encoder-1, request.Delta)
	case "user.encoder-press":
		if request.Pressed == nil {
			return response, errors.New("user.encoder-press requires pressed=true or false")
		}
		result, err = model.Button(fmt.Sprintf("encoder.%d.press", request.Encoder), *request.Pressed)
	case "user.jog":
		result, err = model.Jog(request.Delta)
	case "user.expression":
		result, err = model.Expression(request.Value)
	case "advance":
		result, err = model.Advance(request.Milliseconds)
	case "snapshot":
		state := model.Snapshot()
		response.Type = "snapshot"
		response.State = &state
		return response, nil
	case "reset":
		model.Reset()
		result.Changed = []string{"*"}
	default:
		return response, fmt.Errorf("unknown request type %q", request.Type)
	}
	if err != nil {
		return response, err
	}
	response.Changed = result.Changed
	response.Warning = result.Warning
	response.MIDI = integerMessages(result.Outbound)
	if len(response.MIDI) > 0 {
		response.MIDIDirection = "device-to-host"
	}
	return response, nil
}

func midiBytes(values []int) ([]byte, error) {
	if len(values) == 0 {
		return nil, errors.New("host.midi requires a non-empty bytes array")
	}
	message := make([]byte, len(values))
	for index, value := range values {
		if value < 0 || value > 255 {
			return nil, fmt.Errorf("MIDI byte %d is out of range: %d", index, value)
		}
		message[index] = byte(value)
	}
	return message, nil
}

func integerMessages(messages [][]byte) [][]int {
	if len(messages) == 0 {
		return nil
	}
	converted := make([][]int, len(messages))
	for messageIndex, message := range messages {
		converted[messageIndex] = make([]int, len(message))
		for byteIndex, value := range message {
			converted[messageIndex][byteIndex] = int(value)
		}
	}
	return converted
}

func simulationFaderIndex(raw json.RawMessage) (int, error) {
	if len(raw) == 0 {
		return 0, errors.New("fader must be 1..8 or \"master\"")
	}
	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		if strings.EqualFold(name, "master") {
			return surface.FaderCount - 1, nil
		}
		value, conversionErr := strconv.Atoi(name)
		if conversionErr != nil {
			return 0, errors.New("fader must be 1..8 or \"master\"")
		}
		if value >= 1 && value <= surface.ChannelCount {
			return value - 1, nil
		}
		return 0, errors.New("fader must be 1..8 or \"master\"")
	}
	var value int
	if err := json.Unmarshal(raw, &value); err == nil && value >= 1 && value <= surface.ChannelCount {
		return value - 1, nil
	}
	return 0, errors.New("fader must be 1..8 or \"master\"")
}

func runDevices(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("devices", flag.ContinueOnError)
	flags.SetOutput(output)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("devices does not accept positional arguments")
	}

	destinations, err := midi.Destinations()
	if err != nil {
		return err
	}
	if len(destinations) == 0 {
		fmt.Fprintln(output, "No MIDI output destinations found.")
		return nil
	}
	for _, destination := range destinations {
		state := "online"
		if destination.Offline {
			state = "offline"
		}
		fmt.Fprintf(output, "[%d] %s (%s, id=%d", destination.Index, fallback(destination.Name, "Unnamed MIDI destination"), state, destination.UniqueID)
		if destination.Manufacturer != "" {
			fmt.Fprintf(output, ", manufacturer=%s", destination.Manufacturer)
		}
		if destination.Model != "" && destination.Model != destination.Name {
			fmt.Fprintf(output, ", model=%s", destination.Model)
		}
		fmt.Fprintf(output, ", max-sysex-rate=%d bytes/s", destination.MaxSysExRate)
		fmt.Fprintln(output, ")")
	}
	return nil
}

func runFirmware(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("firmware requires inspect or send")
	}
	switch args[0] {
	case "inspect":
		return runFirmwareInspect(args[1:], stdout)
	case "trusted":
		return runFirmwareTrusted(args[1:], stdout)
	case "send":
		return runFirmwareSend(args[1:], stdin, stdout, stderr)
	default:
		return fmt.Errorf("unknown firmware command %q", args[0])
	}
}

func runFirmwareTrusted(args []string, output io.Writer) error {
	if len(args) != 0 {
		return errors.New("usage: xtouch-cli firmware trusted")
	}
	for _, release := range firmware.TrustedReleases() {
		fmt.Fprintf(output, "X-Touch %s\n", release.Version)
		fmt.Fprintf(output, "  payload:  %s\n", release.PayloadFile)
		fmt.Fprintf(output, "  sha256:   %s\n", release.PayloadSHA256)
		fmt.Fprintf(output, "  source:   %s\n", release.SourceURL)
		fmt.Fprintf(output, "  verified: %s\n", release.VerifiedAt)
	}
	return nil
}

func runFirmwareInspect(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("firmware inspect", flag.ContinueOnError)
	flags.SetOutput(output)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: xtouch-cli firmware inspect FILE")
	}
	image, err := firmware.Load(flags.Arg(0))
	if err != nil {
		return err
	}
	printImage(output, image)
	return nil
}

func runFirmwareSend(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("firmware send", flag.ContinueOnError)
	flags.SetOutput(stderr)
	destinationSelector := flags.String("destination", "", "CoreMIDI destination index or exact name")
	allowAnyDestination := flags.Bool("allow-any-destination", false, "allow a destination whose metadata does not resemble X-Touch/update mode")
	allowUntrustedImage := flags.Bool("allow-untrusted-image", false, "allow a structurally valid image whose hash is not in the embedded trust manifest")
	yes := flags.Bool("yes", false, "skip the interactive prompt; requires --expected-sha256")
	expectedSHA := flags.String("expected-sha256", "", "required full image SHA-256 when --yes is used")
	rate := flags.Int("bytes-per-second", firmware.StandardMIDIRate, "send rate; cannot exceed standard MIDI speed (3125)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || *destinationSelector == "" {
		return errors.New("usage: xtouch-cli firmware send --destination INDEX_OR_NAME FILE")
	}
	if *rate <= 0 || *rate > firmware.StandardMIDIRate {
		return fmt.Errorf("--bytes-per-second must be between 1 and %d", firmware.StandardMIDIRate)
	}

	image, err := firmware.Load(flags.Arg(0))
	if err != nil {
		return err
	}
	if image.Trusted == nil && !*allowUntrustedImage {
		return fmt.Errorf("image hash %s is not in the embedded trust manifest; inspect it and pass --allow-untrusted-image only after independent verification", image.SHA256)
	}

	destinations, err := midi.Destinations()
	if err != nil {
		return err
	}
	destination, err := selectDestination(destinations, *destinationSelector)
	if err != nil {
		return err
	}
	if destination.Offline {
		return fmt.Errorf("destination %q is offline", destination.Name)
	}
	if !looksLikeXTouch(destination) && !*allowAnyDestination {
		return fmt.Errorf("destination %q does not resemble an X-Touch updater; pass --allow-any-destination only after verifying the port", destination.Name)
	}

	printImage(stdout, image)
	fmt.Fprintf(stdout, "Destination: [%d] %s\n", destination.Index, fallback(destination.Name, "Unnamed MIDI destination"))
	fmt.Fprintf(stdout, "Rate:        %d bytes/s (%.1f s estimated)\n", *rate, image.EstimatedDuration(*rate).Seconds())
	fmt.Fprintf(stdout, "CoreMIDI:    destination reports max SysEx rate %d bytes/s\n", destination.MaxSysExRate)
	fmt.Fprintln(stdout, "\nBefore continuing: connect directly by USB, close every other MIDI client, use stable power, and boot the X-Touch while holding DISPLAY.")
	fmt.Fprintln(stdout, "DO NOT disconnect USB or power during transmission.")

	if *yes {
		if !strings.EqualFold(*expectedSHA, image.SHA256) {
			return errors.New("--yes requires --expected-sha256 with the image's complete SHA-256")
		}
	} else {
		phrase := "UPDATE " + image.SHA256[:8]
		fmt.Fprintf(stdout, "\nType %q to begin: ", phrase)
		reader := bufio.NewReader(stdin)
		response, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read confirmation: %w", err)
		}
		if strings.TrimSpace(response) != phrase {
			return errors.New("confirmation did not match; nothing was sent")
		}
	}

	output, err := midi.OpenOutput(destination)
	if err != nil {
		return err
	}
	defer output.Close()

	fmt.Fprintf(stdout, "\nSending %d packets. Do not interrupt.\n", len(image.Messages))
	started := time.Now()
	for index, message := range image.Messages {
		packetStarted := time.Now()
		if err := output.Send(message); err != nil {
			return fmt.Errorf("transfer failed at packet %d/%d: %w; leave the X-Touch powered on and retry the complete transfer", index+1, len(image.Messages), err)
		}
		minimumPacketTime := time.Duration(len(message)) * time.Second / time.Duration(*rate)
		if remaining := minimumPacketTime - time.Since(packetStarted); remaining > 0 {
			time.Sleep(remaining)
		}
		if index == len(image.Messages)-1 || (index+1)%max(1, len(image.Messages)/100) == 0 {
			fmt.Fprintf(stdout, "\rProgress: %d/%d (%d%%)", index+1, len(image.Messages), (index+1)*100/len(image.Messages))
		}
	}
	time.Sleep(250 * time.Millisecond)
	fmt.Fprintf(stdout, "\nTransfer completed in %.1f s. Power-cycle the X-Touch, verify firmware at normal startup, then return it to MC + USB mode.\n", time.Since(started).Seconds())
	return nil
}

func printImage(output io.Writer, image *firmware.Image) {
	versionName := "unknown release"
	if image.Trusted != nil {
		versionName = "trusted X-Touch " + image.Trusted.Version
	}
	fmt.Fprintf(output, "Firmware:    %s\n", versionName)
	fmt.Fprintf(output, "Payload:     %s (%d bytes)\n", image.PayloadName, len(image.Payload))
	fmt.Fprintf(output, "SHA-256:     %s\n", image.SHA256)
	fmt.Fprintf(output, "Packets:     %d data + 1 finalize\n", image.DataPackets)
	fmt.Fprintf(output, "Target:      full-size X-Touch (device byte 0x40)\n")
}

func selectDestination(destinations []midi.Destination, selector string) (midi.Destination, error) {
	if index, err := strconv.Atoi(selector); err == nil {
		for _, destination := range destinations {
			if destination.Index == index {
				return destination, nil
			}
		}
		return midi.Destination{}, fmt.Errorf("MIDI destination index %d was not found; run xtouch-cli devices", index)
	}

	var matches []midi.Destination
	for _, destination := range destinations {
		if strings.EqualFold(destination.Name, selector) {
			matches = append(matches, destination)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return midi.Destination{}, fmt.Errorf("destination name %q is ambiguous; select an index from xtouch-cli devices", selector)
	}
	return midi.Destination{}, fmt.Errorf("MIDI destination %q was not found; run xtouch-cli devices", selector)
}

func looksLikeXTouch(destination midi.Destination) bool {
	metadata := strings.ToLower(destination.Name + " " + destination.Manufacturer + " " + destination.Model)
	return strings.Contains(metadata, "x-touch") || strings.Contains(metadata, "xtouch") || strings.Contains(metadata, "update")
}

func fallback(value, fallbackValue string) string {
	if value == "" {
		return fallbackValue
	}
	return value
}
