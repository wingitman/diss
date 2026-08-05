package browser

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Entry struct {
	Path      string
	Name      string
	Directory bool
}

func Scan(ctx context.Context, directory, mode string) ([]Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		if entry.IsDir() {
			result = append(result, Entry{Path: path, Name: entry.Name(), Directory: true})
			continue
		}
		if mode == "data" || audioFile(entry.Name()) {
			result = append(result, Entry{Path: path, Name: entry.Name()})
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Directory != result[j].Directory {
			return result[i].Directory
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result, nil
}

func audioFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp3", ".wav", ".flac":
		return true
	default:
		return false
	}
}
