package observers

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// PurgeArtifacts removes files in dir older than maxAge. Returns deleted count.
func PurgeArtifacts(dir string, maxAge time.Duration) (int, error) {
	if dir == "" || maxAge <= 0 {
		return 0, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	var removed int
	var errs error
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if err := os.Remove(path); err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		removed++
	}
	return removed, errs
}
