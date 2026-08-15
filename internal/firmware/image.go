package firmware

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const StandardMIDIRate = 3125

var (
	behringerManufacturer = []byte{0x00, 0x20, 0x32}
	finalizeMessage       = []byte{0xf0, 0x00, 0x20, 0x32, 0x40, 0x47, 0xf7}
)

type Image struct {
	SourcePath  string
	PayloadName string
	Payload     []byte
	Messages    [][]byte
	DataPackets int
	SHA256      string
	Trusted     *TrustedRelease
}

func Load(path string) (*Image, error) {
	payload, payloadName, err := readPayload(path)
	if err != nil {
		return nil, err
	}

	image, err := Parse(payload)
	if err != nil {
		return nil, fmt.Errorf("validate %q: %w", payloadName, err)
	}
	image.SourcePath = path
	image.PayloadName = payloadName
	return image, nil
}

func Parse(payload []byte) (*Image, error) {
	if len(payload) == 0 {
		return nil, errors.New("firmware payload is empty")
	}

	messages, err := splitSysEx(payload)
	if err != nil {
		return nil, err
	}
	if len(messages) < 2 {
		return nil, errors.New("firmware must contain data packets and a finalize packet")
	}

	dataPackets := 0
	dataPacketLength := 0
	for index, message := range messages {
		if len(message) < len(finalizeMessage) {
			return nil, fmt.Errorf("packet %d is too short", index+1)
		}
		if !bytes.Equal(message[1:4], behringerManufacturer) {
			return nil, fmt.Errorf("packet %d does not use the Behringer manufacturer ID", index+1)
		}
		if message[4] != 0x40 {
			return nil, fmt.Errorf("packet %d targets device byte 0x%02x, not full-size X-Touch 0x40", index+1, message[4])
		}

		switch message[5] {
		case 0x46:
			if index == len(messages)-1 {
				return nil, errors.New("firmware ends with a data packet instead of finalize command 0x47")
			}
			if dataPacketLength == 0 {
				dataPacketLength = len(message)
			} else if len(message) != dataPacketLength {
				return nil, fmt.Errorf("data packet %d has length %d; expected consistent length %d", index+1, len(message), dataPacketLength)
			}
			dataPackets++
		case 0x47:
			if index != len(messages)-1 {
				return nil, fmt.Errorf("finalize command appears before the last packet at packet %d", index+1)
			}
			if !bytes.Equal(message, finalizeMessage) {
				return nil, errors.New("finalize packet has an unexpected payload")
			}
		default:
			return nil, fmt.Errorf("packet %d has unsupported firmware command 0x%02x", index+1, message[5])
		}
	}
	if dataPackets == 0 {
		return nil, errors.New("firmware contains no data packets")
	}

	digest := sha256.Sum256(payload)
	digestHex := hex.EncodeToString(digest[:])
	image := &Image{
		Payload:     append([]byte(nil), payload...),
		Messages:    messages,
		DataPackets: dataPackets,
		SHA256:      digestHex,
	}
	if release, ok := LookupTrustedRelease(digestHex); ok {
		if release.PayloadBytes != len(payload) || release.PacketCount != len(messages) {
			return nil, fmt.Errorf("trusted manifest metadata disagrees with payload hash %s", digestHex)
		}
		image.Trusted = &release
	}
	return image, nil
}

func (image *Image) EstimatedDuration(bytesPerSecond int) time.Duration {
	if bytesPerSecond <= 0 {
		return 0
	}
	return time.Duration(len(image.Payload)) * time.Second / time.Duration(bytesPerSecond)
}

func readPayload(path string) ([]byte, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open firmware: %w", err)
	}
	defer file.Close()

	if !strings.EqualFold(filepath.Ext(path), ".zip") {
		payload, err := io.ReadAll(file)
		if err != nil {
			return nil, "", fmt.Errorf("read firmware: %w", err)
		}
		return payload, filepath.Base(path), nil
	}

	info, err := file.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("stat firmware archive: %w", err)
	}
	archive, err := zip.NewReader(file, info.Size())
	if err != nil {
		return nil, "", fmt.Errorf("open firmware archive: %w", err)
	}

	var syxFiles []*zip.File
	for _, entry := range archive.File {
		if !entry.FileInfo().IsDir() && strings.EqualFold(filepath.Ext(entry.Name), ".syx") {
			syxFiles = append(syxFiles, entry)
		}
	}
	if len(syxFiles) != 1 {
		return nil, "", fmt.Errorf("firmware archive must contain exactly one .syx payload; found %d", len(syxFiles))
	}

	entry, err := syxFiles[0].Open()
	if err != nil {
		return nil, "", fmt.Errorf("open firmware payload: %w", err)
	}
	defer entry.Close()
	payload, err := io.ReadAll(entry)
	if err != nil {
		return nil, "", fmt.Errorf("read firmware payload: %w", err)
	}
	return payload, syxFiles[0].Name, nil
}

func splitSysEx(payload []byte) ([][]byte, error) {
	var messages [][]byte
	for offset := 0; offset < len(payload); {
		if payload[offset] != 0xf0 {
			return nil, fmt.Errorf("unexpected byte 0x%02x at offset %d; expected SysEx start 0xf0", payload[offset], offset)
		}
		endRelative := bytes.IndexByte(payload[offset:], 0xf7)
		if endRelative < 0 {
			return nil, fmt.Errorf("unterminated SysEx packet at offset %d", offset)
		}
		end := offset + endRelative
		message := payload[offset : end+1]
		for byteIndex, value := range message[1 : len(message)-1] {
			if value&0x80 != 0 {
				return nil, fmt.Errorf("non-data byte 0x%02x inside packet at offset %d", value, offset+byteIndex+1)
			}
		}
		messages = append(messages, append([]byte(nil), message...))
		offset = end + 1
	}
	return messages, nil
}
