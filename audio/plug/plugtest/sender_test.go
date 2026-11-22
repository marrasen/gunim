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
