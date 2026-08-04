package project

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Track struct {
	Path     string
	Title    string
	Duration float64
}

func Audio(paths []string) ([]Track, error) {
	var files []string
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			err = filepath.Walk(path, func(name string, info os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !info.IsDir() && strings.EqualFold(filepath.Ext(name), ".mp3") {
					files = append(files, name)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		} else if strings.EqualFold(filepath.Ext(path), ".mp3") || strings.EqualFold(filepath.Ext(path), ".wav") || strings.EqualFold(filepath.Ext(path), ".flac") {
			files = append(files, path)
		}
	}
	sort.Strings(files)
	tracks := make([]Track, 0, len(files))
	for _, path := range files {
		tracks = append(tracks, Track{Path: path, Title: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))})
	}
	return tracks, nil
}

func Probe(ctx context.Context, tracks []Track) ([]Track, error) {
	for index := range tracks {
		data, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", tracks[index].Path).CombinedOutput()
		if err != nil {
			return tracks, fmt.Errorf("probe %s: %w: %s", tracks[index].Title, err, strings.TrimSpace(string(data)))
		}
		if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%f", &tracks[index].Duration); err != nil {
			return tracks, fmt.Errorf("parse duration for %s: %w", tracks[index].Title, err)
		}
	}
	return tracks, nil
}

func TotalDuration(tracks []Track) float64 {
	var total float64
	for _, track := range tracks {
		total += track.Duration
	}
	return total
}

func ConvertAndBurn(ctx context.Context, device string, tracks []Track) error {
	if len(tracks) == 0 {
		return fmt.Errorf("no audio tracks selected")
	}
	if TotalDuration(tracks) > 80*60 {
		return fmt.Errorf("audio project exceeds an 80-minute CD")
	}
	temp, err := os.MkdirTemp("", "diss-audio-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	wavs := make([]string, 0, len(tracks))
	for index, track := range tracks {
		wav := filepath.Join(temp, fmt.Sprintf("track-%02d.wav", index+1))
		output, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", track.Path, "-ar", "44100", "-ac", "2", "-sample_fmt", "s16", wav).CombinedOutput()
		if err != nil {
			return fmt.Errorf("convert %s: %w: %s", track.Title, err, strings.TrimSpace(string(output)))
		}
		wavs = append(wavs, wav)
	}
	args := []string{"-v", "dev=" + device, "speed=8", "-dao", "-audio", "-pad", "-eject"}
	args = append(args, wavs...)
	output, err := exec.CommandContext(ctx, "cdrecord", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("burn failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func BurnData(ctx context.Context, device string, paths []string, appendSession bool) error {
	if len(paths) == 0 {
		return fmt.Errorf("no data items selected")
	}
	args := []string{"-dvd-compat"}
	if appendSession {
		args = append(args, "-M", device)
	} else {
		args = append(args, "-Z", device)
	}
	args = append(args, "-R", "-J")
	args = append(args, paths...)
	output, err := exec.CommandContext(ctx, "growisofs", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("data burn failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
