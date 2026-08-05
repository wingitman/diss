# diss

`diss` is a Delbysoft terminal UI for inspecting, composing, extending, and
burning optical media.

Built with Go, Bubble Tea, and Lip Gloss. The project was created with
`delbyapps new` and follows the shared Delbysoft TUI blueprint.

## Usage

```sh
diss ~/Music/SUNO                 # audio-CD project
diss song-one.mp3 song-two.mp3   # audio-CD project
diss --data ~/Documents           # data-disc project
diss                                # open the file browser
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
| `n` | Open the file browser |
| `Space` | Mark/unmark the focused file |
| `A` | Mark all compatible files in the directory |
| `a` | Add all marked files to the project |
| `Backspace` | Browse the parent directory |
| `f` | Open a native multi-file chooser and append the results to the project |
| `x` | Remove the focused project item |
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
- Displays existing disc audio tracks beside newly selected project tracks.
- Converts MP3/WAV/FLAC sources to CD audio with `ffmpeg`.
- Burns finalized audio CDs with `cdrecord`.
- Appends audio to an open multisession audio disc with `cdrecord -multi`.
- Creates or appends DVD/BD data sessions with `growisofs`.
- Refuses to treat finalized media as appendable.
- Requires explicit confirmation before every write.
- Streams burn progress and tool output, with cancellation through `q`, `Ctrl+C`, or `Esc`.
- Verifies the disc after writing and reports completion or failure in the status/log area.

Finalized audio CDs cannot be extended in place. `diss` reports that state and
refuses a direct append rather than risking a failed write. Existing tracks
are still shown from the disc TOC; replacement-disc ripping is a separate
workflow because it requires a new blank disc.

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
artifacts under `releases/`; it does not change the installed command. `make
install` builds the current checkout from source when Go exists and atomically
replaces the command resolved from `PATH` (or `$HOME/.local/bin/diss` when no
existing command is found). When Go is unavailable, it installs only the exact
matching release artifact and fails if that artifact is missing. The installer
reports PATH conflicts so an older binary cannot be mistaken for the newly
built one. `install.ps1` provides the equivalent Windows behavior.

## Updates and configuration

The TOML file is created on first launch and missing keys are migrated without
discarding existing values. Opening it with `o` runs the configured editor
without blocking the event loop and reloads both key handling and hints when
the editor exits.

On KDE/Linux, the native multi-file chooser uses `kdialog` and falls back to
`zenity`. Repeating `f` appends more selections to the existing project.

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
