package disc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type Drive struct {
	Path   string
	Name   string
	Vendor string
	Model  string
}

type Media struct {
	Present   bool
	Kind      string
	State     string
	Writable  bool
	Finalized bool
	Sessions  int
	Tracks    int
	Details   []string
}

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type CommandRunner struct{}

func (CommandRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

type Service struct{ runner Runner }

func NewService(r Runner) Service { return Service{runner: r} }

type lsblkJSON struct {
	Blockdevices []struct {
		Path   string `json:"path"`
		Name   string `json:"name"`
		Type   string `json:"type"`
		Model  string `json:"model"`
		Vendor string `json:"vendor"`
	} `json:"blockdevices"`
}

func (s Service) Drives(ctx context.Context) ([]Drive, error) {
	data, err := s.runner.Run(ctx, "lsblk", "-J", "-o", "NAME,PATH,TYPE,MODEL,VENDOR")
	if err != nil {
		return nil, fmt.Errorf("discover drives: %w: %s", err, strings.TrimSpace(string(data)))
	}
	var result lsblkJSON
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse drive list: %w", err)
	}
	var drives []Drive
	for _, item := range result.Blockdevices {
		if item.Type != "rom" {
			continue
		}
		drives = append(drives, Drive{Path: item.Path, Name: item.Name, Vendor: strings.TrimSpace(item.Vendor), Model: strings.TrimSpace(item.Model)})
	}
	return drives, nil
}

func (s Service) Inspect(ctx context.Context, path string) (Media, error) {
	data, err := s.runner.Run(ctx, "udevadm", "info", "--query=property", "--name="+path)
	if err != nil {
		return Media{}, fmt.Errorf("inspect media: %w: %s", err, strings.TrimSpace(string(data)))
	}
	props := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok {
			props[key] = value
		}
	}
	media := Media{Present: props["ID_CDROM_MEDIA"] == "1", State: props["ID_CDROM_MEDIA_STATE"]}
	media.Writable = props["ID_CDROM_MEDIA_CD_R"] == "1" || props["ID_CDROM_MEDIA_DVD_R"] == "1" || props["ID_CDROM_MEDIA_BD_R"] == "1" || props["ID_CDROM_MEDIA_CD_RW"] == "1"
	switch {
	case props["ID_CDROM_MEDIA_CD_R"] == "1":
		media.Kind = "CD-R"
	case props["ID_CDROM_MEDIA_CD_RW"] == "1":
		media.Kind = "CD-RW"
	case props["ID_CDROM_MEDIA_DVD_R"] == "1":
		media.Kind = "DVD-R"
	case props["ID_CDROM_MEDIA_BD_R"] == "1":
		media.Kind = "BD-R"
	}
	if !media.Present {
		return media, nil
	}
	info, infoErr := s.runner.Run(ctx, "cdrecord", "-minfo", "dev="+path)
	if infoErr != nil {
		return media, nil
	}
	media.Details = strings.Split(strings.TrimSpace(string(info)), "\n")
	media.Sessions = numberAfter(string(info), "Number of Sessions:")
	media.Tracks = numberAfter(string(info), "Number of Tracks:")
	media.Finalized = strings.Contains(string(info), "Disk status: complete") || strings.Contains(string(info), "session status: complete")
	return media, nil
}

func numberAfter(text, prefix string) int {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
			return n
		}
	}
	return 0
}
