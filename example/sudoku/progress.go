package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// levelCount is how many levels the map holds.
const levelCount = 60

// progress is what the player has done, kept between runs: the stars
// won on each level, the highest level open, and the game left half
// played.
type progress struct {
	Stars    []int
	Unlocked int
	Current  *savedGame `json:",omitempty"`
}

// savedGame is a game left half played.
type savedGame struct {
	Level                         int
	Cells                         Grid
	Notes                         [81]uint16
	Lives, Score, Mistakes, Hints int
}

// progressFile is where progress is kept, in dir.
func progressFile(dir string) string { return filepath.Join(dir, "progress.json") }

// configDir returns the folder the game keeps its progress in.
func configDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = os.TempDir()
	}
	return filepath.Join(d, "gunim-candy-sudoku")
}

// loadProgress reads the progress kept in dir, or starts afresh.
func loadProgress(dir string) progress {
	p := progress{Unlocked: 1}
	b, err := os.ReadFile(progressFile(dir))
	if err == nil {
		_ = json.Unmarshal(b, &p)
	}
	p.Unlocked = max(1, min(p.Unlocked, levelCount))
	if len(p.Stars) != levelCount {
		stars := make([]int, levelCount)
		copy(stars, p.Stars)
		p.Stars = stars
	}
	return p
}

// save writes the progress to dir, whole or not at all.
func (p progress) save(dir string) error {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	tmp := progressFile(dir) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, progressFile(dir))
}

// forget removes the progress kept in dir.
func forget(dir string) error {
	err := os.Remove(progressFile(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// saved returns the game half played, as saved.
func (g *game) saved() *savedGame {
	return &savedGame{Level: g.Level, Cells: g.Cells, Notes: g.Notes, Lives: g.Lives, Score: g.Score, Mistakes: g.Mistakes, Hints: g.Hints}
}

// restore picks up a saved game where it was left, if it fits the
// level's puzzle.
func (g *game) restore(s *savedGame) bool {
	if s == nil || s.Level != g.Level || s.Lives <= 0 {
		return false
	}
	for c := range 81 {
		if g.Givens[c] && s.Cells[c] != g.Cells[c] {
			return false
		}
		if s.Cells[c] != 0 && s.Cells[c] != g.puzzle.Solution[c] {
			return false
		}
	}
	g.Cells, g.Notes = s.Cells, s.Notes
	g.Lives, g.Score, g.Mistakes, g.Hints = s.Lives, s.Score, s.Mistakes, s.Hints
	return true
}
