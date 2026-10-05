package band

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// tieredParts returns parts of the fake pieces: two of tier 1, one each
// of tiers 2 and 3, and two of tier 4, the solo looping twice.
func tieredParts() []Part {
	parts := fakeParts(6)[:6]
	for i, t := range []int{1, 1, 2, 3, 4, 4} {
		parts[i].Tier = t
		parts[i].Name = fmt.Sprintf("t%d-%d", t, i)
	}
	parts[5].Loops = 2
	return parts
}

// row writes what each part plays in a phrase: - resting, i intro, l
// loop, o outro.
func row(s Status) string {
	var b strings.Builder
	for _, p := range s.Parts {
		switch {
		case !p.Playing:
			b.WriteByte('-')
		default:
			b.WriteByte(p.Piece.String()[0])
		}
	}
	return b.String()
}

// playPhrase reads b to the start of its next phrase.
func playPhrase(t *testing.T, b *TierBand) {
	t.Helper()
	buf := make([]float32, 2*(b.t.phraseAt(b.phrase+1)-b.at))
	if _, err := b.Read(buf); err != nil {
		t.Fatal(err)
	}
}

func TestTiersAddTheirPartsAndTheSoloComesRound(t *testing.T) {
	s := &Tiers{Title: "Fake", BPM: 136, PhraseBars: 16, Parts: tieredParts()}
	b := NewTiers(s)
	if b.Tiers() != 4 || b.Tier() != 1 {
		t.Fatalf("%d tiers, at tier %d; want 4, at 1", b.Tiers(), b.Tier())
	}
	var got []string
	got = append(got, row(b.Watch()))
	for p := 1; p <= 11; p++ {
		switch p {
		case 1:
			// Asked for while phrase 0 plays, it starts at phrase 1.
			b.SetTier(4)
		case 9:
			b.SetTier(2)
		}
		playPhrase(t, b)
		got = append(got, row(b.Watch()))
	}
	want := []string{
		"ii----", // tier 1 comes in
		"lliiii", // tier 4 asked for in phrase 0
		"llllll",
		"llllll",
		"lllllo", // the solo has looped twice
		"llllli", // and comes in again
		"llllll",
		"llllll",
		"lllllo",
		"llloo-", // tier 2: tiers 3 and 4 leave
		"lll---",
		"lll---",
	}
	for p := range want {
		if got[p] != want[p] {
			t.Fatalf("phrase %d plays %s, want %s; all: %v", p, got[p], want[p], got)
		}
	}
}

func TestATierOutOfRangeIsHeldToTheSongs(t *testing.T) {
	b := NewTiers(&Tiers{BPM: 136, PhraseBars: 16, Parts: tieredParts(), Start: 9})
	if b.Tier() != 4 {
		t.Fatalf("a start at 9 plays tier %d, want 4", b.Tier())
	}
	b.SetTier(-2)
	if b.Tier() != 1 {
		t.Fatalf("tier -2 sets tier %d, want 1", b.Tier())
	}
}

func TestWatchCountsTheBars(t *testing.T) {
	b := NewTiers(&Tiers{BPM: 136, PhraseBars: 16, Parts: tieredParts()})
	buf := make([]float32, 2*(barAt(5)+10))
	if _, err := b.Read(buf); err != nil {
		t.Fatal(err)
	}
	if s := b.Watch(); s.Bar != 5 || s.PhraseBars != 16 || len(s.Parts) != 6 || s.Parts[0].Tier != 1 {
		t.Fatalf("Watch returned %+v", s)
	}
}

func TestPartsStartAndStopOnTheirOwnOverTheTiers(t *testing.T) {
	b := NewTiers(&Tiers{BPM: 136, PhraseBars: 16, Parts: tieredParts()})
	var got []string
	got = append(got, row(b.Watch()))
	for p := 1; p <= 6; p++ {
		switch p {
		case 1:
			// Asked for while phrase 0 plays: the tier-3 part comes in,
			// and a tier-1 part leaves.
			mustSet(t, b, "t3-3", PartOn)
			mustSet(t, b, "t1-0", PartOff)
		case 3:
			// The tier rises, and the part turned off stays off.
			b.SetTier(4)
		case 5:
			mustSet(t, b, "t1-0", PartAuto)
			mustSet(t, b, "t4-4", PartOff)
		}
		playPhrase(t, b)
		got = append(got, row(b.Watch()))
	}
	want := []string{
		"ii----",
		"ol-i--", // t1-0 leaves, and t3-3 comes in
		"-l-l--",
		"-lilii", // tier 4: the rest come in, and t1-0 stays off
		"-lllll",
		"illlol", // t1-0 back with the tier, and t4-4 off
		"llll-o", // the solo has looped twice
	}
	for p := range want {
		if got[p] != want[p] {
			t.Fatalf("phrase %d plays %s, want %s; all: %v", p, got[p], want[p], got)
		}
	}
	if s := b.Watch(); s.Parts[4].Control != PartOff || s.Parts[0].Control != PartAuto {
		t.Fatalf("Watch tells controls %v and %v, want off and auto", s.Parts[4].Control, s.Parts[0].Control)
	}
	if err := b.SetPart("no-such-part", PartOn); !errors.Is(err, ErrNoPart) {
		t.Fatalf("SetPart returned %v for a part the song has none of, want ErrNoPart", err)
	}
}

func mustSet(t *testing.T, b *TierBand, name string, c PartControl) {
	t.Helper()
	if err := b.SetPart(name, c); err != nil {
		t.Fatal(err)
	}
}
