package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

type Keybinds struct {
	Up         string `toml:"up"`
	Down       string `toml:"down"`
	Left       string `toml:"left"`
	Right      string `toml:"right"`
	PageUp     string `toml:"page_up"`
	PageDown   string `toml:"page_down"`
	First      string `toml:"first"`
	Last       string `toml:"last"`
	Confirm    string `toml:"confirm"`
	Cancel     string `toml:"cancel"`
	Quit       string `toml:"quit"`
	Help       string `toml:"help"`
	OpenConfig string `toml:"open_config"`
	Refresh    string `toml:"refresh"`
	Inspect    string `toml:"inspect"`
	Burn       string `toml:"burn"`
	Audio      string `toml:"audio_project"`
	Data       string `toml:"data_project"`
	Copy       string `toml:"copy"`
	Updates    string `toml:"updates"`
	Install    string `toml:"install_update"`
	Rollback   string `toml:"rollback"`
}

type UI struct {
	ShowHints bool `toml:"show_hints"`
	ShowLogo  bool `toml:"show_logo"`
}

type Updates struct {
	DisableChecks bool   `toml:"disable_checks"`
	RepoPath      string `toml:"repo_path"`
	Terminal      string `toml:"terminal"`
	CurrentCommit string `toml:"current_commit"`
}

type Config struct {
	Keybinds Keybinds `toml:"keybinds"`
	UI       UI       `toml:"ui"`
	Updates  Updates  `toml:"updates"`
}

func Defaults() Config {
	return Config{
		Keybinds: Keybinds{Up: "k", Down: "j", Left: "h", Right: "l", PageUp: "pgup", PageDown: "pgdown", First: "g", Last: "G", Confirm: "enter", Cancel: "esc", Quit: "q", Help: "?", OpenConfig: "o", Refresh: "r", Inspect: "i", Burn: "b", Audio: "a", Data: "d", Copy: "y", Updates: "U", Install: "I", Rollback: "R"},
		UI:       UI{ShowHints: true, ShowLogo: true},
	}
}

func Path() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "delbysoft", "diss.toml")
}

func Load() (Config, error) {
	cfg := Defaults()
	path := Path()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return cfg, err
		}
		return cfg, write(path, cfg)
	}
	if err != nil {
		return cfg, err
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}
	if err := write(path, cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func write(path string, cfg Config) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return toml.NewEncoder(file).Encode(cfg)
}

func ResolveEditor() (string, []string, error) {
	for _, value := range []string{os.Getenv("VISUAL"), os.Getenv("EDITOR")} {
		if value = strings.TrimSpace(value); value != "" {
			parts := strings.Fields(value)
			return parts[0], parts[1:], nil
		}
	}
	candidates := []string{"nvim", "vim", "nano", "vi"}
	if runtime.GOOS == "windows" {
		candidates = []string{"notepad"}
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil, nil
		}
	}
	return "", nil, fmt.Errorf("no editor found; set VISUAL or EDITOR")
}
