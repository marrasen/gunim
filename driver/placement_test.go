package driver

import (
	"testing"

	"github.com/marrasen/gunim/geom"
)

// Two monitors side by side: a primary one at 100% with a task bar
// along its bottom, and one at 200% to its right.
var twoMonitors = []Monitor{
	{
		Name: "left", Primary: true, Scale: 1, CoordsPerLogical: 1,
		Bounds: geom.Rc(0, 0, 1920, 1080), WorkArea: geom.Rc(0, 0, 1920, 1040),
	},
	{
		Name: "right", Scale: 2, CoordsPerLogical: 2,
		Bounds: geom.Rc(1920, 0, 3840, 2160), WorkArea: geom.Rc(1920, 0, 3840, 2160),
	},
}

func TestFitPlacement(t *testing.T) {
	tests := []struct {
		name     string
		in       Placement
		monitors []Monitor
		frame    geom.Insets
		want     Placement
		ok       bool
	}{
		{
			name: "kept where it was",
			in:   Placement{Bounds: geom.Rc(100, 100, 800, 600)},
			want: Placement{Bounds: geom.Rc(100, 100, 800, 600)}, ok: true,
		},
		{
			name: "kept on the second monitor, maximized",
			in:   Placement{Bounds: geom.Rc(2500, 300, 1600, 1200), Maximized: true},
			want: Placement{Bounds: geom.Rc(2500, 300, 1600, 1200), Maximized: true}, ok: true,
		},
		{
			name:     "kept with most of it off the right edge, the title bar still in reach",
			in:       Placement{Bounds: geom.Rc(1850, 100, 800, 600)},
			monitors: twoMonitors[:1],
			want:     Placement{Bounds: geom.Rc(1850, 100, 800, 600)}, ok: true,
		},
		{
			name:     "centred on the primary once its monitor has gone",
			in:       Placement{Bounds: geom.Rc(2500, 300, 800, 600), Maximized: true},
			monitors: twoMonitors[:1],
			want:     Placement{Bounds: geom.Rc(560, 220, 800, 600), Maximized: true}, ok: true,
		},
		{
			name: "centred when the title bar is under the task bar",
			in:   Placement{Bounds: geom.Rc(100, 1045, 800, 600)},
			want: Placement{Bounds: geom.Rc(560, 220, 800, 600)}, ok: true,
		},
		{
			name: "centred when the title bar is above the top of every screen",
			in:   Placement{Bounds: geom.Rc(100, -300, 800, 600)},
			want: Placement{Bounds: geom.Rc(560, 220, 800, 600)}, ok: true,
		},
		{
			name:     "centred when only a sliver of the title bar shows",
			in:       Placement{Bounds: geom.Rc(1900, 100, 800, 600)},
			monitors: twoMonitors[:1],
			want:     Placement{Bounds: geom.Rc(560, 220, 800, 600)}, ok: true,
		},
		{
			// 40 logical pixels are 80 device pixels at 200%, and only 60 show; on the first monitor 60 would do
			name:     "the grip is in logical pixels",
			in:       Placement{Bounds: geom.Rc(1000, 100, 980, 600)},
			monitors: twoMonitors[1:],
			want:     Placement{Bounds: geom.Rc(3350, 780, 980, 600)}, ok: true,
		},
		{
			name:     "shrunk to the work area it is on",
			in:       Placement{Bounds: geom.Rc(50, 20, 2500, 1500)},
			monitors: twoMonitors[:1],
			want:     Placement{Bounds: geom.Rc(0, 0, 1920, 1040)}, ok: true,
		},
		{
			name:     "shrunk only along the side that is too long",
			in:       Placement{Bounds: geom.Rc(50, 20, 800, 1500)},
			monitors: twoMonitors[:1],
			want:     Placement{Bounds: geom.Rc(50, 0, 800, 1040)}, ok: true,
		},
		{
			name: "shrunk and centred on the primary",
			in:   Placement{Bounds: geom.Rc(-5000, 0, 3000, 900)},
			want: Placement{Bounds: geom.Rc(0, 70, 1920, 900)}, ok: true,
		},
		{
			name:  "the system's title bar counts",
			in:    Placement{Bounds: geom.Rc(100, 5, 800, 600)},
			frame: geom.Insets{Top: 30, Left: 1, Right: 1, Bottom: 1},
			want:  Placement{Bounds: geom.Rc(560, 234, 800, 600)}, ok: true,
		},
		{
			name:     "the frame fits in the work area too",
			in:       Placement{Bounds: geom.Rc(10, 40, 1920, 1040)},
			monitors: twoMonitors[:1],
			frame:    geom.Insets{Top: 30, Left: 1, Right: 1, Bottom: 1},
			want:     Placement{Bounds: geom.Rc(1, 30, 1918, 1009)}, ok: true,
		},
		{
			name:     "a monitor with no work area uses its bounds",
			in:       Placement{Bounds: geom.Rc(100, 1045, 800, 600)},
			monitors: []Monitor{{Bounds: geom.Rc(0, 0, 1920, 1080)}},
			want:     Placement{Bounds: geom.Rc(100, 1045, 800, 600)}, ok: true,
		},
		{
			name: "no primary centres on the first",
			in:   Placement{Bounds: geom.Rc(-3000, 0, 800, 600)},
			monitors: []Monitor{
				{Bounds: geom.Rc(0, 0, 1000, 1000)},
				{Bounds: geom.Rc(1000, 0, 1000, 1000)},
			},
			want: Placement{Bounds: geom.Rc(100, 200, 800, 600)}, ok: true,
		},
		{
			name: "empty bounds",
			in:   Placement{Maximized: true},
			want: Placement{Maximized: true},
		},
		{
			name:     "no monitors",
			in:       Placement{Bounds: geom.Rc(1, 2, 3, 4)},
			monitors: []Monitor{},
			want:     Placement{Bounds: geom.Rc(1, 2, 3, 4)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := tt.monitors
			if ms == nil {
				ms = twoMonitors
			}
			got, ok := FitPlacement(tt.in, ms, tt.frame)
			if got != tt.want || ok != tt.ok {
				t.Errorf("FitPlacement(%v) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}
