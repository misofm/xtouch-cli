# xtouch-cli

`xtouch-cli` is a small, auditable command-line tool for inspecting,
simulating, and maintaining the full-size Behringer X-Touch. It includes a
portable software model of the surface in Mackie Control mode and a
safety-gated firmware sender for macOS CoreMIDI.

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
There is no tagged binary release yet; until one is published, build from source
or install the current main branch with:

```sh
go install github.com/misofm/xtouch-cli/cmd/xtouch-cli@latest
```

## Current command surface

The implemented CLI is deliberately small:

| Command | Availability | Purpose |
| --- | --- | --- |
| `devices` | macOS | List CoreMIDI output destinations and their stable IDs. |
| `simulate describe` | All platforms | Print the machine-readable full-surface control catalog. |
| `simulate run` | All platforms | Execute deterministic `xtouch.sim/v1` NDJSON scenarios. |
| `simulate serve` | macOS | Expose the model as temporary CoreMIDI input/output endpoints. |
| `firmware inspect FILE` | All platforms | Parse and validate a `.syx` file or official single-payload ZIP without opening MIDI. |
| `firmware trusted` | All platforms | Print the embedded trusted-release manifest. |
| `firmware send` | macOS | Safety-gated, rate-limited transfer to an explicitly selected CoreMIDI destination. |

Run `xtouch-cli help` for the top-level syntax. Unknown simulation requests sent
to `simulate serve` produce structured `xtouch.sim/v1` error responses without
terminating the virtual device; filesystem, CoreMIDI, and firmware safety errors
still fail the process with a nonzero exit status.

## Surface model and simulation

The `surface` Go package is a deterministic digital twin of the MIDI-facing
X-Touch in Mackie Control (`MC`) mode. It models:

- all channel, encoder-press, function, transport, navigation, automation,
  modifier, utility, footswitch, and fader-touch note messages;
- nine bidirectional 14-bit faders;
- eight relative encoders, four encoder-ring modes, and center LEDs;
- channel meters, sticky overload, and deterministic 300 ms-per-division decay;
- the 112-character LCD buffer and eight firmware-1.22+ strip colors;
- the 10-digit time display, two-digit assignment display, SMPTE/Beats/Rude
  Solo LEDs, jog wheel, expression input, and supported MCU configuration
  SysEx;
- firmware identity replies, defaulting to the upgraded X-Touch version 1.25.

The physical control inventory comes from Behringer's quick-start guide. The
wire map is the MCU protocol: applications still decide what controls such as
`F1`, `Flip`, `Touch`, or an encoder assignment mean. Keeping those semantics
outside the device model lets Studio test different workflows against one
stable hardware contract.

Print the machine-readable control catalog:

```sh
./xtouch-cli simulate describe
```

Run a newline-delimited JSON scenario from stdin or a file:

```sh
./xtouch-cli simulate run --script examples/console-basics.ndjson
```

Each input line produces one `xtouch.sim/v1` result. Commands are:

| Command | Purpose |
| --- | --- |
| `host.midi` | Apply bytes sent by the host to motors, LEDs, rings, meters, or displays. |
| `user.button` | Press or release any named input from `simulate describe`. |
| `user.fader` | Move fader `1`…`8` or `"master"` at position `0`…`16383`. |
| `user.fader-touch` | Touch or release a channel/master fader. |
| `user.encoder-turn` | Turn encoder `1`…`8` by a signed relative delta. |
| `user.encoder-press` | Press or release encoder `1`…`8`. |
| `user.jog` | Turn the jog wheel by a signed relative delta. |
| `user.expression` | Set the expression input to `0`…`127`. |
| `advance` | Advance deterministic time in milliseconds, including meter decay. |
| `snapshot` | Emit the complete modeled surface state. |
| `reset` | Restore the power-on model state. |

For example:

```json
{"id":"play","type":"user.button","control":"transport.play","pressed":true}
{"id":"motor","type":"host.midi","bytes":[224,0,64]}
{"id":"decay","type":"advance","milliseconds":300}
{"id":"state","type":"snapshot"}
```

Simulation never opens physical MIDI devices, and generic firmware-update
SysEx is rejected. The NDJSON driver is suitable for protocol tests, trace
replay, and subprocess integration.

On macOS, the same model can also create temporary browser-visible CoreMIDI
input and output endpoints:

```sh
./xtouch-cli simulate serve
```

Keep that process running and open Studio with `?xtouch=simulator` in the URL,
then connect `X-Touch Simulator INT` from the MIDI menu and type NDJSON `user.*`
commands into the process. Without that query parameter Studio prefers a real
X-Touch when both are online. Host feedback from Studio is applied to the model
and reported as `host.midi` events. The endpoints disappear when the process
exits; they neither connect to nor send data to a physical X-Touch. Use
`--stay-open` when commands come from a finite redirected file and the virtual
endpoints should remain available afterward.

Linux and Windows retain the portable script simulator but do not yet expose a
virtual MIDI backend.

## Programmable-surface API roadmap

The simulator establishes the semantic model, but the CLI is not intended to
stop at firmware maintenance. The next major API should be a persistent,
bidirectional **surface session** rather than a collection of unrelated
one-shot MIDI writes. A session is needed for fader-touch arbitration, meter
refresh, output coalescing, reconnect, and authoritative state replay.

The proposed command families are:

```text
xtouch-cli devices [list] [--format text|json]
xtouch-cli devices watch [--format ndjson]
xtouch-cli devices probe --device DEVICE [--format json]
xtouch-cli diagnose --device DEVICE [--interactive]

xtouch-cli midi monitor --device DEVICE [--decode raw|mcu|ctrl]
xtouch-cli midi send --device DEVICE --hex "90 10 7f"
xtouch-cli trace record --device DEVICE FILE
xtouch-cli trace decode FILE
xtouch-cli trace replay --device DEVICE FILE --dry-run

xtouch-cli surface listen --device DEVICE
xtouch-cli surface set fader --device DEVICE --strip 1 --position 8192
xtouch-cli surface set led --device DEVICE --control mute --strip 1 --state solid
xtouch-cli surface set ring --device DEVICE --strip 1 --mode dot --position 6
xtouch-cli surface set lcd --device DEVICE --strip 1 --upper Vocals --lower -3.2dB
xtouch-cli surface set color --device DEVICE --strip 1 --color cyan
xtouch-cli surface set meter --device DEVICE --strip 1 --level 8
xtouch-cli surface set time --device DEVICE --text "01:23:45:12"
xtouch-cli surface clear --device DEVICE

xtouch-cli session --device DEVICE [--format ndjson]
xtouch-cli mapping validate FILE
xtouch-cli mapping run --device DEVICE --config FILE
```

These commands are a roadmap, not part of the current release. The first stable
machine contract should be a versioned JSON Lines session envelope:

```json
{"schema":"xtouch.session/v1","type":"command","id":"42","command":{"kind":"fader.set","strip":1,"position":8192}}
{"schema":"xtouch.session/v1","type":"ack","id":"42","seq":18}
{"schema":"xtouch.session/v1","type":"event","seq":19,"device":"coremidi:entity:123","event":{"kind":"fader.move","strip":1,"position":8200},"raw":[224,8,64]}
```

Contract and safety principles:

- use stable device/entity identities; destination indices are only a human
  convenience;
- keep progress and diagnostics on stderr and machine-readable events on
  stdout;
- expose user-facing strips as `1..8` and the ninth fader as `master`;
- accept raw `0..16383` or normalized fader positions; require an explicit
  taper before accepting dB because MCU itself does not define one;
- suppress motor feedback while a fader is touched, then apply the latest
  desired value after release;
- coalesce output by family: motors and buttons first, meters at approximately
  10 Hz, displays and colors on change;
- never move faders during `surface clear` unless explicitly requested;
- reject unknown SysEx by default, and always route firmware-shaped SysEx
  through the existing trusted firmware workflow;
- make raw replay require explicit motor permission and remain dry-run by
  default;
- reconnect only to the same stable device identity and replay authoritative
  desired state without silently taking over another surface.

The intended Go package boundary is `protocol/mcu` for pure MCU bytes,
`protocol/xtouch` for model-specific extensions, `surface` for desired state and
scheduling, and `transport` for duplex discovery and I/O. The JSON contract
should stabilize before promising a stable public Go API.

Recommended implementation order:

1. Add duplex CoreMIDI discovery/pairing, ordinary MIDI packet sending, input
   callbacks, and hot-plug events.
2. Extract a pure typed MCU/X-Touch codec from the existing model.
3. Add JSON device listing/probing, diagnostics, and semantic/raw monitoring.
4. Add safety-gated `surface set` commands for faders, LEDs, rings, LCDs, and
   meters.
5. Ship the persistent NDJSON session with scheduling, touch suppression,
   reconnect events, and state replay.
6. Physically validate firmware-1.25 colors, ring patterns, time digits, clip
   states, and pedals before declaring those APIs stable.

State files, mapping execution, trace replay, Ctrl mode, native RTP-MIDI,
Extenders, and non-macOS transports can follow the session MVP.

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
- [TouchMCU reverse-engineered Mackie Control protocol reference](https://github.com/NicoG60/TouchMCU/blob/main/doc/mackie_control_protocol.md)
- [Official Behringer firmware-update video](https://www.youtube.com/watch?v=nElNot_WY34)
- [Firmware 1.22 release notes and downgrade warning](https://cdn-media.empowertribe.com/587016fdfd5f40bcb20df65fe262890e/Release-Notes_BE_0808-AAD_X-TOUCH_v1-22.pdf)

## License

MIT
