package filemanager

import (
	"testing"
	"time"
)

// said is what the tests' TimeText says of every time.
const said = "the program's time"

func saysProgramTime(o *Options) { o.TimeText = func(time.Time) string { return said } }

// A program's TimeText says when each item in the list was modified.
func TestTheListSaysTimesAsTheProgramAsks(t *testing.T) {
	h := newHarnessWith(t, saysProgramTime, "a.txt", "b/")
	h.until("the rows say the program's time", func() bool {
		pg := h.b.listing.cur
		n := 0
		for i := range pg.grid.Rows() {
			r, ok := pg.view(i)
			if !ok || r.Modified != said {
				return false
			}
			n++
		}
		return n == 2
	})
}

// modifiedFact gives the Modified fact the preview shows, "" for none.
func (h *harness) modifiedFact() string {
	pv := h.b.preview
	if pv.cur == nil {
		return ""
	}
	for _, f := range pv.shown.Facts {
		if f.Label == "Modified" {
			return f.Value
		}
	}
	return ""
}

// The preview says when the folder and a file were modified in the program's words.
func TestThePreviewSaysTimesAsTheProgramAsks(t *testing.T) {
	h := newHarnessWith(t, saysProgramTime, "a.txt")
	h.until("the folder's preview says the program's time", func() bool {
		return h.b.preview.shown.Type == "This folder" && h.modifiedFact() == said
	})
	h.pick("a.txt")
	h.until("the file's preview says the program's time", func() bool {
		return h.b.preview.shown.Title == "a.txt" && h.modifiedFact() == said
	})
}

// The Properties dialog says its times in the program's words, and in its own without them.
func TestThePropertiesSayTimesAsTheProgramAsks(t *testing.T) {
	h := newHarness(t, "a.txt")
	path := h.a.ps.Join(h.dir, "a.txt")
	p, _, err := readProps(LocalFS(), []string{path}, func(time.Time) string { return said })
	if err != nil {
		t.Fatal(err)
	}
	if p.Modified != said {
		t.Fatalf("Modified says %q", p.Modified)
	}
	if p.Created != "" && p.Created != said {
		t.Fatalf("Created says %q", p.Created)
	}
	p, _, err = readProps(LocalFS(), []string{path}, fmtTime)
	if err != nil {
		t.Fatal(err)
	}
	if p.Modified == said || p.Modified == "" {
		t.Fatalf("with no program's words, Modified says %q", p.Modified)
	}
}
