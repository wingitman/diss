# diss

`diss` is a Delbysoft terminal UI for inspecting, composing, extending, and
burning optical media. The name is intentional: when you diss someone, you
burn them.

Built with Go, Bubble Tea, and Lip Gloss. The project was created with
`delbyapps new` and follows the shared Delbysoft TUI blueprint.

## Usage

```sh
diss ~/Music/SUNO                 # audio-CD project
diss song-one.mp3 song-two.mp3   # audio-CD project
diss --data ~/Documents           # data-disc project
```

The main screen shows detected optical drives, inserted media, project
contents, capacity/status, and a bounded inspection viewport. Long
`cdrecord` output is clipped and scrolled rather than being allowed to escape
the terminal layout.

## Keybindings

All keys are remappable in `~/.config/delbysoft/diss.toml`. Defaults include:

| Key | Action |
| --- | --- |
| `j` / `k`, arrows | Navigate the focused list |
| `h` / `l`, `Tab` | Change focus |
| `pgup` / `pgdown` | Scroll inspection output |
| `g` / `G` | First/last item |
| `Enter` | Inspect the selected drive or confirm a burn |
| `i` | Reset/show inspection output |
| `a` / `d` | Select audio/data project |
| `b` | Open the destructive burn confirmation |
| `y` | Copy the selected path |
| `o` | Open and reload the TOML configuration |
| `?` | Help overlay |
| `U` | Check update state without installing |
| `I` | Confirm and launch the latest update in a detached terminal |
| `R` | Confirm and launch rollback to the previous history commit |
| `Esc` | Cancel/back/reset scroll |
| `q` / `Ctrl+C` | Quit |

Hints are contextual and key names are highlighted. The same calculated layout
drives terminal rendering and mouse hitboxes. Resize and small-terminal paths
are bounded by tests.

## Disc operations

- Inspects optical drives with `lsblk`, `udevadm`, and `cdrecord`.
- Reads media type, blank/finalized state, sessions, tracks, and raw details.
- Converts MP3/WAV/FLAC sources to CD audio with `ffmpeg`.
- Burns finalized audio CDs with `cdrecord`.
- Creates or appends DVD/BD data sessions with `growisofs`.
- Refuses to treat finalized media as appendable.
- Requires explicit confirmation before every write.

Linux is the first complete native backend. The domain interfaces and build
targets are cross-platform; macOS and Windows backends can be added without
changing the TUI or project model.

## Build and install

```sh
go mod tidy
make build
make test
make build-all
make install
```

`build-all` creates Linux amd64/arm64, macOS amd64/arm64, and Windows amd64
artifacts under `releases/`. `install.sh` builds from source when Go exists and
otherwise selects only the matching release artifact. `install.ps1` provides
the equivalent Windows behavior.

## Updates and configuration

The TOML file is created on first launch and missing keys are migrated without
discarding existing values. Opening it with `o` runs the configured editor
without blocking the event loop and reloads both key handling and hints when
the editor exits.

`U` performs a non-blocking fetch and source/update-state check, including dirty
checkout status and recent history. `I` and `R` require a clean checkout and
launch explicit update/rollback scripts in a detached terminal. The scripts
restore the original branch ref after installing the selected commit.

## Verification

```sh
go vet ./...
go test ./...
make build-all
```
