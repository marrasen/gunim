package main

import (
	"testing"

	"github.com/marrasen/gunim"
)

// TestWire checks that everything the two halves exchange would also
// cross a socket, so moving the application out of process stays a
// transport change.
func TestWire(t *testing.T) {
	err := gunim.CheckWire(
		JobList{Jobs: []Job{{ID: "1", Title: "Reindex", Status: JobRunning, Progress: 0.5}}},
		ConfirmState{Title: "Delete?"},
		JobProgress{ID: "1", Progress: 0.25},
		RunRequested{ID: "1"},
		DeleteRequested{ID: "1"},
		Confirmed{What: "Delete?"},
		Cancelled{},
		ThemeToggled{},
	)
	if err != nil {
		t.Fatal(err)
	}
}
