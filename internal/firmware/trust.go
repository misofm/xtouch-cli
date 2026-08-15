package firmware

import (
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

//go:embed trusted_releases.json
var trustedReleasesJSON []byte

type TrustedRelease struct {
	Product       string `json:"product"`
	Model         string `json:"model"`
	Version       string `json:"version"`
	PayloadFile   string `json:"payloadFile"`
	PayloadSHA256 string `json:"payloadSHA256"`
	PayloadBytes  int    `json:"payloadBytes"`
	PacketCount   int    `json:"packetCount"`
	ArchiveFile   string `json:"archiveFile"`
	ArchiveSHA256 string `json:"archiveSHA256"`
	SourceURL     string `json:"sourceURL"`
	VerifiedAt    string `json:"verifiedAt"`
}

type trustManifest struct {
	SchemaVersion int              `json:"schemaVersion"`
	Releases      []TrustedRelease `json:"releases"`
}

var trustedManifest = mustLoadTrustedManifest()

func TrustedReleases() []TrustedRelease {
	return slices.Clone(trustedManifest.Releases)
}

func LookupTrustedRelease(sha256 string) (TrustedRelease, bool) {
	for _, release := range trustedManifest.Releases {
		if strings.EqualFold(release.PayloadSHA256, sha256) {
			return release, true
		}
	}
	return TrustedRelease{}, false
}

func mustLoadTrustedManifest() trustManifest {
	var manifest trustManifest
	if err := json.Unmarshal(trustedReleasesJSON, &manifest); err != nil {
		panic(fmt.Sprintf("invalid embedded firmware trust manifest: %v", err))
	}
	if manifest.SchemaVersion != 1 {
		panic(fmt.Sprintf("unsupported embedded firmware trust schema %d", manifest.SchemaVersion))
	}
	seen := make(map[string]struct{}, len(manifest.Releases))
	for index, release := range manifest.Releases {
		if release.Product == "" || release.Model == "" || release.Version == "" || release.PayloadFile == "" || release.SourceURL == "" || release.VerifiedAt == "" {
			panic(fmt.Sprintf("trusted firmware release %d is missing required provenance", index))
		}
		if !validSHA256(release.PayloadSHA256) || !validSHA256(release.ArchiveSHA256) {
			panic(fmt.Sprintf("trusted firmware release %s has an invalid SHA-256", release.Version))
		}
		if release.PayloadBytes <= 0 || release.PacketCount < 2 {
			panic(fmt.Sprintf("trusted firmware release %s has invalid size metadata", release.Version))
		}
		digest := strings.ToLower(release.PayloadSHA256)
		if _, exists := seen[digest]; exists {
			panic(fmt.Sprintf("trusted firmware manifest contains duplicate payload hash %s", digest))
		}
		seen[digest] = struct{}{}
	}
	return manifest
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
