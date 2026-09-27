package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// prefs are the settings the app keeps between runs.
type prefs struct {
	Favourites  []string
	ShowHidden  bool
	HidePreview bool
	Light       bool
	Zoom        float32
	Sidebar     float32
	Sort        SortBy
	Desc        bool
	// FavNames holds the names given to favourites, by path.
	FavNames map[string]string
}

// defaultPrefsPath is where the settings live: gunim-files/prefs.json in
// the user's configuration folder.
func defaultPrefsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding where to keep settings: %w", err)
	}
	return filepath.Join(dir, "gunim-files", "prefs.json"), nil
}

// loadPrefs reads the settings at path. A missing file gives the
// defaults.
func loadPrefs(path string) (prefs, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return prefs{}, nil
	}
	if err != nil {
		return prefs{}, fmt.Errorf("reading the settings in %s: %w", path, err)
	}
	var p prefs
	if err := json.Unmarshal(b, &p); err != nil {
		return prefs{}, fmt.Errorf("reading the settings in %s: %w", path, err)
	}
	return p, nil
}

// savePrefs writes the settings to path through a file beside it, so a
// failed write leaves the old settings whole.
func savePrefs(path string, p prefs) error {
	b, jerr := json.MarshalIndent(p, "", "  ")
	if jerr != nil {
		return fmt.Errorf("saving the settings: %w", jerr)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("saving the settings: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "prefs-*.json")
	if err != nil {
		return fmt.Errorf("saving the settings: %w", err)
	}
	_, werr := tmp.Write(b)
	if err := errors.Join(werr, tmp.Close()); err != nil {
		return errors.Join(fmt.Errorf("saving the settings in %s: %w", path, err), os.Remove(tmp.Name()))
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return errors.Join(fmt.Errorf("saving the settings in %s: %w", path, err), os.Remove(tmp.Name()))
	}
	return nil
}
