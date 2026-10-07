package widget

import (
	"fmt"
	"strings"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// manyItems returns n items, as a long list of fonts or of records might be.
func manyItems(n int) []string {
	items := make([]string, n)
	for i := range items {
		items[i] = fmt.Sprintf("Item number %d", i)
	}
	return items
}

// BenchmarkClosedDropdownLayout lays out a closed drop-down of 100 000 items, as each frame does.
func BenchmarkClosedDropdownLayout(b *testing.B) {
	d := NewDropdown(manyItems(100_000)...)
	f := gunim.Frame{Scale: 1}
	c := gunim.Loose(geom.Sz(400, 36))
	d.Layout(c, f, gunim.Children{})
	b.ResetTimer()
	for range b.N {
		d.Layout(c, f, gunim.Children{})
	}
}

// BenchmarkOpenMenuFrame lays out and paints an open menu of 100 000 items, scrolled half way, as each frame does.
func BenchmarkOpenMenuFrame(b *testing.B) {
	m := NewMenu(manyItems(100_000)...)
	m.Breaks = []int{10, 20_000, 50_005, 90_000}
	m.Highlight(50_000)
	f := gunim.Frame{Scale: 1}
	c := gunim.Loose(geom.Sz(600, 480))
	var p paint.Painter
	m.Layout(c, f, gunim.Children{})
	b.ResetTimer()
	for range b.N {
		m.Layout(c, f, gunim.Children{})
		p.Reset()
		m.Paint(&p, f, geom.Sz(600, 480), gunim.Children{})
	}
}

// BenchmarkOpenDropdownMenu opens the list of a drop-down of 100 000 items: its menu's first layout and paint.
func BenchmarkOpenDropdownMenu(b *testing.B) {
	d := NewDropdown(manyItems(100_000)...)
	d.Selected = 50_000
	f := gunim.Frame{Scale: 1}
	d.Layout(gunim.Loose(geom.Sz(400, 36)), f, gunim.Children{})
	c := gunim.Loose(geom.Sz(600, 480))
	var p paint.Painter
	b.ResetTimer()
	for range b.N {
		m := d.listMenu()
		m.Layout(c, f, gunim.Children{})
		p.Reset()
		m.Paint(&p, f, geom.Sz(600, 480), gunim.Children{})
	}
}

// BenchmarkOpenMenu opens a menu of 100 000 items that nothing has measured: its first layout and paint.
func BenchmarkOpenMenu(b *testing.B) {
	items := manyItems(100_000)
	f := gunim.Frame{Scale: 1}
	c := gunim.Loose(geom.Sz(600, 480))
	var p paint.Painter
	for range b.N {
		m := NewMenu(items...)
		m.Layout(c, f, gunim.Children{})
		p.Reset()
		m.Paint(&p, f, geom.Sz(600, 480), gunim.Children{})
	}
}

func TestAClosedDropdownMeasuresItsItemsOnlyWhenTheyChange(t *testing.T) {
	d := NewDropdown(manyItems(2000)...)
	f := gunim.Frame{Scale: 1}
	c := gunim.Loose(geom.Sz(800, 36))
	d.Layout(c, f, gunim.Children{})
	short := d.size.W
	if allocs := testing.AllocsPerRun(5, func() { d.Layout(c, f, gunim.Children{}) }); allocs > 0 {
		t.Fatalf("a layout with the items as they were allocates %v times, measuring them again", allocs)
	}
	d.Items = append(manyItems(10), strings.Repeat("W", 40))
	d.Layout(c, f, gunim.Children{})
	if d.size.W <= short {
		t.Fatalf("with a longer item the drop-down is %v wide, as it was before", d.size.W)
	}
}

// textOps counts the text ops in ops.
func textOps(ops []paint.Op) int {
	n := 0
	for _, op := range ops {
		if _, ok := op.(*paint.TextOp); ok {
			n++
		}
	}
	return n
}

func TestALongMenuDrawsOnlyTheRowsInView(t *testing.T) {
	m := NewMenu(manyItems(5000)...)
	m.Breaks = []int{100, 2600, 2601}
	m.Highlight(2500)
	m.Layout(gunim.Loose(geom.Sz(600, 480)), gunim.Frame{Scale: 1}, gunim.Children{})
	shown := int(m.card.Size().H/m.row) + 2
	if n := textOps(painted(m, geom.Sz(600, 480))); n > shown {
		t.Fatalf("a menu showing about %d rows drew %d texts", shown, n)
	}
	rowShown(t, m, 2500, "opened on it")
}

func TestOpeningALongDropdownMeasuresNoItemAgain(t *testing.T) {
	d := NewDropdown(manyItems(5000)...)
	d.Selected = 2500
	f := gunim.Frame{Scale: 1}
	d.Layout(gunim.Loose(geom.Sz(400, 36)), f, gunim.Children{})
	open := func() {
		m := d.listMenu()
		m.Layout(gunim.Loose(geom.Sz(600, 480)), f, gunim.Children{})
		painted(m, geom.Sz(600, 480))
	}
	// Measuring an item takes many allocations, and 5000 of them many more than the rows in view take.
	if allocs := testing.AllocsPerRun(2, open); allocs > 2000 {
		t.Fatalf("opening the list allocates %v times", allocs)
	}
}

func TestALongMenusRowsAreFoundPastItsLinesAndCaptions(t *testing.T) {
	m := NewMenu(manyItems(3000)...)
	for i := 5; i < 3000; i += 7 {
		m.Breaks = append(m.Breaks, i)
	}
	m.Captions = []int{0, 700, 2999}
	m.Layout(gunim.Loose(geom.Sz(600, 480)), gunim.Frame{Scale: 1}, gunim.Children{})
	for _, i := range []int{0, 1, 4, 5, 6, 699, 700, 701, 1500, 2998, 2999} {
		m.scroll.jumpTo(m.scroll.clamp(m.rowTop(i) - m.rowTop(0) - 100))
		r := m.RowRect(i)
		if got := m.rowAt(geom.Pt(r.Min.X+20, r.Center().Y)); got != i {
			t.Fatalf("the middle of row %d's rectangle %v finds row %d", i, r, got)
		}
		if i%7 == 5 {
			if got := m.rowAt(geom.Pt(r.Min.X+20, r.Min.Y-menuBreak/2)); got != -1 {
				t.Fatalf("the line above row %d finds row %d", i, got)
			}
		}
	}
	if m.enabled(700) || !m.enabled(701) {
		t.Fatal("a caption counts as an item to pick, or the item below it does not")
	}
}
