package firmware

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func testPayload() []byte {
	data := []byte{0xf0, 0x00, 0x20, 0x32, 0x40, 0x46, 0x00, 0x01, 0x02, 0xf7}
	return append(data, finalizeMessage...)
}

func TestParseAcceptsXTouchFirmwareEnvelope(t *testing.T) {
	image, err := Parse(testPayload())
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if image.DataPackets != 1 || len(image.Messages) != 2 {
		t.Fatalf("unexpected packet counts: data=%d total=%d", image.DataPackets, len(image.Messages))
	}
	if image.Trusted != nil {
		t.Fatal("synthetic image must not be recognized as a known release")
	}
}

func TestTrustedManifestIncludesOfficial125Payload(t *testing.T) {
	release, ok := LookupTrustedRelease("d17daae1e7973f9e2eca24fe57a0a4aa705ab121c6ff33ccd29548cc2ee68b8c")
	if !ok {
		t.Fatal("official 1.25 payload is missing from trusted manifest")
	}
	if release.Version != "1.25" || release.PayloadBytes != 80647 || release.PacketCount != 897 {
		t.Fatalf("unexpected trusted metadata: %+v", release)
	}
}

func TestParseRejectsWrongDevice(t *testing.T) {
	payload := testPayload()
	payload[4] = 0x41
	if _, err := Parse(payload); err == nil {
		t.Fatal("Parse() accepted a non-X-Touch device byte")
	}
}

func TestParseRejectsMissingFinalize(t *testing.T) {
	payload := testPayload()[:10]
	if _, err := Parse(payload); err == nil {
		t.Fatal("Parse() accepted an image without a finalize packet")
	}
}

func TestLoadReadsSinglePayloadFromZip(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "firmware.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, err := archive.Create("X-TOUCH_test.syx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bytes.NewReader(testPayload()).WriteTo(entry); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	image, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if image.PayloadName != "X-TOUCH_test.syx" {
		t.Fatalf("PayloadName = %q", image.PayloadName)
	}
}
