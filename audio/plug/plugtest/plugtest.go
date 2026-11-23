// Package plugtest helps test plugins: it counts what a processor
// allocates, plays an instrument notes and tells it of them ahead,
// drives a plugin's ports, to check what comes out, and reads the hits
// on a trigger. For a plugin's views, it checks that a view fits a size
// ([Fits]), and draws one at many sizes for a person to choose its sizes
// by ([Sheet]).
//
// A plugin's test checks its window's view at its least size, at the
// size it opens at, and at a large size, as
// [plug.Descriptor.ViewMinSize] and [plug.Descriptor.ViewSize] give them:
//
//	for _, s := range []geom.Size{d.ViewMinSize, d.ViewSize, geom.Sz(1536, 864)} {
//		plugtest.Fits(t, newView(), s, plugtest.Fit{Least: map[gunim.Node]geom.Size{meter: geom.Sz(150, 90)}})
//	}
package plugtest

import (
	"fmt"
	"math"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/marrasen/gunim/audio/plug"
)

// AllocsUnder runs run, and returns how many things were made in it
// with a function named fn among their callers: the allocations of the
// code under test alone, as other goroutines allocate as they like
// meanwhile. fn is part of a function's full name, as
// "maximizer.(*processor).Process".
func AllocsUnder(t *testing.T, fn string, run func()) int64 {
	t.Helper()
	was := runtime.MemProfileRate
	runtime.MemProfileRate = 1
	defer func() { runtime.MemProfileRate = was }()
	count := func() int64 {
		// The profile is as of the last collection, finished.
		runtime.GC()
		runtime.GC()
		var recs []runtime.MemProfileRecord
		n, _ := runtime.MemProfile(nil, true)
		for {
			recs = make([]runtime.MemProfileRecord, n+50)
			var ok bool
			if n, ok = runtime.MemProfile(recs, true); ok {
				recs = recs[:n]
				break
			}
		}
		var total int64
		for _, r := range recs {
			frames := runtime.CallersFrames(r.Stack())
			for {
				f, more := frames.Next()
				if strings.Contains(f.Function, fn) {
					total += r.AllocObjects
					break
				}
				if !more {
					break
				}
			}
		}
		return total
	}
	before := count()
	run()
	return count() - before
}

// Play prepares pr as s says and plays it as an instrument, as a host
// does, for the given number of frames in blocks of s.MaxBlock, and
// returns what came out: s.Channels channels of that many frames. The
// first block carries each settable parameter's Default as a change at
// frame 0, as [Drive]'s does. Each event's
// Frame counts from the start, and Play hands it to the block it falls
// in, in the order of their frames. Play fills Out with NaN before each
// block, so a frame the instrument leaves unwritten shows.
func Play(t *testing.T, pr plug.Processor, s plug.Setup, frames int, params []plug.Param, events ...plug.Event) [][]float32 {
	t.Helper()
	d := plug.Descriptor{Params: params}
	return Drive(t, &d, pr, s, frames, Ports{Events: events}).Out
}

// Ports is what [Drive] feeds a processor, each of it as long as the
// run, its frames counted from the start.
type Ports struct {
	// In is the main input's channels, or nil for an instrument.
	In [][]float32
	// AuxIn holds each aux input's channels, by [plug.Descriptor.AuxIn]:
	// nil, or a list too short, leaves one unconnected.
	AuxIn [][][]float32
	// ControlIn holds each control input's values, by
	// [plug.Descriptor.ControlIn]: nil, or a list too short, leaves one
	// unconnected.
	ControlIn [][]float32
	// Changes are changes to parameters, after each one's Default at
	// frame 0.
	Changes []plug.Change
	// Events are notes, for a plugin that takes them. One marked
	// [plug.Event.Live] plays the live part, for a plugin with a
	// [plug.Descriptor.LivePart].
	Events []plug.Event
	// ResetAt are frames to call [plug.Processor.Reset] at, as a host
	// does on a seek: each before the block starting there, so each is
	// a whole number of blocks.
	ResetAt []int
	// Upcoming is news of notes, for a plugin that looks ahead, as
	// [Announce] makes it.
	Upcoming []Ahead
	// Chords is news of chords, for a plugin that looks ahead to them.
	Chords []ChordAhead
	// Hits is news of hits, for a trigger input that hears of them
	// ahead, as [AnnounceHits] makes it.
	Hits []HitAhead
	// NotesAhead is how many frames ahead every notes output's wire asks to hear of notes, as
	// [plug.NotesWriter.Ahead] gives it.
	NotesAhead int
	// Tempo is each block's [plug.Position.Tempo] and EndTempo, in
	// quarter notes a minute: zero where none is known. Each block's [plug.Position.Beat]
	// counts the beats at that tempo from the start.
	Tempo float64
}

// Ahead is news of a note as [Drive] sends it: in the block holding
// frame At, with Frame, where the note lands, counted from the start
// too. Drive turns it into the block's own count.
type Ahead struct {
	At int
	plug.Upcoming
}

// ChordAhead is news of a chord as [Drive] sends it: in the block
// holding frame At, with Frame, where the chord starts, counted from the
// start too. Drive turns it into the block's own count. A Now holds
// from At, and a Removed's Frame is 0.
type ChordAhead struct {
	At int
	plug.ChordNews
}

// Chord returns news of kind k of chord id of group, named name, with
// the given tones, lowest first, in C major, for a test to send. Its
// root is the lowest tone's pitch class, and its bass that pitch class
// in the octave from C2.
func Chord(k plug.ChordKind, frame int, id, group int32, name string, tones ...float64) plug.ChordNews {
	c := plug.ChordNews{Kind: k, Frame: frame, ID: id, Group: group, Name: name, Scale: 0b1010_1011_0101}
	c.Count = copy(c.Tones[:], tones)
	if c.Count > 0 {
		low := int(tones[0])
		c.Root = ((low % 12) + 12) % 12
		c.Bass = float64(36 + c.Root)
	}
	return c
}

// HitAhead is news of a hit as [Drive] sends it: in the block holding
// frame At, with Frame, where the hit lands, counted from the start
// too. Drive turns it into the block's own count. A Removed's Frame is
// 0.
type HitAhead struct {
	At int
	plug.HitNews
}

// AnnounceHits returns the news a host sends of the hits in values, a
// trigger's as long as the run, for control input port hearing window
// frames ahead: a Coming for each, window frames before it, or at the
// start. Each hit's ID counts from 1, in the order of their frames. Its
// Beat is where it lands at tempo, in quarter notes a minute, as
// [Drive] counts [plug.Position.Beat] at that [Ports.Tempo], in bars of
// four, and its Bar the bar of 4/4 it lands in; a tempo of zero leaves
// them 0.
func AnnounceHits(s plug.Setup, window, port int, tempo float64, values []float32) []HitAhead {
	var out []HitAhead
	for f, v := range values {
		if !(v > 0) {
			continue
		}
		n := plug.HitNews{Kind: plug.HitComing, Port: port, ID: int32(len(out) + 1), Frame: f, Velocity: float64(v)}
		if tempo > 0 {
			n.Beat, n.BarBeats = float64(f)*tempo/60/s.Rate, 4
			n.Bar = FourFour(n.Beat)
		}
		out = append(out, HitAhead{At: max(f-window, 0), HitNews: n})
	}
	return out
}

// FourFour is the bar of 4/4 that beat, in quarter notes from the start, falls in, as [Drive] tells it.
func FourFour(beat float64) plug.Bar {
	n := math.Floor(beat / 4)
	return plug.Bar{Number: int(n) + 1, Start: 4 * n, Beats: 4, Unit: 4}
}

// Announce returns the news a host sends of the NoteOns in events, for
// a plugin looking window frames ahead, played in blocks of s.MaxBlock:
// a Coming for each, in the block holding the frame window before it,
// or the first block, where the note lands in a later block.
func Announce(s plug.Setup, window int, events ...plug.Event) []Ahead {
	var out []Ahead
	for _, e := range events {
		if e.Kind != plug.NoteOn {
			continue
		}
		at := max(e.Frame-window, 0)
		if at/s.MaxBlock == e.Frame/s.MaxBlock {
			continue
		}
		out = append(out, Ahead{At: at, Upcoming: plug.Upcoming{Kind: plug.Coming, Frame: e.Frame, Note: e.Note,
			Group: e.Group, Port: e.Port, Pitch: e.Pitch, Velocity: e.Velocity, Turn: e.Turn, KeyTurn: e.KeyTurn, Mod: e.Mod,
			Length: e.Length}})
	}
	return out
}

// Result is what came out of a processor [Drive] ran, each of it as long
// as the run.
type Result struct {
	// Out is the main output's channels.
	Out [][]float32
	// AuxOut holds each aux output's channels, by
	// [plug.Descriptor.AuxOut].
	AuxOut [][][]float32
	// ControlOut holds each control output's values, by
	// [plug.Descriptor.ControlOut].
	ControlOut [][]float32
	// LiveOut is the live part's channels, for a plugin with a
	// [plug.Descriptor.LivePart], or nil for one without.
	LiveOut [][]float32
	// NotesOut holds the events written on each notes output, by [plug.Descriptor.NotesOut], each with its Frame
	// counted from the start.
	NotesOut [][]plug.Event
	// NotesNews holds the news told on each notes output, by [plug.Descriptor.NotesOut], each At the start of the block
	// it was told in, and with its Frame counted from the start but a Removed's.
	NotesNews [][]Ahead
}

// Drive prepares pr, a processor of the plugin d describes, as s says,
// and runs it as a host does, for the given number of frames in blocks
// of s.MaxBlock, with the ports in p connected and every output
// connected, and returns what came out. A plugin with a
// [plug.Descriptor.LivePart] gets [plug.Block.LiveOut] connected too, and
// may be sent live events; one without is sent none. The first block carries each of
// d's settable parameters' Default as a change at frame 0, and p's changes and
// events go to the blocks they fall in, in the order of their frames, and
// its news to the blocks holding their At, in the order of At.
// Drive fills every output with NaN before each block, so a frame the
// processor leaves unwritten shows. A generator, as
// [plug.Descriptor.Generator] reports one, runs with In and Out nil,
// and its Result's Out is nil too. Every notes output is connected, with room for [plug.NotesOutRoom] events a
// block, and Drive fails t where a plugin writes an event out of its block, out of order, of a kind a notes output
// carries none of, or a NoteOn of a note still sounding. It fails t too where a plugin tells of a note landing in
// the block it tells in, or tells of one twice with Coming. As a host does, Drive ends a full writer's notes at the
// block's end.
func Drive(t *testing.T, d *plug.Descriptor, pr plug.Processor, s plug.Setup, frames int, p Ports) Result {
	t.Helper()
	generator := d.Generator()
	if frames <= 0 || s.MaxBlock <= 0 || (s.Channels <= 0 && !generator) {
		t.Fatalf("plugtest: Drive needs frames, MaxBlock and Channels above 0, has %d, %d and %d", frames, s.MaxBlock, s.Channels)
	}
	if generator && p.In != nil {
		t.Fatalf("plugtest: Drive was given a main input for %s, a generator", d.ID)
	}
	if len(p.AuxIn) > len(d.AuxIn) || len(p.ControlIn) > len(d.ControlIn) {
		t.Fatalf("plugtest: Drive was given %d aux and %d control inputs, for a plugin of %d and %d",
			len(p.AuxIn), len(p.ControlIn), len(d.AuxIn), len(d.ControlIn))
	}
	long := func(what string, chans [][]float32) {
		for c, ch := range chans {
			if ch != nil && len(ch) < frames {
				t.Fatalf("plugtest: %s channel %d has %d frames, of the %d run", what, c, len(ch), frames)
			}
		}
	}
	long("the main input's", p.In)
	for i, port := range p.AuxIn {
		long(fmt.Sprintf("aux input %d's", i), port)
	}
	long("the control inputs'", p.ControlIn)
	inputs := len(d.NotesInputs())
	port := func(what string, at int, port int32) {
		if port < 0 || port > 0 && int(port) >= inputs {
			t.Fatalf("plugtest: %s at frame %d comes on notes input %d, of %s's %d", what, at, port, d.ID, inputs)
		}
	}
	events := slices.Clone(p.Events)
	slices.SortStableFunc(events, func(a, b plug.Event) int { return a.Frame - b.Frame })
	for _, e := range events {
		if e.Frame < 0 || e.Frame >= frames {
			t.Fatalf("plugtest: an event at frame %d falls outside the %d played", e.Frame, frames)
		}
		port("an event", e.Frame, e.Port)
		if e.Live && !d.LivePart {
			t.Fatalf("plugtest: a live event at frame %d, for %s, which has no live part", e.Frame, d.ID)
		}
		if e.Live && e.Kind != plug.NoteOn && e.Kind != plug.NoteOff && e.Kind != plug.Expression && e.Kind != plug.Pedal {
			t.Fatalf("plugtest: a live event at frame %d of kind %d, which a host never marks Live", e.Frame, e.Kind)
		}
	}
	resets := map[int]bool{}
	for _, f := range p.ResetAt {
		if f <= 0 || f >= frames || f%s.MaxBlock != 0 {
			t.Fatalf("plugtest: a reset at frame %d, which starts no block after the first of the %d run", f, frames)
		}
		resets[f] = true
	}
	var changes []plug.Change
	for _, prm := range d.Params {
		if prm.Flags&plug.ReadOnly == 0 {
			changes = append(changes, plug.Change{ID: prm.ID, Value: prm.Default})
		}
	}
	more := slices.Clone(p.Changes)
	slices.SortStableFunc(more, func(a, b plug.Change) int { return a.Frame - b.Frame })
	for _, c := range more {
		if c.Frame < 0 || c.Frame >= frames {
			t.Fatalf("plugtest: a change at frame %d falls outside the %d run", c.Frame, frames)
		}
	}
	changes = append(changes, more...)
	news := slices.Clone(p.Upcoming)
	slices.SortStableFunc(news, func(a, b Ahead) int { return a.At - b.At })
	for _, u := range news {
		if u.At < 0 || u.At >= frames {
			t.Fatalf("plugtest: news sent at frame %d falls outside the %d run", u.At, frames)
		}
		port("news", u.At, u.Port)
		if end := (u.At/s.MaxBlock + 1) * s.MaxBlock; u.Kind != plug.Removed && u.Frame < end {
			t.Fatalf("plugtest: news sent at frame %d of note %d landing at %d, which is in its own block", u.At, u.Note, u.Frame)
		}
	}
	chords := slices.Clone(p.Chords)
	slices.SortStableFunc(chords, func(a, b ChordAhead) int { return a.At - b.At })
	for _, c := range chords {
		if c.At < 0 || c.At >= frames {
			t.Fatalf("plugtest: chord news sent at frame %d falls outside the %d run", c.At, frames)
		}
		port("chord news", c.At, c.Port)
		if (c.Kind == plug.ChordComing || c.Kind == plug.ChordMoved) && c.Frame < c.At {
			t.Fatalf("plugtest: chord news sent at frame %d of chord %d starting at %d, before it is sent", c.At, c.ID, c.Frame)
		}
	}
	hits := slices.Clone(p.Hits)
	slices.SortStableFunc(hits, func(a, b HitAhead) int { return a.At - b.At })
	for _, h := range hits {
		if h.At < 0 || h.At >= frames {
			t.Fatalf("plugtest: hit news sent at frame %d falls outside the %d run", h.At, frames)
		}
		if h.Port < 0 || h.Port >= len(d.ControlIn) || d.ControlIn[h.Port].Ahead <= 0 {
			t.Fatalf("plugtest: hit news sent at frame %d for control input %d, which hears of no hits ahead", h.At, h.Port)
		}
		if h.Kind != plug.HitRemoved && h.Frame < h.At {
			t.Fatalf("plugtest: hit news sent at frame %d of hit %d landing at %d, before it is sent", h.At, h.ID, h.Frame)
		}
	}

	// What comes out, and the room each block writes to.
	res := Result{Out: channels(s.Channels, frames), AuxOut: make([][][]float32, len(d.AuxOut)),
		ControlOut: channels(len(d.ControlOut), frames)}
	auxOut := make([][][]float32, len(d.AuxOut))
	for i, port := range d.AuxOut {
		n := port.Channels
		if n == 0 {
			n = s.Channels
		}
		res.AuxOut[i] = channels(n, frames)
		auxOut[i] = channels(n, s.MaxBlock)
	}
	out, controlOut := channels(s.Channels, s.MaxBlock), channels(len(d.ControlOut), s.MaxBlock)
	if generator {
		res.Out, out = nil, nil
	}
	var liveOut [][]float32
	if d.LivePart {
		res.LiveOut, liveOut = channels(s.Channels, frames), channels(s.Channels, s.MaxBlock)
	}
	notesOut := make([]*plug.NotesWriter, len(d.NotesOut))
	for i := range notesOut {
		notesOut[i] = plug.NewNotesWriter(plug.NotesOutRoom)
		notesOut[i].SetAhead(p.NotesAhead)
	}
	res.NotesOut = make([][]plug.Event, len(d.NotesOut))
	res.NotesNews = make([][]Ahead, len(d.NotesOut))
	sounding := make([]map[int32]bool, len(d.NotesOut))
	told := make([]map[int32]bool, len(d.NotesOut))
	for i := range sounding {
		sounding[i], told[i] = map[int32]bool{}, map[int32]bool{}
	}
	sent := func(b *plug.Block, at int) {
		for i, w := range b.NotesOut {
			for _, u := range w.News() {
				switch u.Kind {
				case plug.Coming, plug.Moved:
					if u.Frame < b.Frames {
						t.Fatalf("plugtest: %s told of note %d landing at frame %d, in the block of %d at %d it tells in, on notes output %d",
							d.ID, u.Note, u.Frame+at, b.Frames, at, i)
					}
					if u.Kind == plug.Coming && told[i][u.Note] {
						t.Fatalf("plugtest: %s told of note %d with Coming twice, at frame %d on notes output %d", d.ID, u.Note, at, i)
					}
					told[i][u.Note] = true
					u.Frame += at
				case plug.Removed:
					delete(told[i], u.Note)
				default:
					t.Fatalf("plugtest: %s told news of kind %d at frame %d on notes output %d", d.ID, u.Kind, at, i)
				}
				res.NotesNews[i] = append(res.NotesNews[i], Ahead{At: at, Upcoming: u})
			}
			last := 0
			for _, e := range w.Events() {
				if e.Frame < last || e.Frame >= b.Frames {
					t.Fatalf("plugtest: %s wrote an event at frame %d of a block of %d, after one at %d, on notes output %d",
						d.ID, e.Frame, b.Frames, last, i)
				}
				last = e.Frame
				if e.Live || e.Kind != plug.NoteOn && e.Kind != plug.NoteOff && e.Kind != plug.Expression && e.Kind != plug.Pedal {
					t.Fatalf("plugtest: %s wrote an event of kind %d, live %v, at frame %d on notes output %d",
						d.ID, e.Kind, e.Live, e.Frame+at, i)
				}
				switch e.Kind {
				case plug.NoteOn:
					if sounding[i][e.Note] {
						t.Fatalf("plugtest: %s started note %d at frame %d on notes output %d, while it still sounds",
							d.ID, e.Note, e.Frame+at, i)
					}
					sounding[i][e.Note] = true
					delete(told[i], e.Note)
				case plug.NoteOff:
					delete(sounding[i], e.Note)
				case plug.Expression, plug.Pedal, plug.PastStart, plug.PastNote:
				}
				e.Frame += at
				res.NotesOut[i] = append(res.NotesOut[i], e)
			}
			if w.Full() {
				// The host ends the notes of a full writer.
				clear(sounding[i])
			}
		}
	}

	// Every block is made before the first runs, so the running
	// allocates nothing of its own.
	var blocks []plug.Block
	for at := 0; at < frames; at += s.MaxBlock {
		n := min(s.MaxBlock, frames-at)
		b := plug.Block{Frames: n, Position: plug.Position{Frame: int64(at), Song: int64(at), Playing: true, Tempo: p.Tempo, EndTempo: p.Tempo,
			Beat: float64(at) * p.Tempo / 60 / s.Rate},
			In: window(p.In, at, n), Out: window(out, 0, n), ControlIn: window(p.ControlIn, at, n),
			ControlOut: window(controlOut, 0, n), LiveOut: window(liveOut, 0, n), NotesOut: notesOut}
		if p.Tempo > 0 {
			b.Position.Bar = FourFour(b.Position.Beat)
		}
		for _, port := range p.AuxIn {
			b.AuxIn = append(b.AuxIn, window(port, at, n))
		}
		for _, port := range auxOut {
			b.AuxOut = append(b.AuxOut, window(port, 0, n))
		}
		for len(events) > 0 && events[0].Frame < at+n {
			e := events[0]
			e.Frame -= at
			b.Events = append(b.Events, e)
			events = events[1:]
		}
		for len(news) > 0 && news[0].At < at+n {
			u := news[0].Upcoming
			if u.Kind == plug.Removed {
				u.Frame = 0
			} else {
				u.Frame -= at
			}
			b.Upcoming = append(b.Upcoming, u)
			news = news[1:]
		}
		for len(chords) > 0 && chords[0].At < at+n {
			c := chords[0].ChordNews
			switch c.Kind {
			case plug.ChordRemoved:
				c.Frame = 0
			case plug.ChordNow:
				c.Frame = chords[0].At - at
			default:
				c.Frame -= at
			}
			b.Chords = append(b.Chords, c)
			chords = chords[1:]
		}
		for len(hits) > 0 && hits[0].At < at+n {
			h := hits[0].HitNews
			if h.Kind == plug.HitRemoved {
				h.Frame = 0
			} else {
				h.Frame -= at
			}
			b.Hits = append(b.Hits, h)
			hits = hits[1:]
		}
		for len(changes) > 0 && changes[0].Frame < at+n {
			c := changes[0]
			c.Frame -= at
			b.Changes = append(b.Changes, c)
			changes = changes[1:]
		}
		blocks = append(blocks, b)
	}
	pr.Prepare(s)
	playBlocks(pr, blocks, resets, &res, func(b *plug.Block, at int, reset bool) {
		if reset {
			// The host ends the notes a reset plugin sent, and takes back those it told of.
			for i := range sounding {
				clear(sounding[i])
				clear(told[i])
			}
		}
		sent(b, at)
	})
	return res
}

// Pulse is a hit on a trigger, as [Pulses] reads it: the frame it is on,
// and its velocity, from 0 to 1.
type Pulse struct {
	Frame    int
	Velocity float32
}

// Pulses returns the hits on a trigger's values, as
// [plug.ControlPort.Trigger] has them: each frame above 0. It fails t
// where a value is below 0, above 1 or not a number, which no trigger
// carries.
func Pulses(t *testing.T, values []float32) []Pulse {
	t.Helper()
	var out []Pulse
	for f, v := range values {
		if !(v >= 0 && v <= 1) {
			t.Fatalf("plugtest: a trigger is %v at frame %d, outside 0 to 1", v, f)
		}
		if v > 0 {
			out = append(out, Pulse{Frame: f, Velocity: v})
		}
	}
	return out
}

// channels returns n channels of the given number of frames.
func channels(n, frames int) [][]float32 {
	out := make([][]float32, n)
	for c := range out {
		out[c] = make([]float32, frames)
	}
	return out
}

// window returns the frames from at, n of them, of each of chans, or nil
// where chans is nil, as a port unconnected.
func window(chans [][]float32, at, n int) [][]float32 {
	if chans == nil {
		return nil
	}
	out := make([][]float32, len(chans))
	for c, ch := range chans {
		if ch != nil {
			out[c] = ch[at : at+n]
		}
	}
	return out
}

// playBlocks runs pr over blocks, each with its outputs filled with NaN
// first, and copies what comes out to res. It resets pr before each block
// starting at a frame in resets, clears the block's notes outputs, and calls after, where set, once each block is
// processed.
func playBlocks(pr plug.Processor, blocks []plug.Block, resets map[int]bool, res *Result,
	after func(b *plug.Block, at int, reset bool)) {
	at := 0
	for i := range blocks {
		b := &blocks[i]
		if resets[at] {
			pr.Reset()
		}
		for _, w := range b.NotesOut {
			w.Clear()
		}
		fill(b.Out)
		fill(b.LiveOut)
		fill(b.ControlOut)
		for _, port := range b.AuxOut {
			fill(port)
		}
		pr.Process(b)
		keep(res.Out, b.Out, at)
		keep(res.ControlOut, b.ControlOut, at)
		keep(res.LiveOut, b.LiveOut, at)
		for i, port := range b.AuxOut {
			keep(res.AuxOut[i], port, at)
		}
		if after != nil {
			after(b, at, resets[at])
		}
		at += b.Frames
	}
}

// fill fills each of chans with NaN.
func fill(chans [][]float32) {
	nan := float32(math.NaN())
	for _, ch := range chans {
		for f := range ch {
			ch[f] = nan
		}
	}
}

// keep copies each of chans to dst's, from frame at.
func keep(dst, chans [][]float32, at int) {
	for c, ch := range chans {
		copy(dst[c][at:], ch)
	}
}
