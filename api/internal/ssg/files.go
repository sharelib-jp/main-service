// Package ssg builds and publishes complete, immutable catalog generations.
package ssg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

var ErrBusy = errors.New("catalog generation/publishing is already running")
var generationPattern = regexp.MustCompile(`^g-[0-9a-f]{32}$`)

func generationDir(output, id string) (string, error) {
	if !generationPattern.MatchString(id) {
		return "", fmt.Errorf("invalid generation ID")
	}
	return filepath.Join(output, "releases", id), nil
}
func lock(output string) (*os.File, error) {
	if err := os.MkdirAll(output, 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(output, ".generation.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrBusy
		}
		return nil, err
	}
	return f, nil
}
func unlock(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".write-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
func readID(output, name string) string {
	data, _ := os.ReadFile(filepath.Join(output, "control", name))
	id := strings.TrimSpace(string(data))
	if !generationPattern.MatchString(id) {
		return ""
	}
	return id
}
func Current(output string) string { return readID(output, "published") }
func PublicDir(output, id string) string {
	dir, err := generationDir(output, id)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "public")
}
func publishID(output, id string) error {
	previous := Current(output)
	if previous != "" && previous != id {
		if err := atomicWrite(filepath.Join(output, "control", "previous"), []byte(previous+"\n"), 0644); err != nil {
			return err
		}
	}
	return atomicWrite(filepath.Join(output, "control", "published"), []byte(id+"\n"), 0644)
}

// Called only under the shared generation/publisher lock, after publication.
func prune(output string) {
	current, previous := Current(output), readID(output, "previous")
	if current == "" {
		return
	}
	pending := readID(output, "requested")
	entries, _ := os.ReadDir(filepath.Join(output, "releases"))
	for _, entry := range entries {
		if generationPattern.MatchString(entry.Name()) && entry.Name() != current && entry.Name() != previous && entry.Name() != pending {
			_ = os.RemoveAll(filepath.Join(output, "releases", entry.Name()))
		}
	}
}

// Initialize establishes shared directory ownership before the gateway starts.
func Initialize(output string) error {
	guard, err := lock(output)
	if err != nil {
		return err
	}
	defer unlock(guard)
	for _, name := range []string{"control", "releases", "assets"} {
		if err := os.MkdirAll(filepath.Join(output, name), 0755); err != nil {
			return err
		}
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
