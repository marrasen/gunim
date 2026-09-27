package text

import (
	"math/rand"
	"strings"
	"testing"
)

// cr, lf and ls are a carriage return, a newline and a Unicode line separator.
var cr, lf, ls = string(rune(0xd)), string(rune(0xa)), string(rune(0x2028))

func TestWindowsLineEndsWrapAndEndLines(t *testing.T) {
	line := "The quick brown fox jumps over the lazy dog, and some more words follow here."
	s := strings.Repeat(line+cr+lf, 3)
	for w := float32(1); w < 600; w += 7 {
		p := Default().Layout(s, Style{Size: 12}, w)
		if len(p.Lines) < 4 {
			t.Fatalf("at %v px, %d lines, want at least the three lines and the empty last one", w, len(p.Lines))
		}
	}
	p := Default().Layout(s, Style{Size: 12}, 0)
	runes := []rune(s)
	for i, l := range p.Lines[:3] {
		if got := string(runes[l.Run.Start:l.Run.End]); got != line {
			t.Fatalf("line %d holds %q, want the line without its ending", i, got)
		}
	}
}

func TestEveryKindOfLineBreakEndsALine(t *testing.T) {
	s := "one" + cr + "two" + lf + "three" + cr + lf + "four" + ls + "five"
	p := Default().Layout(s, Style{Size: 12}, 0)
	runes := []rune(s)
	got := make([]string, 0, len(p.Lines))
	for _, l := range p.Lines {
		got = append(got, string(runes[l.Run.Start:l.Run.End]))
	}
	if strings.Join(got, ",") != "one,two,three,four,five" {
		t.Fatalf("the lines are %q", got)
	}
}

func TestLayoutSurvivesOddText(t *testing.T) {
	pieces := make([]string, 0, 40)
	pieces = append(pieces, "/", `\`, "a", "C:", ":", "-", ".", " ", "(", "}", "fi", "->", "123", cr+lf)
	for _, r := range []rune{0x9, 0xd, 0xa, 0xb, 0xc, 0x85, 0x2028, 0x2029, 0x0, 0x1b, 0xad, 0xfeff, 0x301, 0x200b,
		0x200d, 0xfe0f, 0xa0, 0x1f600, 0x1f468, 0x627, 0x5e9, 0x4e2d, 0x2014, 0x2026} {
		pieces = append(pieces, string(r))
	}
	r := rand.New(rand.NewSource(7))
	for _, f := range []*Face{Default(), GoMono(false, false)} {
		for range 3000 {
			var b strings.Builder
			for k := r.Intn(40); k >= 0; k-- {
				b.WriteString(pieces[r.Intn(len(pieces))])
			}
			w := float32(r.Intn(400))
			lines := r.Intn(3)
			func() {
				defer func() {
					if e := recover(); e != nil {
						t.Fatalf("laying out %q at %v px, %d lines, panics: %v", b.String(), w, lines, e)
					}
				}()
				f.Layout(b.String(), Style{Size: 12, MaxLines: lines}, w)
			}()
		}
	}
}
