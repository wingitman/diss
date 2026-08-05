package browser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestScanFiltersAndSortsAudio(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"z.mp3", "a.wav", "ignore.txt", ".hidden.mp3", "Songs"} {
		path := filepath.Join(dir, name)
		if name == "Songs" {
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := Scan(context.Background(), dir, "audio")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || !entries[0].Directory || entries[1].Name != "a.wav" || entries[2].Name != "z.mp3" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}
