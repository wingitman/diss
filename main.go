package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	charmansi "github.com/charmbracelet/x/ansi"
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
	driveRows, trackRows               []rect
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
type copyMsg struct{ err error }
type configMsg struct {
	cfg config.Config
	err error
}
type updateMsg struct {
	text  string
	state update.State
}

type model struct {
	config      config.Config
	service     disc.Service
	drives      []disc.Drive
	media       disc.Media
	tracks      []project.Track
	dataPaths   []string
	projectMode string
	selected    int
	focus       int
	detailLines []string
	scroll      int
	width       int
	height      int
	status      string
	err         error
	busy        bool
	confirm     bool
	confirmMode string
	updateState update.State
	help        bool
	quitting    bool
}

func newModel(args []string) model {
	cfg, cfgErr := config.Load()
	m := model{config: cfg, service: disc.NewService(disc.CommandRunner{}), projectMode: "audio", status: "Scanning optical drives…", err: cfgErr}
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
	return tea.Batch(m.refresh(), m.probeTracks())
}

func (m model) refresh() tea.Cmd {
	return func() tea.Msg {
		drives, err := m.service.Drives(context.Background())
		return drivesMsg{drives: drives, err: err}
	}
}

func (m model) inspect() tea.Cmd {
	if len(m.drives) == 0 {
		return nil
	}
	path := m.drives[min(m.selected, len(m.drives)-1)].Path
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

func (m model) burn() tea.Cmd {
	if len(m.drives) == 0 {
		return nil
	}
	device := m.drives[m.selected].Path
	tracks := append([]project.Track(nil), m.tracks...)
	paths := append([]string(nil), m.dataPaths...)
	appendSession := m.projectMode == "data" && m.media.Present && m.media.State != "blank" && !m.media.Finalized
	return func() tea.Msg {
		var err error
		if m.projectMode == "audio" {
			err = project.ConvertAndBurn(context.Background(), device, tracks)
		} else {
			err = project.BurnData(context.Background(), device, paths, appendSession)
		}
		return burnMsg{err: err}
	}
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
		m.detailLines = append([]string(nil), msg.media.Details...)
		m.scroll = 0
		if m.err != nil {
			m.status = "Media inspection failed"
		} else if !m.media.Present {
			m.status = "Insert a disc to inspect or burn"
		} else {
			m.status = fmt.Sprintf("%s · %s", m.media.Kind, m.media.State)
		}
	case tracksMsg:
		m.tracks, m.err = msg.tracks, msg.err
		if m.err != nil {
			m.status = "Audio metadata failed"
		} else if len(m.tracks) > 0 {
			m.status = fmt.Sprintf("Loaded %d audio track(s)", len(m.tracks))
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
	for index, row := range l.driveRows {
		if row.contains(msg.X, msg.Y) {
			m.selected, m.focus = index, 0
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
		case k.Confirm:
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
	switch key {
	case k.Quit, "ctrl+c":
		m.quitting = true
		return tea.Quit
	case k.Help:
		m.help = true
	case k.OpenConfig:
		m.status = "Opening configuration…"
		return m.openConfig()
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
		if m.canBurn() {
			m.confirm, m.confirmMode = true, "burn"
		} else {
			m.status = "Select a project and insert writable media first"
		}
	case k.Confirm:
		if m.focus == 0 && len(m.drives) > 0 {
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
	case k.Last:
		if count := m.itemCount(); count > 0 {
			m.selected = count - 1
		}
	case k.Left, "tab":
		m.focus = max(0, m.focus-1)
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
	return box("DRIVES", lines, l.left)
}

func (m model) renderProjectPanel(l layout) string {
	inner := l.right.width - 4
	lines := []string{accent(strings.ToUpper(m.projectMode) + " PROJECT")}
	trackRows := []rect{}
	if m.projectMode == "audio" {
		if len(m.tracks) == 0 {
			lines = append(lines, muted("Launch with audio paths:"), result("diss ~/Music"))
		} else {
			for index, track := range m.tracks {
				line := fmt.Sprintf("%s%02d  %-*s %s", map[bool]string{true: "▶ ", false: "  "}[index == m.selected && m.focus == 1], index+1, max(1, inner-26), truncate(track.Title, max(1, inner-26)), duration(track.Duration))
				if index == m.selected && m.focus == 1 {
					line = selected(line, inner)
				}
				lines = append(lines, line)
				trackRows = append(trackRows, rect{l.right.x + 2, l.right.y + 2 + len(trackRows), inner, 1})
			}
			lines = append(lines, "", muted(fmt.Sprintf("total %s / 80:00", duration(project.TotalDuration(m.tracks)))))
		}
	} else {
		if len(m.dataPaths) == 0 {
			lines = append(lines, muted("Launch with data paths:"), result("diss --data ~/Documents"))
		} else {
			for index, path := range m.dataPaths {
				line := fmt.Sprintf("%s%02d  %s", map[bool]string{true: "▶ ", false: "  "}[index == m.selected && m.focus == 1], index+1, truncate(path, inner-5))
				if index == m.selected && m.focus == 1 {
					line = selected(line, inner)
				}
				lines = append(lines, line)
				trackRows = append(trackRows, rect{l.right.x + 2, l.right.y + 2 + len(trackRows), inner, 1})
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
	return box("PROJECT", lines, l.right)
}

func (m *model) renderFooter(l layout) string {
	status := m.status
	if m.err != nil {
		status = "error: " + m.err.Error()
	}
	if m.busy {
		status = "◌ " + status
	}
	return lipgloss.NewStyle().Width(max(1, l.width-1)).Foreground(lipgloss.Color("#666688")).Render(truncate(status, max(1, l.width-1)))
}

func (m model) renderHints(l layout) string {
	if !m.config.UI.ShowHints {
		return ""
	}
	parts := []string{hint(m.config.Keybinds.Up, "move"), hint(m.config.Keybinds.PageDown, "page"), hint(m.config.Keybinds.Inspect, "inspect"), hint(m.config.Keybinds.Burn, "burn"), hint(m.config.Keybinds.Help, "help"), hint(m.config.Keybinds.Quit, "quit")}
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
	lines := []string{accent("CONFIRM " + strings.ToUpper(name) + "?"), "", warning, "", primary("enter") + " confirm    " + muted("esc cancel")}
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
		l.trackRows[index] = rect{l.right.x + 2, l.right.y + 2 + index, max(1, l.right.width-4), 1}
	}
	l.driveRows = visibleRows(l.driveRows, l.left)
	l.trackRows = visibleRows(l.trackRows, l.right)
	return l
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
