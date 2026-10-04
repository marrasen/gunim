package gunim

import (
	"slices"
	"testing"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// fileBasket takes files dragged in from another program, in a square
// 300 on a side, and records what it heard.
type fileBasket struct{ recorder }

func (b *fileBasket) Handle(e input.Event, _ *UI) bool {
	switch e := e.(type) {
	case input.DragOver:
		_, ok := e.Data.(input.Files)
		b.events = append(b.events, e)
		return ok
	case input.Drop, input.DragLeave:
		b.events = append(b.events, e)
		return true
	}
	return false
}

func (b *fileBasket) Layout(Constraints, Frame, Children) geom.Size { return geom.Sz(300, 300) }

// heard returns what b heard since the last call, and forgets it.
func (b *fileBasket) heard() []input.Event {
	out := b.events
	b.events = nil
	return out
}

func TestFilesDraggedInAreOfferedToTheNodesUnderThemAsTheyMove(t *testing.T) {
	w := newTestWindow()
	bk := &fileBasket{}
	w.ui.Insert(w.ui.Root(), bk)
	run(w, 1)
	files := []string{"/music/a.mp3"}

	// The system tells the files a moment after the drag arrives.
	w.Input(driver.FilesOver{Pos: geom.Pt(10, 10)})
	w.Input(driver.FilesOver{Pos: geom.Pt(20, 30), Paths: files, Mods: input.ModShift})
	got := bk.heard()
	if len(got) != 2 {
		t.Fatalf("two moves over the basket brought %v", got)
	}
	first, _ := got[0].(input.DragOver)
	second, _ := got[1].(input.DragOver)
	if f, ok := first.Data.(input.Files); !ok || len(f.Paths) != 0 {
		t.Fatalf("the first move brought %v, want Files with none known yet", got[0])
	}
	if f, ok := second.Data.(input.Files); !ok || !slices.Equal(f.Paths, files) || second.Pos != geom.Pt(20, 30) || second.Mods != input.ModShift {
		t.Fatalf("the second move brought %+v, want the files at 20,30 with Shift", got[1])
	}

	// Off the basket, it hears the files left; back, it hears them
	// again; and as they leave the window, it hears that.
	w.Input(driver.FilesOver{Pos: geom.Pt(500, 10), Paths: files})
	if left := bk.heard(); len(left) != 1 {
		t.Fatalf("a move off the basket brought %v, want DragLeave", left)
	} else if _, ok := left[0].(input.DragLeave); !ok {
		t.Fatalf("a move off the basket brought %v, want DragLeave", left)
	}
	w.Input(driver.FilesOver{Pos: geom.Pt(10, 10), Paths: files})
	w.Input(driver.FilesLeft{})
	if back := bk.heard(); len(back) != 2 {
		t.Fatalf("a move back and a leave brought %v, want DragOver and DragLeave", back)
	} else if _, ok := back[1].(input.DragLeave); !ok {
		t.Fatalf("the files leaving the window brought %v, want DragLeave", back[1])
	}

	// A drop reaches the basket with the files as its Data, and no
	// leave follows.
	w.Input(driver.FilesOver{Pos: geom.Pt(10, 10), Paths: files})
	w.Input(input.Drop{Pos: geom.Pt(10, 10), Paths: files})
	run(w, 1)
	got = bk.heard()
	if len(got) != 2 {
		t.Fatalf("a move and a drop brought %v", got)
	}
	d, ok := got[1].(input.Drop)
	if f, isFiles := d.Data.(input.Files); !ok || !isFiles || !slices.Equal(f.Paths, files) || !slices.Equal(d.Paths, files) {
		t.Fatalf("the drop brought %v, want a Drop of the files", got[1])
	}
	w.Input(driver.FilesLeft{})
	if after := bk.heard(); len(after) != 0 {
		t.Fatalf("after the drop the basket heard %v", after)
	}
}
