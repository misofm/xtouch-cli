package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/misofm/xtouch-cli/internal/firmware"
	"github.com/misofm/xtouch-cli/internal/midi"
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
  xtouch-cli firmware inspect FILE
  xtouch-cli firmware trusted
  xtouch-cli firmware send --destination INDEX_OR_NAME FILE
  xtouch-cli version

Firmware sending is safety-gated, rate-limited, and never happens implicitly.`)
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
