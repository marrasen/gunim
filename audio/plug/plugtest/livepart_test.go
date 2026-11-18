package plugtest

import (
	"testing"

	"github.com/marrasen/gunim/audio/plug"
)

// twoParts is an instrument with a live part: it sounds the number of
// song notes down on Out and of live notes down on LiveOut. Reset ends
// the song's notes alone.
type twoParts struct{ song, live int }

func (p *twoParts) Prepare(plug.Setup) {}
func (p *twoParts) Latency() int       { return 0 }
func (p *twoParts) Tail() int          { return 0 }
func (p *twoParts) Reset()             { p.song = 0 }

func (p *twoParts) Process(b *plug.Block) {
	next := 0
	for f := range b.Frames {
		for ; next < len(b.Events) && b.Events[next].Frame <= f; next++ {
			e := b.Events[next]
			n := &p.song
			if e.Live {
				n = &p.live
			}
			switch e.Kind {
			case plug.NoteOn:
				*n++
			case plug.NoteOff:
				*n--
			}
		}
		for _, ch := range b.Out {
			ch[f] = float32(p.song)
		}
		for _, ch := range b.LiveOut {
			ch[f] = float32(p.live)
		}
	}
}

func TestDriveConnectsTheLivePartAndResets(t *testing.T) {
	d := plug.Descriptor{ID: "test.two", Notes: true, LivePart: true}
	s := plug.Setup{Rate: 48000, MaxBlock: 64, Channels: 2}
	res := Drive(t, &d, &twoParts{}, s, 256, Ports{ResetAt: []int{128}, Events: []plug.Event{
		{Frame: 10, Kind: plug.NoteOn, Note: 1, Pitch: 60, Velocity: 1},
		{Frame: 20, Kind: plug.NoteOn, Note: 1, Pitch: 64, Velocity: 1, Live: true},
	}})
	if res.LiveOut == nil {
		t.Fatal("Drive left the live part's output unconnected")
	}
	for _, c := range []struct {
		frame      int
		song, live float32
	}{{30, 1, 1}, {130, 0, 1}} {
		if got := res.Out[0][c.frame]; got != c.song {
			t.Errorf("the main output at frame %d is %v, want %v", c.frame, got, c.song)
		}
		if got := res.LiveOut[1][c.frame]; got != c.live {
			t.Errorf("the live output at frame %d is %v, want %v", c.frame, got, c.live)
		}
	}
	// Without a live part, the output stays unconnected.
	d.LivePart = false
	if res := Drive(t, &d, &twoParts{}, s, 64, Ports{}); res.LiveOut != nil {
		t.Error("Drive connected a live output for a plugin without a live part")
	}
}
