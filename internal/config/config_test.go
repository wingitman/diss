package config

import "testing"

func TestMigrateConflictingBrowserAndCancelKeys(t *testing.T) {
	cfg := migrateKeybinds(Config{Keybinds: Keybinds{Cancel: "n", Browse: "n"}})
	if cfg.Keybinds.Cancel != "esc" || cfg.Keybinds.Browse != "n" {
		t.Fatalf("unexpected migrated keys: %+v", cfg.Keybinds)
	}
}
