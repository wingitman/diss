package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/wingitman/diss/internal/config"
	"github.com/wingitman/diss/internal/disc"
	"github.com/wingitman/diss/internal/project"
)

func TestLayoutNeverCreatesNegativePanels(t *testing.T) {
	for _, size := range [][2]int{{20, 8}, {40, 8}, {40, 12}, {85, 24}, {140, 40}} {
		l := (layout{width: size[0], height: size[1]}).calculate()
		for name, panel := range map[string]rect{"left": l.left, "right": l.right, "footer": l.footer, "hints": l.hints} {
			if panel.width < 1 || panel.height < 1 {
				t.Fatalf("%s panel invalid at %dx%d: %+v", name, size[0], size[1], panel)
			}
		}
	}
}

func TestTruncateIsBounded(t *testing.T) {
	for width := 1; width < 20; width++ {
		if got := truncate(strings.Repeat("x", 100), width); len([]rune(got)) > width {
			t.Fatalf("width %d produced %q", width, got)
		}
	}
}

func TestWindowBounds(t *testing.T) {
	lines := []string{"one", "two", "three"}
	if got := window(lines, 100, 2); len(got) != 2 {
		t.Fatalf("got %d lines, want 2", len(got))
	}
	if got := window(lines, -4, 2); got[0] != "one" {
		t.Fatalf("unexpected first line: %q", got[0])
	}
}

func TestViewFitsSmallTerminal(t *testing.T) {
	for _, size := range [][2]int{{40, 8}, {40, 12}, {100, 20}, {140, 40}} {
		m := model{config: configForTest(), width: size[0], height: size[1], projectMode: "audio", detailLines: make([]string, 100), media: disc.Media{Present: true, Kind: "CD-R", State: "blank"}}
		for i := range m.detailLines {
			m.detailLines[i] = strings.Repeat("detail ", 20)
		}
		view := m.View()
		if got := lipgloss.Height(view); got > m.height {
			t.Fatalf("view height %d exceeds terminal height %d at %dx%d", got, m.height, m.width, m.height)
		}
		if got := lipgloss.Width(view); got > m.width {
			t.Fatalf("view width %d exceeds terminal width %d at %dx%d", got, m.width, m.width, m.height)
		}
	}
}

func TestWidePanelsShareTheSameVerticalRegion(t *testing.T) {
	m := model{config: configForTest(), width: 120, height: 30, projectMode: "audio"}
	l := m.layout()
	if l.narrow {
		t.Fatal("test requires a wide layout")
	}
	if l.left.y != l.right.y || l.left.height != l.right.height {
		t.Fatalf("panels are not aligned: left=%+v right=%+v", l.left, l.right)
	}
	view := m.View()
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("wide view height %d exceeds terminal height %d", got, m.height)
	}
}

func configForTest() config.Config { return config.Defaults() }

func TestMouseRowsStayInsideRenderedPanels(t *testing.T) {
	m := model{config: configForTest(), width: 100, height: 20, projectMode: "audio", drives: []disc.Drive{{Path: "/dev/sr0"}}, tracks: make([]project.Track, 30)}
	l := m.layoutWithRows()
	for _, row := range append(l.driveRows, l.trackRows...) {
		if !row.contains(row.x, row.y) {
			t.Fatalf("row does not contain its origin: %+v", row)
		}
		if row.y < l.left.y || row.y >= l.left.y+l.left.height && row.y < l.right.y || row.y >= l.right.y+l.right.height {
			t.Fatalf("row escaped panel bounds: %+v", row)
		}
	}
}
