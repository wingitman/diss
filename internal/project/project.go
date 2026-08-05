package project

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type Track struct {
	Path     string
	Title    string
	Duration float64
}

type BurnEvent struct {
	Phase    string
	Line     string
	Progress int
	Done     bool
	Err      error
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

func ConvertAndBurn(ctx context.Context, device string, tracks []Track, appendSession bool) error {
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
	args := []string{"-v", "dev=" + device, "speed=8"}
	if appendSession {
		args = append(args, "-multi", "-audio", "-pad", "-eject")
	} else {
		args = append(args, "-dao", "-audio", "-pad", "-eject")
	}
	args = append(args, wavs...)
	output, err := exec.CommandContext(ctx, "cdrecord", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("burn failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func RunBurn(ctx context.Context, device string, tracks []Track, paths []string, appendSession bool, emit func(BurnEvent)) error {
	if len(tracks) == 0 && len(paths) == 0 {
		return fmt.Errorf("no project items selected")
	}
	var err error
	if len(tracks) > 0 {
		err = runAudioBurn(ctx, device, tracks, appendSession, emit)
	} else {
		err = runDataBurn(ctx, device, paths, appendSession, emit)
	}
	if err != nil {
		return err
	}
	emit(BurnEvent{Phase: "Verifying disc", Progress: 98})
	output, verifyErr := exec.CommandContext(ctx, "cdrecord", "-minfo", "dev="+device).CombinedOutput()
	if verifyErr != nil {
		return fmt.Errorf("verification failed: %w: %s", verifyErr, strings.TrimSpace(string(output)))
	}
	emit(BurnEvent{Phase: "Disc verified", Progress: 100})
	return nil
}

func runAudioBurn(ctx context.Context, device string, tracks []Track, appendSession bool, emit func(BurnEvent)) error {
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
		emit(BurnEvent{Phase: fmt.Sprintf("Converting track %d/%d", index+1, len(tracks)), Progress: index * 30 / max(1, len(tracks))})
		wav := filepath.Join(temp, fmt.Sprintf("track-%02d.wav", index+1))
		output, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", track.Path, "-ar", "44100", "-ac", "2", "-sample_fmt", "s16", wav).CombinedOutput()
		if err != nil {
			return fmt.Errorf("convert %s: %w: %s", track.Title, err, strings.TrimSpace(string(output)))
		}
		wavs = append(wavs, wav)
	}
	args := []string{"-v", "dev=" + device, "speed=8"}
	if appendSession {
		args = append(args, "-multi", "-audio", "-pad", "-eject")
	} else {
		args = append(args, "-dao", "-audio", "-pad", "-eject")
	}
	args = append(args, wavs...)
	return runBurnCommand(ctx, "Writing audio tracks", 30, 95, "cdrecord", args, emit)
}

func runDataBurn(ctx context.Context, device string, paths []string, appendSession bool, emit func(BurnEvent)) error {
	args := []string{"-dvd-compat"}
	if appendSession {
		args = append(args, "-M", device)
	} else {
		args = append(args, "-Z", device)
	}
	args = append(args, "-R", "-J")
	args = append(args, paths...)
	return runBurnCommand(ctx, "Writing data session", 0, 95, "growisofs", args, emit)
}

var writeProgress = regexp.MustCompile(`(?i)(?:Track\s+\d+:\s+)?\s*(\d+)\s+of\s+(\d+)\s+MB`)

func runBurnCommand(ctx context.Context, phase string, start, end int, name string, args []string, emit func(BurnEvent)) error {
	cmd := exec.CommandContext(ctx, name, args...)
	writer := &burnWriter{emit: emit, phase: phase, start: start, end: end}
	cmd.Stdout = writer
	cmd.Stderr = writer
	emit(BurnEvent{Phase: phase, Progress: start})
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("burn cancelled: %w", ctx.Err())
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	emit(BurnEvent{Phase: phase + " complete", Progress: end})
	return nil
}

type burnWriter struct {
	mu     sync.Mutex
	emit   func(BurnEvent)
	phase  string
	start  int
	end    int
	buffer string
}

func (w *burnWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buffer += string(data)
	for {
		index := strings.IndexAny(w.buffer, "\r\n")
		if index < 0 {
			break
		}
		line := strings.TrimSpace(w.buffer[:index])
		w.buffer = strings.TrimLeft(w.buffer[index+1:], "\r\n")
		if line == "" {
			continue
		}
		progress := w.start
		if match := writeProgress.FindStringSubmatch(line); len(match) == 3 {
			current, _ := strconv.Atoi(match[1])
			total, _ := strconv.Atoi(match[2])
			if total > 0 {
				progress = w.start + (w.end-w.start)*current/total
			}
		}
		w.emit(BurnEvent{Phase: w.phase, Line: line, Progress: progress})
	}
	return len(data), nil
}

var _ io.Writer = (*burnWriter)(nil)

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
