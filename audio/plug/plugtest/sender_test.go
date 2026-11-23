package plugtest

import (
	"testing"

	"github.com/marrasen/gunim/audio/plug"
)

func TestDriveReturnsTheNoteASenderWrites(t *testing.T) {
	s := plug.Setup{Rate: 48000, Channels: 2, MaxBlock: 64}
	in := channels(2, 256)
	res := Drive(t, &Sender, Sender.New(nil).Processor(), s, 256, Ports{In: in, ResetAt: []int{192}})
	got := res.NotesOut[0]
	want := []plug.Event{SenderNote, {Frame: SenderFrames, Kind: plug.NoteOff, Note: SenderNote.Note},
		{Frame: 192, Kind: plug.NoteOn, Note: 1, Pitch: 60, Velocity: 1}}
	if len(got) != len(want) {
		t.Fatalf("the sender wrote %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d is %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDriveReturnsTheNewsATellerTells(t *testing.T) {
	s := plug.Setup{Rate: 48000, Channels: 2, MaxBlock: 64}
	res := Drive(t, &Teller, Teller.New(nil).Processor(), s, 1280, Ports{In: channels(2, 1280), NotesAhead: 256})
	news := res.NotesNews[0]
	if len(news) != 1 || news[0].Kind != plug.Coming || news[0].Frame != TellerFrame || news[0].Note != TellerNote.Note ||
		news[0].At < TellerFrame-256-64 || news[0].At+64 > TellerFrame {
		t.Fatalf("the teller told %+v, want one Coming of note %d landing at %d, told up to 256 frames ahead",
			news, TellerNote.Note, TellerFrame)
	}
	if got := res.NotesOut[0]; len(got) != 2 || got[0] != TellerNote {
		t.Errorf("the teller sent %+v, want its note at %d", got, TellerFrame)
	}
	if res := Drive(t, &Teller, Teller.New(nil).Processor(), s, 1280, Ports{In: channels(2, 1280)}); len(res.NotesNews[0]) != 0 {
		t.Errorf("with no look-ahead asked, the teller told %+v", res.NotesNews[0])
	}
}
