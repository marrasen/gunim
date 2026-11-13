//go:build linux && (arm64 || amd64)

package kmsbench

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A scene with no way to open its window is refused before the screen is opened.
func TestASceneWithNoOpenIsRefused(t *testing.T) {
	if _, err := Bench(context.Background(), Options{Scene: &Scene{Name: "status"}}); err == nil {
		t.Fatal("a scene with no Open ran")
	}
}

// Report counts the goals missed, and says so last.
func TestReportCountsTheGoalsMissed(t *testing.T) {
	var b strings.Builder
	busy := Result{Rate: 60, StillAfter: 50 * time.Second, RestFor: time.Second}
	missed, err := Report(&b, busy)
	if err != nil {
		t.Fatal(err)
	}
	if missed == 0 || !strings.Contains(b.String(), "MISSED") || !strings.HasSuffix(b.String(), "goals missed\n") {
		t.Fatalf("a screen still drawing 50 s on reports %d missed:\n%s", missed, b.String())
	}
}

// The default script starts with the replay's header, as the driver reads it.
func TestDragScriptHasItsHeader(t *testing.T) {
	if s := DragScript(90, time.Second); !strings.HasPrefix(string(s), "gunim-evdev 1 ") {
		t.Fatalf("the script starts %q", s[:min(len(s), 20)])
	}
}
