package text

import (
	"slices"
	"strings"
	"testing"
)

// lines returns the text of each line s is set in at width.
func lines(s string, width float32) []string {
	p := Default().Layout(s, Style{Size: 12}, width)
	runes := []rune(s)
	out := make([]string, 0, len(p.Lines))
	for _, l := range p.Lines {
		out = append(out, string(runes[l.Run.Start:l.Run.End]))
	}
	return out
}

func TestAPathBreaksAfterItsSeparators(t *testing.T) {
	for _, s := range []string{
		`C:\Users\marcusj\AppData\Local\Temp\gunim-files-demo-123\Sample`,
		`/home/marcus/.local/share/Trash/files/report-2026.txt`,
	} {
		sep := "/"
		if strings.ContainsRune(s, 0x5c) {
			sep = string(rune(0x5c))
		}
		got := lines(s, 150)
		if len(got) < 2 {
			t.Fatalf("%q sets in one line at 150 px", s)
		}
		if strings.Join(got, "") != s {
			t.Fatalf("the lines %q lose text of %q", got, s)
		}
		for i, l := range got[:len(got)-1] {
			if !strings.HasSuffix(l, sep) {
				t.Fatalf("line %d of %q is %q, which ends between separators", i, s, l)
			}
		}
	}
}

func TestADriveKeepsItsColon(t *testing.T) {
	got := lines(`C:\Users\marcusj\AppData\Local\Temp`, 100)
	if got[0] == "C:" {
		t.Fatalf("the path breaks after its drive: %q", got)
	}
}

func TestALongPartOfAPathStillBreaks(t *testing.T) {
	long := strings.Repeat("x", 80)
	got := lines(`/tmp/`+long, 100)
	if len(got) < 3 || got[0] != "/tmp/" {
		t.Fatalf("a part too wide for a line sets as %q", got)
	}
}

func TestProseBreaksAsBefore(t *testing.T) {
	s := "one two-three four, five: six"
	if !slices.Equal(breakText([]rune(s)), []rune(s)) {
		t.Fatal("the breaker reads prose with no path in it differently")
	}
}
