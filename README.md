# xtouch-cli

`xtouch-cli` is a small, auditable command-line tool for inspecting and
maintaining the full-size Behringer X-Touch. Its first maintenance operation is
a safety-gated firmware sender for macOS CoreMIDI.

The tool does not bundle firmware and is not affiliated with or endorsed by
Behringer or Music Tribe.

## Why a dedicated CLI?

Firmware transfer should not live in the Studio browser app. An update is an
infrequent, interruption-sensitive maintenance operation, while a browser can
reload, suspend, lose permission, or close its MIDI endpoint. The CLI instead
uses CoreMIDI directly and has no external runtime or MIDI-library dependency.

The firmware parser and inspector are portable Go. Sending currently supports
macOS; Windows and Linux can add backends behind the small
`internal/midi.Output` interface.

## Build

Requirements for a local build:

- Go 1.22 or newer
- macOS command-line developer tools for the CoreMIDI CGO bridge

```sh
go build -o xtouch-cli ./cmd/xtouch-cli
```

No `sudo`, Homebrew library, or installed MIDI application is required.

To build separate Apple Silicon and Intel binaries plus a universal binary:

```sh
VERSION=0.1.0 ./scripts/build-macos-universal.sh
```

Public prebuilt binaries should be code-signed and notarized before release.

## Firmware trust model

The CLI accepts either a `.syx` payload directly or an official ZIP containing
exactly one `.syx` file:

```sh
./xtouch-cli firmware inspect /path/to/X-TOUCH_sysex_update_1-25_1-07.syx
./xtouch-cli firmware inspect /path/to/X-TOUCH_sysex_update_1-25_1-07.zip
```

In both cases, trust is based on the SHA-256 of the **SysEx payload**, not its
filename or the ZIP hash. The binary embeds the reviewable
[`trusted_releases.json`](internal/firmware/trusted_releases.json) manifest.
Each entry records:

- product, model, and firmware version;
- payload filename, size, packet count, and SHA-256;
- archive filename and SHA-256;
- official source URL and verification date.

List the releases trusted by the current binary:

```sh
./xtouch-cli firmware trusted
```

The send command refuses a structurally valid but untrusted hash by default.
`--allow-untrusted-image` exists for independently verified future firmware; it
must not be used merely to bypass a mismatch.

Adding a trusted release requires a focused code review that verifies its model,
official provenance, payload structure, both hashes, byte count, and packet
count. The CLI never downloads or silently updates its trust manifest.

## Firmware update workflow

First inspect the file:

```sh
./xtouch-cli firmware inspect /path/to/firmware.syx
```

Then prepare the hardware:

1. Record the firmware version briefly shown above `BARS` during a normal boot.
2. Close Studio, browsers, DAWs, MIDI monitors, and every other MIDI client.
3. Connect the full-size X-Touch directly to the Mac over USB and use stable
   power.
4. Power the X-Touch off.
5. Hold `DISPLAY` while powering it on to enter firmware-update mode.
6. List the CoreMIDI destinations:

   ```sh
   ./xtouch-cli devices
   ```

Send the trusted payload to the updater destination by its displayed index:

```sh
./xtouch-cli firmware send --destination 0 /path/to/firmware.syx
```

The CLI prints the target, hash, and estimated duration, then requires a typed
confirmation derived from the payload hash. It sends each complete SysEx packet
at no more than the standard MIDI rate of 3,125 bytes per second. Do not close
the terminal, disconnect USB, interrupt the process, sleep the Mac, or remove
power during transfer.

On completion, power-cycle the unit if it does not restart automatically, verify
the new version during normal startup, and restore `MC + USB` mode.

For controlled non-interactive use, `--yes` is accepted only with the complete
matching hash:

```sh
./xtouch-cli firmware send \
  --destination 0 \
  --yes \
  --expected-sha256 d17daae1e7973f9e2eca24fe57a0a4aa705ab121c6ff33ccd29548cc2ee68b8c \
  /path/to/firmware.syx
```

## Safety boundaries

- This tool targets the **full-size X-Touch** firmware device byte `0x40`. It
  rejects payloads for other X-Touch models.
- It validates the Behringer SysEx envelope, consistent data packets, final
  command, target byte, size, packet count, and hash before opening MIDI output.
- It reopens the selected destination by its stable CoreMIDI unique ID, so a
  hot-plug event cannot silently redirect an index to another device.
- It rejects offline and non-X-Touch-looking destinations unless explicitly
  overridden.
- It cannot prove that the hardware is in update mode. The operator must hold
  `DISPLAY` during power-on and verify the displayed destination.
- A CLI cannot make interrupted firmware updates harmless. If sending fails,
  leave the unit powered on and retry the complete transfer before power-cycling.
- Do not treat firmware downgrades as routine. Behringer explicitly warns that
  units shipped with firmware 1.22 must not be downgraded to older releases.

Behringer currently publishes firmware 1.25 but no accompanying 1.25 change
list. Building this tool is not a recommendation to update an already-working
unit without a relevant reason.

## Sources

- [Official X-Touch product and download page](https://www.behringer.com/en/products/0808-AAD)
- [Official X-Touch quick-start guide](https://cdn-media.empowertribe.com/5f4ebaa5746d48b39c2bc317641de448/QSG_BE_0808-AAD_X-TOUCH_WW.pdf)
- [Official Behringer firmware-update video](https://www.youtube.com/watch?v=nElNot_WY34)
- [Firmware 1.22 release notes and downgrade warning](https://cdn-media.empowertribe.com/587016fdfd5f40bcb20df65fe262890e/Release-Notes_BE_0808-AAD_X-TOUCH_v1-22.pdf)

## License

MIT
