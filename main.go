package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	charmansi "github.com/charmbracelet/x/ansi"
	"github.com/wingitman/diss/internal/browser"
	"github.com/wingitman/diss/internal/config"
	"github.com/wingitman/diss/internal/disc"
	"github.com/wingitman/diss/internal/project"
	"github.com/wingitman/diss/internal/update"
)

var version = "dev"
var commit = "dev"

type rect struct{ x, y, width, height int }

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.width && y >= r.y && y < r.y+r.height
}

type layout struct {
	width, height                      int
	header, left, right, footer, hints rect
	driveRows, trackRows, browserRows  []rect
	narrow                             bool
}

func (l layout) calculate() layout {
	if l.width < 1 {
		l.width = 80
	}
	if l.height < 1 {
		l.height = 24
	}
	l.header = rect{0, 0, l.width, 2}
	l.footer = rect{0, l.height - 2, l.width, 1}
	l.hints = rect{0, l.height - 1, l.width, 1}
	bodyY := l.header.y + l.header.height
	bodyH := l.height - l.header.height - l.footer.height - l.hints.height
	if bodyH < 4 {
		bodyH = 4
	}
	l.narrow = l.width < 86
	if l.narrow {
		leftH := max(1, bodyH/2)
		l.left = rect{1, bodyY, max(1, l.width-2), leftH}
		l.right = rect{1, l.left.y + l.left.height, max(1, l.width-2), max(1, bodyH-leftH)}
	} else {
		leftW := max(30, l.width*38/100)
		rightW := max(30, l.width-leftW-3)
		l.left = rect{1, bodyY, leftW, bodyH}
		l.right = rect{l.left.x + leftW + 2, bodyY, rightW, bodyH}
	}
	return l
}

type drivesMsg struct {
	drives []disc.Drive
	err    error
}

type mediaMsg struct {
	media disc.Media
	err   error
}

type tracksMsg struct {
	tracks []project.Track
	err    error
}

type burnMsg struct{ err error }
type burnEventMsg struct{ event project.BurnEvent }
type copyMsg struct{ err error }
type configMsg struct {
	cfg config.Config
	err error
}
type updateMsg struct {
	text  string
	state update.State
}
type browserMsg struct {
	directory string
	entries   []browser.Entry
	err       error
}
type explorerMsg struct{ err error }
type chooserMsg struct {
	paths     []string
	directory string
	err       error
}

type model struct {
	config        config.Config
	service       disc.Service
	drives        []disc.Drive
	media         disc.Media
	discTracks    []disc.Track
	tracks        []project.Track
	dataPaths     []string
	projectMode   string
	selected      int
	driveSelected int
	focus         int
	detailLines   []string
	scroll        int
	width         int
	height        int
	status        string
	err           error
	busy          bool
	confirm       bool
	confirmMode   string
	updateState   update.State
	burnCancel    context.CancelFunc
	burnEvents    <-chan project.BurnEvent
	burnPhase     string
	burnProgress  int
	burnLog       []string
	viewMode      string
	browseDir     string
	browseItems   []browser.Entry
	browseCursor  int
	marked        map[string]bool
	help          bool
	quitting      bool
}

func newModel(args []string) model {
	cfg, cfgErr := config.Load()
	m := model{config: cfg, service: disc.NewService(disc.CommandRunner{}), projectMode: "audio", viewMode: "project", marked: map[string]bool{}, status: "Scanning optical drives…", err: cfgErr}
	if len(args) > 0 && args[0] == "--data" {
		m.projectMode = "data"
		m.dataPaths = append([]string(nil), args[1:]...)
	} else if len(args) > 0 {
		tracks, err := project.Audio(args)
		m.tracks, m.err = tracks, err
	}
	return m
}

func (m model) Init() tea.Cmd {
	commands := []tea.Cmd{m.refresh(), m.probeTracks()}
	return tea.Batch(commands...)
}

func (m model) refresh() tea.Cmd {
	return func() tea.Msg {
		drives, err := m.service.Drives(context.Background())
		return drivesMsg{drives: drives, err: err}
	}
}

func (m model) inspect() tea.Cmd {
	drive, ok := m.selectedDrive()
	if !ok {
		return nil
	}
	path := drive.Path
	return func() tea.Msg {
		media, err := m.service.Inspect(context.Background(), path)
		return mediaMsg{media: media, err: err}
	}
}

func (m model) probeTracks() tea.Cmd {
	if len(m.tracks) == 0 {
		return nil
	}
	tracks := append([]project.Track(nil), m.tracks...)
	return func() tea.Msg {
		tracks, err := project.Probe(context.Background(), tracks)
		return tracksMsg{tracks: tracks, err: err}
	}
}

func (m model) scanDirectory(directory string) tea.Cmd {
	mode := m.projectMode
	return func() tea.Msg {
		entries, err := browser.Scan(context.Background(), directory, mode)
		return browserMsg{directory: directory, entries: entries, err: err}
	}
}

func (m *model) openBrowser() tea.Cmd {
	directory := m.browseDir
	if directory == "" {
		directory, _ = os.Getwd()
	}
	m.viewMode, m.browseCursor, m.busy = "browser", 0, true
	m.status = "Scanning " + directory
	return m.scanDirectory(directory)
}

func (m model) openExplorer() tea.Cmd {
	directory := m.browseDir
	if directory == "" {
		directory, _ = os.Getwd()
	}
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("explorer", directory)
		case "darwin":
			cmd = exec.Command("open", directory)
		default:
			cmd = exec.Command("xdg-open", directory)
		}
		return explorerMsg{err: cmd.Start()}
	}
}

func (m model) chooseFiles() tea.Cmd {
	return m.runChooser(false, false)
}

func (m model) chooseDirectory(changeOnly bool) tea.Cmd {
	return m.runChooser(true, changeOnly)
}

func (m model) runChooser(directory, changeOnly bool) tea.Cmd {
	start := m.browseDir
	if start == "" {
		start, _ = os.Getwd()
	}
	return func() tea.Msg {
		paths, selectedDir, err := chooseNative(start, directory)
		if err != nil {
			return chooserMsg{err: err}
		}
		if changeOnly {
			return chooserMsg{directory: selectedDir}
		}
		return chooserMsg{paths: paths, directory: selectedDir}
	}
}

func chooseNative(start string, directory bool) ([]string, string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		return nil, "", fmt.Errorf("native chooser is not implemented on Windows yet")
	case "darwin":
		return nil, "", fmt.Errorf("native chooser is not implemented on macOS yet")
	default:
		if _, err := exec.LookPath("kdialog"); err == nil {
			if directory {
				cmd = exec.Command("kdialog", "--getexistingdirectory", start)
			} else {
				cmd = exec.Command("kdialog", "--getopenfilename", start, "*.mp3 *.wav *.flac|Audio files\n*|All files", "--multiple", "--separate-output")
			}
		} else if _, err := exec.LookPath("zenity"); err == nil {
			if directory {
				cmd = exec.Command("zenity", "--file-selection", "--directory", "--filename="+filepath.Join(start, ""))
			} else {
				cmd = exec.Command("zenity", "--file-selection", "--multiple", "--separator=\n", "--filename="+filepath.Join(start, ""))
			}
		} else {
			return nil, "", fmt.Errorf("no native file chooser found; install kdialog or zenity")
		}
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, "", nil // Closing the chooser is a normal cancellation.
	}
	values := splitChooserOutput(string(out))
	if directory {
		if len(values) == 0 {
			return nil, "", nil
		}
		return nil, values[0], nil
	}
	return values, "", nil
}

func splitChooserOutput(output string) []string {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}

func (m *model) addMarked() tea.Cmd {
	if len(m.marked) == 0 {
		m.status = "Mark files with space before adding them"
		return nil
	}
	paths := make([]string, 0, len(m.marked))
	for _, entry := range m.browseItems {
		if m.marked[entry.Path] {
			paths = append(paths, entry.Path)
		}
	}
	m.addPaths(paths)
	m.marked = map[string]bool{}
	if m.projectMode == "audio" {
		m.busy = true
		m.err = nil
		return m.probeTracks()
	}
	return nil
}

func (m *model) addPaths(paths []string) {
	if m.projectMode == "audio" {
		for _, path := range paths {
			tracks, err := project.Audio([]string{path})
			if err != nil {
				m.err = err
				continue
			}
			m.tracks = appendUniqueTracks(m.tracks, tracks)
		}
	} else {
		for _, path := range paths {
			if !contains(m.dataPaths, path) {
				m.dataPaths = append(m.dataPaths, path)
			}
		}
	}
	m.viewMode, m.focus = "project", 1
	m.selected = max(0, m.itemCount()-1)
	m.status = fmt.Sprintf("Added %d item(s) to project", len(paths))
}

func appendUniqueTracks(existing, added []project.Track) []project.Track {
	for _, track := range added {
		found := false
		for _, current := range existing {
			if current.Path == track.Path {
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, track)
		}
	}
	return existing
}

func contains(paths []string, target string) bool {
	for _, path := range paths {
		if path == target {
			return true
		}
	}
	return false
}

func (m *model) burn() tea.Cmd {
	drive, ok := m.selectedDrive()
	if !ok {
		return nil
	}
	device := drive.Path
	tracks := append([]project.Track(nil), m.tracks...)
	paths := append([]string(nil), m.dataPaths...)
	appendSession := m.media.Present && m.media.State != "blank" && !m.media.Finalized && len(m.discTracks) > 0
	ctx, cancel := context.WithCancel(context.Background())
	m.burnCancel = cancel
	events := make(chan project.BurnEvent, 32)
	m.burnEvents = events
	go func() {
		err := project.RunBurn(ctx, device, tracks, paths, appendSession, func(event project.BurnEvent) { events <- event })
		events <- project.BurnEvent{Done: true, Err: err}
		close(events)
	}()
	return waitBurnEvent(events)
}

func waitBurnEvent(events <-chan project.BurnEvent) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-events
		if !ok {
			return burnEventMsg{event: project.BurnEvent{Done: true}}
		}
		return burnEventMsg{event: event}
	}
}

func (m model) selectedDrive() (disc.Drive, bool) {
	if len(m.drives) == 0 {
		return disc.Drive{}, false
	}
	return m.drives[min(max(0, m.driveSelected), len(m.drives)-1)], true
}

func (m model) openConfig() tea.Cmd {
	editor, args, err := config.ResolveEditor()
	if err != nil {
		return func() tea.Msg { return configMsg{err: err} }
	}
	cmd := exec.Command(editor, append(args, config.Path())...)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		cfg, loadErr := config.Load()
		if err != nil {
			loadErr = err
		}
		return configMsg{cfg: cfg, err: loadErr}
	})
}

func (m model) copySelected() tea.Cmd {
	text := ""
	if m.projectMode == "audio" && len(m.tracks) > 0 {
		text = m.tracks[min(m.selected, len(m.tracks)-1)].Path
	} else if m.projectMode == "data" && len(m.dataPaths) > 0 {
		text = m.dataPaths[min(m.selected, len(m.dataPaths)-1)]
	}
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("pbcopy")
		case "windows":
			cmd = exec.Command("clip")
		default:
			if _, err := exec.LookPath("wl-copy"); err == nil {
				cmd = exec.Command("wl-copy")
			} else {
				cmd = exec.Command("xclip", "-selection", "clipboard")
			}
		}
		cmd.Stdin = strings.NewReader(text)
		err := cmd.Run()
		return copyMsg{err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case drivesMsg:
		m.busy, m.drives, m.err = false, msg.drives, msg.err
		m.driveSelected = min(m.driveSelected, max(0, len(m.drives)-1))
		if m.err != nil {
			m.status = "Drive discovery failed"
		} else if len(m.drives) == 0 {
			m.status = "No optical drives detected"
		} else {
			m.status = fmt.Sprintf("%d optical drive(s) found", len(m.drives))
			m.busy = true
			return m, m.inspect()
		}
	case mediaMsg:
		m.busy, m.media, m.err = false, msg.media, msg.err
		m.discTracks = append([]disc.Track(nil), msg.media.TracksInfo...)
		m.detailLines = append([]string(nil), msg.media.Details...)
		m.scroll = 0
		if m.err != nil {
			m.status = "Media inspection failed"
		} else if !m.media.Present {
			m.status = "Insert a disc to inspect or burn"
		} else {
			m.status = fmt.Sprintf("%s · %s", m.media.Kind, m.media.State)
			if len(m.media.Warnings) > 0 {
				m.status = fmt.Sprintf("%s · %d media warning(s)", m.status, len(m.media.Warnings))
			}
		}
	case browserMsg:
		m.busy, m.browseDir, m.browseItems, m.err = false, msg.directory, msg.entries, msg.err
		m.browseCursor = min(m.browseCursor, max(0, len(m.browseItems)-1))
		if m.err != nil {
			m.status = "File browser failed"
		} else {
			m.status = fmt.Sprintf("%d item(s) in %s", len(m.browseItems), msg.directory)
		}
	case explorerMsg:
		if msg.err != nil {
			m.status = "Could not open the default file explorer"
		} else {
			m.status = "Opened the current directory in the default file explorer"
		}
	case chooserMsg:
		m.busy = false
		if msg.err != nil {
			m.status = msg.err.Error()
		} else if msg.directory != "" {
			m.browseDir, m.viewMode, m.browseCursor, m.busy = msg.directory, "browser", 0, true
			m.status = "Scanning " + msg.directory
			return m, m.scanDirectory(msg.directory)
		} else if len(msg.paths) > 0 {
			m.addPaths(msg.paths)
			if m.projectMode == "audio" {
				m.busy = true
				return m, m.probeTracks()
			}
		} else {
			m.status = "Chooser cancelled"
		}
	case tracksMsg:
		m.tracks, m.err = msg.tracks, msg.err
		if m.err != nil {
			m.status = "Audio metadata failed"
		} else if len(m.tracks) > 0 {
			m.status = fmt.Sprintf("Loaded %d audio track(s)", len(m.tracks))
		}
	case burnEventMsg:
		event := msg.event
		if event.Phase != "" {
			m.burnPhase = event.Phase
			m.status = event.Phase
		}
		if event.Progress > m.burnProgress {
			m.burnProgress = event.Progress
		}
		if event.Line != "" {
			m.burnLog = append(m.burnLog, event.Line)
			if len(m.burnLog) > 12 {
				m.burnLog = m.burnLog[len(m.burnLog)-12:]
			}
		}
		if event.Done {
			m.busy, m.burnCancel, m.burnEvents = false, nil, nil
			if event.Err != nil {
				m.err = event.Err
				m.status = "Burn failed: " + event.Err.Error()
			} else {
				m.burnProgress = 100
				m.status = "Burn completed successfully"
			}
		} else {
			return m, waitBurnEvent(m.burnEvents)
		}
	case burnMsg:
		m.busy, m.confirm, m.err = false, false, msg.err
		if m.err != nil {
			m.status = "Burn failed"
		} else {
			m.status = "Burn complete; disc finalized or session appended"
			m.busy = true
			return m, m.inspect()
		}
	case copyMsg:
		if msg.err != nil {
			m.status = "Clipboard unavailable"
		} else {
			m.status = "Copied selected path"
		}
	case configMsg:
		m.busy = false
		m.config, m.err = msg.cfg, msg.err
		if m.err != nil {
			m.status = "Config reload failed"
		} else {
			m.status = "Configuration reloaded; hints updated"
		}
	case updateMsg:
		m.busy, m.status = false, msg.text
		m.updateState = msg.state
	case tea.MouseMsg:
		return m, m.handleMouse(msg)
	case tea.KeyMsg:
		return m, m.handleKey(msg.String())
	}
	return m, nil
}

func (m *model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	l := m.layoutWithRows()
	if m.viewMode == "browser" && msg.Type == tea.MouseWheelUp {
		m.browseCursor = max(0, m.browseCursor-3)
		return nil
	}
	if m.viewMode == "browser" && msg.Type == tea.MouseWheelDown {
		m.browseCursor = min(max(0, len(m.browseItems)-1), m.browseCursor+3)
		return nil
	}
	if msg.Type == tea.MouseWheelUp {
		m.scroll = max(0, m.scroll-3)
		return nil
	}
	if msg.Type == tea.MouseWheelDown {
		m.scroll += 3
		return nil
	}
	if msg.Type != tea.MouseLeft {
		return nil
	}
	if m.viewMode == "browser" {
		start, _ := visibleWindow(len(m.browseItems), m.browseCursor, browserItemCapacity(l.right))
		for index, row := range l.browserRows {
			if row.contains(msg.X, msg.Y) {
				itemIndex := start + index
				m.browseCursor = itemIndex
				m.marked[m.browseItems[itemIndex].Path] = !m.marked[m.browseItems[itemIndex].Path]
				return nil
			}
		}
		return nil
	}
	for index, row := range l.driveRows {
		if row.contains(msg.X, msg.Y) {
			m.selected, m.driveSelected, m.focus = index, index, 0
			m.busy = true
			return m.inspect()
		}
	}
	for index, row := range l.trackRows {
		if row.contains(msg.X, msg.Y) {
			m.selected, m.focus = index, 1
			return nil
		}
	}
	if l.footer.contains(msg.X, msg.Y) {
		m.help = true
	}
	if l.hints.contains(msg.X, msg.Y) {
		m.help = true
	}
	return nil
}

func (m *model) handleKey(key string) tea.Cmd {
	k := m.config.Keybinds
	if m.help {
		if key == k.Cancel || key == k.Help || key == k.Quit {
			m.help = false
		}
		return nil
	}
	if m.confirm {
		switch key {
		case k.Confirm, "enter":
			m.busy, m.confirm = true, false
			if m.confirmMode == "burn" {
				m.status = "Preparing media and burning…"
				return m.burn()
			}
			return m.launchUpdate(m.confirmMode == "latest")
		case k.Cancel:
			m.confirm = false
			m.status = "Burn cancelled"
		}
		return nil
	}
	if m.burnCancel != nil {
		if key == k.Quit || key == "ctrl+c" || key == k.Cancel {
			m.burnCancel()
			m.status = "Cancelling burn…"
			return nil
		}
		return nil
	}
	if m.viewMode == "browser" {
		switch key {
		case k.Cancel:
			m.viewMode = "project"
			m.marked = map[string]bool{}
			m.status = "Returned to project"
		case k.Up, "up":
			m.browseCursor = max(0, m.browseCursor-1)
		case k.Down, "down":
			m.browseCursor = min(max(0, len(m.browseItems)-1), m.browseCursor+1)
		case k.PageUp:
			m.browseCursor = max(0, m.browseCursor-8)
		case k.PageDown:
			m.browseCursor = min(max(0, len(m.browseItems)-1), m.browseCursor+8)
		case k.First:
			m.browseCursor = 0
		case k.Last:
			m.browseCursor = max(0, len(m.browseItems)-1)
		case k.Parent:
			parent := filepath.Dir(m.browseDir)
			if parent != m.browseDir {
				m.busy = true
				return m.scanDirectory(parent)
			}
		case k.Explorer:
			return m.openExplorer()
		case k.ChooseFiles:
			return m.chooseFiles()
		case k.ChooseDir:
			return m.chooseDirectory(false)
		case k.ChangeDir:
			return m.chooseDirectory(true)
		case k.Confirm:
			if len(m.browseItems) > 0 {
				entry := m.browseItems[m.browseCursor]
				if entry.Directory {
					m.busy = true
					return m.scanDirectory(entry.Path)
				}
				m.marked[entry.Path] = !m.marked[entry.Path]
			}
		case k.Mark:
			if len(m.browseItems) > 0 {
				entry := m.browseItems[m.browseCursor]
				m.marked[entry.Path] = !m.marked[entry.Path]
			}
		case k.SelectAll:
			for _, entry := range m.browseItems {
				if !entry.Directory {
					m.marked[entry.Path] = true
				}
			}
		case k.AddMarked:
			return m.addMarked()
		}
		return nil
	}
	switch key {
	case k.Quit, "ctrl+c":
		m.quitting = true
		return tea.Quit
	case k.Help:
		m.help = true
	case k.OpenConfig:
		m.status = "Opening configuration…"
		return m.openConfig()
	case k.ChooseFiles:
		return m.chooseFiles()
	case k.Refresh:
		m.busy, m.err, m.status = true, nil, "Refreshing optical drives…"
		return m.refresh()
	case k.Inspect:
		m.scroll = 0
		m.busy = true
		m.status = "Showing media inspection"
		return m.inspect()
	case k.Copy:
		return m.copySelected()
	case k.Updates:
		m.busy = true
		return updateCommand(m.config.Updates.RepoPath)
	case k.Install:
		if m.updateState.Repo == "" || m.updateState.Remote == "" || m.updateState.Remote == m.updateState.Current {
			m.status = "No update is available"
		} else if m.updateState.Dirty {
			m.status = "Cannot update a dirty checkout"
		} else {
			m.confirm, m.confirmMode = true, "latest"
		}
	case k.Rollback:
		if len(m.updateState.History) < 2 {
			m.status = "No older commit is available"
		} else if m.updateState.Dirty {
			m.status = "Cannot rollback a dirty checkout"
		} else {
			m.confirm, m.confirmMode = true, "rollback"
		}
	case k.Audio:
		m.projectMode = "audio"
		m.selected = min(m.selected, max(0, len(m.tracks)-1))
		m.status = "Audio project selected"
	case k.Data:
		m.projectMode = "data"
		m.selected = min(m.selected, max(0, len(m.dataPaths)-1))
		m.status = "Data project selected"
	case k.Burn:
		if m.media.Finalized && m.projectMode == "audio" && len(m.discTracks) > 0 {
			m.status = "This audio CD is finalized; direct append is unavailable"
		} else if m.canBurn() {
			m.confirm, m.confirmMode = true, "burn"
		} else {
			m.status = "Select a project and insert writable media first"
		}
	case k.Remove:
		if m.focus == 1 {
			m.removeSelected()
		}
	case k.Confirm:
		if m.focus == 0 && len(m.drives) > 0 {
			m.selected = m.driveSelected
			m.busy = true
			return m.inspect()
		}
	case k.Up, "up":
		m.move(-1)
	case k.Down, "down":
		m.move(1)
	case k.PageUp:
		m.scroll = max(0, m.scroll-8)
	case k.PageDown:
		m.scroll += 8
	case k.First:
		m.selected = 0
		if m.focus == 0 {
			m.driveSelected = 0
		}
	case k.Last:
		if count := m.itemCount(); count > 0 {
			m.selected = count - 1
			if m.focus == 0 {
				m.driveSelected = m.selected
			}
		}
	case k.Left, "tab":
		m.focus = max(0, m.focus-1)
		if m.focus == 0 {
			m.selected = m.driveSelected
		}
	case k.Right, "shift+tab":
		m.focus = min(1, m.focus+1)
	case k.Cancel:
		m.scroll = 0
	}
	return nil
}

func (m *model) move(delta int) {
	count := m.itemCount()
	if count == 0 {
		return
	}
	if m.focus == 0 {
		m.driveSelected = (m.driveSelected + delta + count) % count
		m.selected = m.driveSelected
		return
	}
	m.selected = (m.selected + delta + count) % count
}

func (m model) itemCount() int {
	if m.focus == 0 {
		return len(m.drives)
	}
	if m.projectMode == "audio" {
		return len(m.tracks)
	}
	return len(m.dataPaths)
}

func (m *model) removeSelected() {
	if m.projectMode == "audio" {
		if m.selected < 0 || m.selected >= len(m.tracks) {
			return
		}
		removed := m.tracks[m.selected].Title
		m.tracks = append(m.tracks[:m.selected], m.tracks[m.selected+1:]...)
		m.selected = min(m.selected, max(0, len(m.tracks)-1))
		m.status = "Removed " + removed
		return
	}
	if m.selected < 0 || m.selected >= len(m.dataPaths) {
		return
	}
	removed := m.dataPaths[m.selected]
	m.dataPaths = append(m.dataPaths[:m.selected], m.dataPaths[m.selected+1:]...)
	m.selected = min(m.selected, max(0, len(m.dataPaths)-1))
	m.status = "Removed " + filepath.Base(removed)
}

func (m model) canBurn() bool {
	if len(m.drives) == 0 || !m.media.Present || !m.media.Writable {
		return false
	}
	return (m.projectMode == "audio" && len(m.tracks) > 0) || (m.projectMode == "data" && len(m.dataPaths) > 0)
}

func (m *model) launchUpdate(latest bool) tea.Cmd {
	target := m.updateState.Remote
	if !latest && len(m.updateState.History) > 1 {
		target = m.updateState.History[1]
	}
	repo, terminal := m.updateState.Repo, m.config.Updates.Terminal
	return func() tea.Msg {
		err := update.LaunchDetached(repo, target, terminal, latest)
		if err != nil {
			return updateMsg{text: "Could not launch detached update: " + err.Error(), state: m.updateState}
		}
		return updateMsg{text: "Update helper launched in a detached terminal", state: m.updateState}
	}
}

func (m model) layout() layout {
	return layout{width: m.width, height: m.height}.calculate()
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	l := m.layout()
	if l.width < 20 || l.height < 8 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F0A47C")).Render("diss needs a terminal at least 20x8")
	}
	if l.height < 12 {
		compact := []string{
			truncate("diss · "+m.projectMode+" project", l.width),
			truncate(m.status, l.width),
			truncate(hint(m.config.Keybinds.Quit, "quit")+"  "+hint(m.config.Keybinds.Help, "help"), l.width),
		}
		return strings.Join(compact, "\n")
	}
	header := m.renderHeader(l)
	left := m.renderDrivePanel(l)
	right := m.renderProjectPanel(l)
	if m.viewMode == "browser" {
		right = m.renderBrowserPanel(l)
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	if l.narrow {
		body = lipgloss.JoinVertical(lipgloss.Left, left, right)
	}
	if m.help {
		body = m.renderHelp(l.width, max(1, l.height-l.header.height-l.footer.height-l.hints.height))
	} else if m.confirm {
		body = m.renderConfirm(l.width, max(1, l.height-l.header.height-l.footer.height-l.hints.height))
	}
	footer := m.renderFooter(l)
	hints := m.renderHints(l)
	return header + "\n" + body + "\n" + footer + "\n" + hints
}

func (m model) renderHeader(l layout) string {
	brand := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Render("delby") + lipgloss.NewStyle().Foreground(lipgloss.Color("#5865F2")).Bold(true).Render("soft")
	label := brand + "  /  diss · optical media studio  [" + m.projectMode + "]  v" + version
	return lipgloss.NewStyle().Width(max(1, l.width-2)).Background(lipgloss.Color("#1A1A2E")).Foreground(lipgloss.Color("#7C9EF0")).Bold(true).Padding(0, 1).Render(truncate(label, max(1, l.width-2)))
}

func (m model) renderDrivePanel(l layout) string {
	inner := l.left.width - 4
	lines := []string{accent("DRIVES")}
	l.driveRows = nil
	if len(m.drives) == 0 {
		lines = append(lines, muted("No optical drives detected"))
	} else {
		for index, drive := range m.drives {
			name := strings.TrimSpace(strings.Join([]string{drive.Vendor, drive.Model}, " "))
			if name == "" {
				name = drive.Path
			}
			line := fmt.Sprintf("%s %s", map[bool]string{true: "▶", false: " "}[index == m.selected && m.focus == 0], name)
			if index == m.selected && m.focus == 0 {
				line = selected(line, inner)
			}
			lines = append(lines, line, muted("  "+drive.Path))
		}
	}
	lines = append(lines, "", accent("MEDIA"))
	if !m.media.Present {
		lines = append(lines, muted("No disc inserted"))
	} else {
		lines = append(lines, fmt.Sprintf("%s  ·  %s", accent(m.media.Kind), m.media.State))
		lines = append(lines, fmt.Sprintf("sessions %d  ·  tracks %d", m.media.Sessions, m.media.Tracks))
		if m.media.Finalized {
			lines = append(lines, muted("Finalized · append unavailable"))
		} else if m.media.Writable {
			lines = append(lines, success("Writable · append may be available"))
		}
	}
	for _, warning := range m.media.Warnings {
		lines = append(lines, warningStyle("warning: "+warning))
	}
	return box("DRIVES", lines, l.left)
}

func (m model) renderProjectPanel(l layout) string {
	inner := l.right.width - 4
	lines := []string{accent(strings.ToUpper(m.projectMode) + " PROJECT")}
	trackRows := []rect{}
	if m.projectMode == "audio" {
		if len(m.discTracks) > 0 {
			lines = append(lines, "", accent("DISC CONTENTS"))
			for _, track := range m.discTracks {
				title := track.Title
				if title == "" {
					title = fmt.Sprintf("Track %02d", track.Number)
				}
				lines = append(lines, fmt.Sprintf("DISC %02d  %-*s %s", track.Number, max(1, inner-24), truncate(title, max(1, inner-24)), duration(track.Duration)))
			}
			if m.media.Finalized {
				lines = append(lines, muted("finalized · direct append unavailable"))
			}
			lines = append(lines, "", accent("NEW TRACKS"))
		}
		if len(m.tracks) == 0 {
			lines = append(lines, muted("Press f to select one or more songs"))
		} else {
			for index, track := range m.tracks {
				line := fmt.Sprintf("%s%02d  %-*s %s", map[bool]string{true: "▶ ", false: "  "}[index == m.selected && m.focus == 1], index+1, max(1, inner-26), truncate(track.Title, max(1, inner-26)), duration(track.Duration))
				if index == m.selected && m.focus == 1 {
					line = selected(line, inner)
				}
				lines = append(lines, line)
				trackRows = append(trackRows, rect{l.right.x + 2, l.right.y + 1 + projectTrackStartLines(m) + index, inner, 1})
			}
			lines = append(lines, "", muted(fmt.Sprintf("total %s / 80:00", duration(project.TotalDuration(m.tracks)))))
		}
	} else {
		if len(m.dataPaths) == 0 {
			lines = append(lines, muted("Press f to select data files"))
		} else {
			for index, path := range m.dataPaths {
				line := fmt.Sprintf("%s%02d  %s", map[bool]string{true: "▶ ", false: "  "}[index == m.selected && m.focus == 1], index+1, truncate(path, inner-5))
				if index == m.selected && m.focus == 1 {
					line = selected(line, inner)
				}
				lines = append(lines, line)
				trackRows = append(trackRows, rect{l.right.x + 2, l.right.y + 1 + projectTrackStartLines(m) + index, inner, 1})
			}
		}
	}
	if len(m.detailLines) > 0 {
		lines = append(lines, "", accent("INSPECTION"))
		available := max(1, l.right.height-len(lines)-3)
		for _, line := range window(m.detailLines, m.scroll, available) {
			lines = append(lines, muted(truncate(line, inner)))
		}
	}
	if m.burnCancel != nil {
		lines = append(lines, "", accent("BURN PROGRESS"), result(progressBar(m.burnProgress, inner)), muted(m.burnPhase))
		for _, line := range m.burnLog {
			lines = append(lines, muted(truncate(line, inner)))
		}
	}
	return box("PROJECT", lines, l.right)
}

func projectTrackStartLines(m model) int {
	lines := 1
	if len(m.discTracks) > 0 && m.projectMode == "audio" {
		lines += 2 + len(m.discTracks)
		if m.media.Finalized {
			lines++
		}
		lines += 2
	}
	return lines
}

func (m model) renderBrowserPanel(l layout) string {
	inner := l.right.width - 4
	lines := []string{accent("ADD TO " + strings.ToUpper(m.projectMode) + " PROJECT"), muted(truncate(m.browseDir, inner-2)), ""}
	start, end := visibleWindow(len(m.browseItems), m.browseCursor, browserItemCapacity(l.right))
	for index := start; index < end; index++ {
		entry := m.browseItems[index]
		mark := "[ ]"
		if m.marked[entry.Path] {
			mark = "[x]"
		}
		icon := "  "
		if entry.Directory {
			icon = "▸ "
		}
		line := fmt.Sprintf("%s %s%s", mark, icon, truncate(entry.Name, inner-8))
		if index == m.browseCursor {
			line = selected(line, inner)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", accent(fmt.Sprintf("%d marked  ·  e explorer", len(m.marked))))
	return box("BROWSER", lines, l.right)
}

func (m *model) renderFooter(l layout) string {
	status := m.status
	if m.err != nil {
		status = "error: " + m.err.Error()
	}
	if m.busy {
		status = "◌ " + status
	}
	if m.burnCancel != nil {
		status += fmt.Sprintf("  %d%%", m.burnProgress)
	}
	return lipgloss.NewStyle().Width(max(1, l.width-1)).Foreground(lipgloss.Color("#666688")).Render(truncate(status, max(1, l.width-1)))
}

func (m model) renderHints(l layout) string {
	if !m.config.UI.ShowHints {
		return ""
	}
	if m.viewMode == "browser" {
		parts := []string{hint(m.config.Keybinds.ChooseFiles, "choose files"), hint(m.config.Keybinds.ChooseDir, "choose dir"), hint(m.config.Keybinds.ChangeDir, "change dir"), hint(m.config.Keybinds.Mark, "mark"), hint(m.config.Keybinds.AddMarked, "add marked"), hint(m.config.Keybinds.Parent, "parent"), hint(m.config.Keybinds.Cancel, "back")}
		return lipgloss.NewStyle().Width(max(1, l.width-1)).Foreground(lipgloss.Color("#666688")).Render(truncate(strings.Join(parts, "  "), max(1, l.width-1)))
	}
	parts := []string{hint(m.config.Keybinds.ChooseFiles, "select files"), hint(m.config.Keybinds.Up, "move"), hint(m.config.Keybinds.PageDown, "page"), hint(m.config.Keybinds.Inspect, "inspect"), hint(m.config.Keybinds.Burn, "burn"), hint(m.config.Keybinds.Help, "help"), hint(m.config.Keybinds.Quit, "quit")}
	return lipgloss.NewStyle().Width(max(1, l.width-1)).Foreground(lipgloss.Color("#666688")).Render(truncate(strings.Join(parts, "  "), max(1, l.width-1)))
}

func hint(key, action string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#7C9EF0")).Bold(true).Render(key) + " " + action
}

func (m model) renderHelp(width, height int) string {
	lines := []string{accent("HELP"), "", "Navigate: ↑/↓ or j/k · switch panel: h/l or tab", "Page: pgup/pgdown · first/last: g/G · confirm: enter", "Cancel/back: esc · inspect: i · config: o · copy: y", "Refresh: r · burn: b · updates: U · quit: q · close: ?/esc"}
	return box("HELP", lines, rect{1, 0, max(1, width-2), max(1, height)})
}

func (m model) renderConfirm(width, height int) string {
	name := "audio CD"
	if m.confirmMode == "latest" {
		name = "latest source update"
	} else if m.confirmMode == "rollback" {
		name = "older source commit rollback"
	} else if m.projectMode == "data" {
		name = "data disc session"
	}
	warning := "This operation permanently writes the inserted media."
	if m.confirmMode != "burn" {
		warning = "This starts an explicit detached source operation."
	}
	confirmKeys := "enter"
	if m.config.Keybinds.Confirm != "" && m.config.Keybinds.Confirm != "enter" {
		confirmKeys += "/" + m.config.Keybinds.Confirm
	}
	lines := []string{accent("CONFIRM " + strings.ToUpper(name) + "?"), "", warning, "", primary(confirmKeys) + " confirm    " + muted("esc cancel")}
	return box("CONFIRM", lines, rect{1, 0, max(1, min(width-2, 76)), max(1, height)})
}

func box(title string, lines []string, r rect) string {
	inner := max(1, r.width-4)
	clipWidth := max(1, inner-2)
	contentH := max(1, r.height-2)
	fit := make([]string, 0, contentH)
	fit = append(fit, accent(title))
	for _, line := range lines {
		if len(fit) >= contentH {
			break
		}
		fit = append(fit, truncate(line, clipWidth))
	}
	for len(fit) < contentH {
		fit = append(fit, "")
	}
	return lipgloss.NewStyle().Width(inner).Height(contentH).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#444466")).Padding(0, 1).Render(strings.Join(fit, "\n"))
}

func (m model) layoutWithRows() layout {
	l := m.layout()
	l.driveRows = make([]rect, len(m.drives))
	for index := range l.driveRows {
		l.driveRows[index] = rect{l.left.x + 2, l.left.y + 2 + index*2, max(1, l.left.width-4), 1}
	}
	count := len(m.dataPaths)
	if m.projectMode == "audio" {
		count = len(m.tracks)
	}
	l.trackRows = make([]rect, count)
	for index := range l.trackRows {
		offset := 1
		if m.projectMode == "audio" {
			offset = projectTrackStartLines(m)
		}
		l.trackRows[index] = rect{l.right.x + 2, l.right.y + 1 + offset + index, max(1, l.right.width-4), 1}
	}
	l.browserRows = make([]rect, len(m.browseItems))
	start, end := visibleWindow(len(m.browseItems), m.browseCursor, browserItemCapacity(l.right))
	l.browserRows = make([]rect, end-start)
	for index := start; index < end; index++ {
		l.browserRows[index-start] = rect{l.right.x + 2, l.right.y + 4 + index - start, max(1, l.right.width-4), 1}
	}
	l.driveRows = visibleRows(l.driveRows, l.left)
	l.trackRows = visibleRows(l.trackRows, l.right)
	l.browserRows = visibleRows(l.browserRows, l.right)
	return l
}

func browserItemCapacity(panel rect) int {
	return max(1, panel.height-8)
}

func visibleWindow(total, cursor, capacity int) (int, int) {
	if total <= 0 || capacity <= 0 {
		return 0, 0
	}
	cursor = min(max(0, cursor), total-1)
	start := (cursor / capacity) * capacity
	end := min(total, start+capacity)
	return start, end
}

func visibleRows(rows []rect, panel rect) []rect {
	visible := rows[:0]
	for _, row := range rows {
		if row.y >= panel.y+1 && row.y < panel.y+panel.height-1 {
			visible = append(visible, row)
		}
	}
	return visible
}

func updateCommand(repo string) tea.Cmd {
	return func() tea.Msg {
		if _, err := exec.LookPath("git"); err != nil {
			return updateMsg{text: "Git unavailable; updates disabled"}
		}
		state, err := update.Check(context.Background(), repo)
		if err != nil {
			return updateMsg{text: err.Error(), state: state}
		}
		if state.Dirty {
			return updateMsg{text: fmt.Sprintf("Updates available but checkout is dirty · %s", state.Repo), state: state}
		}
		if state.Remote != "" && state.Remote != state.Current {
			return updateMsg{text: fmt.Sprintf("Update available · current %.8s · latest %.8s", state.Current, state.Remote), state: state}
		}
		return updateMsg{text: fmt.Sprintf("Up to date · %.8s · %d history entries", state.Current, len(state.History)), state: state}
	}
}

func primary(value string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#7C9EF0")).Bold(true).Render(value)
}
func accent(value string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#F0A47C")).Bold(true).Render(value)
}
func muted(value string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#666688")).Render(value)
}
func result(value string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#B0B0CC")).Render(value)
}
func success(value string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#7CF09C")).Render(value)
}

func progressBar(progress, width int) string {
	width = max(8, width)
	filled := width * max(0, min(100, progress)) / 100
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func warningStyle(value string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#FFE66D")).Render(value)
}

func selected(value string, width int) string {
	return lipgloss.NewStyle().Width(width).Background(lipgloss.Color("#2A2A4A")).Foreground(lipgloss.Color("#EEEEFF")).Bold(true).Render(truncate(value, width))
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	return charmansi.Truncate(value, width, "…")
}

func window(lines []string, offset, count int) []string {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(lines) {
		offset = max(0, len(lines)-count)
	}
	end := min(len(lines), offset+count)
	return lines[offset:end]
}

func duration(seconds float64) string {
	if seconds <= 0 {
		return "--:--"
	}
	return fmt.Sprintf("%02d:%02d", int(seconds)/60, int(seconds)%60)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	m := newModel(os.Args[1:])
	program := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "diss:", err)
		os.Exit(1)
	}
}
