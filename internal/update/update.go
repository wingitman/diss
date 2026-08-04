package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type State struct {
	Repo    string
	Current string
	Remote  string
	Dirty   bool
	History []string
}

func Check(ctx context.Context, repo string) (State, error) {
	state := State{Repo: repo}
	if repo == "" {
		return state, fmt.Errorf("set updates.repo_path to the diss source checkout")
	}
	if output, err := run(ctx, repo, "fetch", "--prune", "--all"); err != nil {
		return state, fmt.Errorf("fetch updates: %w: %s", err, strings.TrimSpace(output))
	}
	current, err := run(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		return state, err
	}
	state.Current = strings.TrimSpace(current)
	if dirty, err := run(ctx, repo, "status", "--porcelain"); err == nil {
		state.Dirty = strings.TrimSpace(dirty) != ""
	}
	if remote, err := run(ctx, repo, "rev-parse", "@{upstream}"); err == nil {
		state.Remote = strings.TrimSpace(remote)
	}
	if history, err := run(ctx, repo, "log", "-5", "--oneline"); err == nil {
		state.History = strings.Split(strings.TrimSpace(history), "\n")
	}
	return state, nil
}

func run(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func LaunchDetached(repo, target, terminal string, latest bool) error {
	if repo == "" {
		return errors.New("missing update repository path")
	}
	dir := filepath.Join(os.TempDir(), "diss-updates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	script := filepath.Join(dir, fmt.Sprintf("update-%d.sh", time.Now().UnixNano()))
	mode := "git checkout --detach " + shellQuote(target)
	content := "#!/bin/sh\nset -eu\ncd " + shellQuote(repo) + "\nprev=$(git rev-parse --abbrev-ref HEAD)\ngit fetch --prune --all\n" + mode + "\nmake install\nif [ \"$prev\" != HEAD ]; then git checkout \"$prev\"; fi\nprintf '\\nDiss update complete. Press Enter to close...\\n'\nread _\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		return err
	}
	term, args, err := terminalCommand(terminal, script)
	if err != nil {
		return err
	}
	return exec.Command(term, args...).Start()
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func terminalCommand(preferred, script string) (string, []string, error) {
	if preferred != "" {
		return preferred, []string{"-e", script}, nil
	}
	for _, candidate := range []struct {
		name string
		args []string
	}{{"x-terminal-emulator", []string{"-e", script}}, {"gnome-terminal", []string{"--", script}}, {"konsole", []string{"-e", script}}, {"alacritty", []string{"-e", script}}, {"kitty", []string{script}}} {
		if _, err := exec.LookPath(candidate.name); err == nil {
			return candidate.name, candidate.args, nil
		}
	}
	return "", nil, errors.New("no supported terminal found for detached update")
}
