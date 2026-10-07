package widget

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

// bigCode returns n lines of Go, about 42 bytes each.
func bigCode(n int) string {
	var b strings.Builder
	b.WriteString("package big\n\nfunc f() {\n")
	for i := range n - 4 {
		fmt.Fprintf(&b, "\tv%d := fmt.Sprintf(\"line %%d\", %d) // note\n", i, i)
	}
	b.WriteString("}\n")
	return b.String()
}

// bigLine returns about n bytes of words on one line.
func bigLine(n int) string {
	var b strings.Builder
	for i := 0; b.Len() < n; i++ {
		fmt.Fprintf(&b, "word%d ", i%977)
	}
	return b.String()
}

// benchStage mounts root in an offscreen window, 800 by 600, focuses
// the node at the click and runs a few frames.
func benchStage(b *testing.B, root gunim.Node, click geom.Point) *gunim.Window {
	b.Helper()
	w := gunimtest.New(b, geom.Sz(800, 600), nil)
	gunim.RegisterView(w, "stage", func(struct{}) gunim.Node { return root }, nil)
	if err := w.Client().Mount(gunim.Root, "stage", "stage", nil); err != nil {
		b.Fatal(err)
	}
	w.Frame(time.Second / 60)
	w.Input(input.PointerDown{Pos: click, Clicks: 1})
	w.Input(input.PointerUp{Pos: click})
	for range 30 {
		w.Frame(time.Second / 60)
	}
	return w
}

// benchIdle times a frame in which nothing but the pointer moved.
func benchIdle(b *testing.B, w *gunim.Window) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		w.Input(input.PointerMove{Pos: geom.Pt(float32(700+i%2), 590)})
		w.Frame(time.Second / 60)
	}
	b.StopTimer()
	b.ReportMetric(float64(len(w.Offscreen().Ops())), "ops/frame")
}

// benchKeys times a key and the frame after it: typing a letter and
// deleting it in turn, so the text keeps its length.
func benchKeys(b *testing.B, w *gunim.Window) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		if i%2 == 0 {
			w.Input(input.TextInput{Text: "x"})
		} else {
			w.Input(input.KeyPress{Key: input.KeyBackspace})
		}
		w.Frame(time.Second / 60)
	}
}

// bigArea returns a text area 50 000 lines long, its caret in the
// middle.
func bigArea(b *testing.B) *gunim.Window {
	a := NewTextArea()
	a.SetText(bigCode(50000), nil)
	w := benchStage(b, &frame{child: a, size: geom.Sz(600, 400)}, geom.Pt(20, 20))
	a.set(len(a.text)/2, false)
	w.Input(input.KeyPress{Key: input.KeyRight})
	for range 30 {
		w.Frame(time.Second / 60)
	}
	return w
}

func BenchmarkTextAreaIdleFrame(b *testing.B) {
	w := bigArea(b)
	benchIdle(b, w)
}

func BenchmarkTextAreaKey(b *testing.B) {
	w := bigArea(b)
	benchKeys(b, w)
}

// bigCodeEditor returns a code editor 50 000 lines long, its caret in
// the middle.
func bigCodeEditor(b *testing.B) *gunim.Window {
	c := NewCodeEditor()
	c.SetText(bigCode(50000), nil)
	w := benchStage(b, &frame{child: c, size: geom.Sz(600, 400)}, geom.Pt(300, 20))
	c.set(len(c.text)/2, false)
	w.Input(input.KeyPress{Key: input.KeyRight})
	for range 30 {
		w.Frame(time.Second / 60)
	}
	return w
}

func BenchmarkCodeEditorIdleFrame(b *testing.B) {
	w := bigCodeEditor(b)
	benchIdle(b, w)
}

func BenchmarkCodeEditorKey(b *testing.B) {
	w := bigCodeEditor(b)
	benchKeys(b, w)
}

// BenchmarkCodeEditorComment times opening a comment in the middle of
// 50 000 lines, which colours every line after it, and closing it.
func BenchmarkCodeEditorComment(b *testing.B) {
	w := bigCodeEditor(b)
	w.Input(input.KeyPress{Key: input.KeyHome})
	w.Frame(time.Second / 60)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		if i%2 == 0 {
			w.Input(input.TextInput{Text: "/*"})
		} else {
			w.Input(input.KeyPress{Key: input.KeyZ, Mods: input.ModControl})
		}
		w.Frame(time.Second / 60)
	}
}

// bigField returns a text field holding a megabyte on one line, its
// caret in the middle.
func bigField(b *testing.B) *gunim.Window {
	t := NewTextField()
	t.SetText(bigLine(1<<20), nil)
	w := benchStage(b, &frame{child: t, size: geom.Sz(600, 36)}, geom.Pt(20, 18))
	t.set(len(t.text)/2, false)
	w.Input(input.KeyPress{Key: input.KeyRight})
	for range 30 {
		w.Frame(time.Second / 60)
	}
	return w
}

func BenchmarkTextFieldIdleFrame(b *testing.B) {
	w := bigField(b)
	benchIdle(b, w)
}

func BenchmarkTextFieldKey(b *testing.B) {
	w := bigField(b)
	benchKeys(b, w)
}

// BenchmarkUndoMemory measures what 500 steps of undo hold on to in a
// text of a megabyte: each edit is a paste, a step of its own.
func BenchmarkUndoMemory(b *testing.B) {
	var held float64
	for range b.N {
		e := &editor{text: []rune(bigLine(1 << 20)), multiline: true}
		u := (*gunim.UI)(nil)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for i := range maxUndo {
			e.set(len(e.text)/2+i, false)
			e.last = otherEdit
			e.pasting = true
			e.insert("x", u)
			e.pasting = false
		}
		runtime.GC()
		runtime.ReadMemStats(&after)
		held = float64(after.HeapAlloc) - float64(before.HeapAlloc)
		runtime.KeepAlive(e)
	}
	b.ReportMetric(held/(1<<20), "MB-held")
}
