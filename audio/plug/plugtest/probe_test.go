package plugtest

import (
	"testing"

	"github.com/marrasen/gunim/audio/plug"
)

func TestAPedalHoldsOnlyTheNotesOfItsPort(t *testing.T) {
	pr := Probe.New(nil).Processor()
	s := plug.Setup{Rate: 48000, Channels: 2, MaxBlock: 64}
	// One lane, group 3, wired into both inputs: the pedal goes down on the side input alone.
	res := Drive(t, &Probe, pr, s, 256, Ports{Events: []plug.Event{
		{Frame: 10, Kind: plug.NoteOn, Note: 1, Group: 3, Pitch: 60, Velocity: 1},
		{Frame: 10, Kind: plug.NoteOn, Note: 2, Group: 3, Port: 1, Pitch: 60, Velocity: 1},
		{Frame: 20, Kind: plug.Pedal, Group: 3, Port: 1, Value: 1},
		{Frame: 30, Kind: plug.NoteOff, Note: 1, Group: 3},
		{Frame: 30, Kind: plug.NoteOff, Note: 2, Group: 3, Port: 1},
		{Frame: 100, Kind: plug.Pedal, Group: 3, Port: 1, Value: 0},
	}})
	for _, c := range []struct {
		name        string
		ch, f, want int
	}{
		{"the main note before it lets go", 0, 29, 1},
		{"the main note as it lets go", 0, 30, 0},
		{"the side note before it lets go", 1, 29, 1},
		{"the side note under the pedal", 1, 99, 1},
		{"the side note as the pedal lifts", 1, 100, 0},
	} {
		if got := res.Out[c.ch][c.f]; got != float32(c.want) {
			t.Errorf("%s rings %v notes, want %d", c.name, got, c.want)
		}
	}
	heard := pr.(*ProbeSynth).Heard()
	if len(heard) != 6 || heard[1].Port != 1 || heard[5].Frame != 100 {
		t.Errorf("the probe heard %+v, want the six events, the second on port 1, the last at frame 100", heard)
	}
}
