package disc

import "testing"

func TestParseTOC(t *testing.T) {
	toc := "track:   1 lba:         0\ntrack:   2 lba:     19838\ntrack:lout lba: 39168\n"
	tracks := parseTOC(toc)
	if len(tracks) != 2 {
		t.Fatalf("got %d tracks, want 2", len(tracks))
	}
	if tracks[0].Number != 1 || tracks[0].EndLBA != 19838 || tracks[1].EndLBA != 39168 {
		t.Fatalf("unexpected tracks: %+v", tracks)
	}
}
