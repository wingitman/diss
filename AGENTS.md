# diss Development Rules

This is a Delbysoft terminal UI. Before making changes, locate and read the
Delbyapps project's `tui-blueprint/blueprint.md` and
`tui-blueprint/checklist.md`. If this project was generated outside
the Delbyapps checkout, use the Delbyapps repository's configured blueprint
path instead.

The shared base features are requirements, not optional polish:

- Go, Bubble Tea, and Lip Gloss.
- Arrow and `hjkl` navigation, quit, confirm, cancel, paging, and help.
- Contextual hints with visibly highlighted keybindings.
- TOML config with remappable keys and `o` editor reload.
- Mouse hitboxes calculated from the same full layout as rendering.
- Resize-safe layouts that include headers, panels, footers, and hints.
- Non-blocking editor, clipboard, history, update, and rollback commands.
- Source-first installation with release-directory fallback.
- Linux, macOS, and Windows builds through `build-all`.
- Dirty-checkout warnings and detached-terminal update/rollback operations.

Keep domain logic separate from the Bubble Tea view. Preserve these behaviors
when adding screens or changing the visual design. Run `make test` and
`make build-all` after meaningful changes.
