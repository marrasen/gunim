package widget

import (
	"strconv"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
)

// BenchmarkADataGridFrameWhileManyRowsLeave times a frame of a grid of 200 000 rows while its first 100 000 leave.
func BenchmarkADataGridFrameWhileManyRowsLeave(b *testing.B) {
	g := NewDataGrid(GridColumn{Title: "Name"})
	g.Row = func(i int) (GridRow, bool) { return GridRow{Cells: [][]GridSpan{{{Text: strconv.Itoa(i)}}}}, true }
	g.rows = 200_000
	w := gunimtest.New(b, geom.Sz(400, 300), nil)
	gunim.RegisterView(w, "stage", func(struct{}) gunim.Node { return &frame{child: g, size: geom.Sz(400, 300)} }, nil)
	if err := w.Client().Mount(gunim.Root, "stage", "stage", nil); err != nil {
		b.Fatal(err)
	}
	type leave struct{}
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ leave, u *gunim.UI) {
		gone := make([]int, 0, 100_000)
		for i := range 100_000 {
			gone = append(gone, i)
		}
		g.Leave(gone, u)
		g.SetRows(100_000, u)
	})
	w.Frame(time.Second / 60)
	if err := w.Client().Patch("stage", leave{}); err != nil {
		b.Fatal(err)
	}
	w.Frame(time.Second / 60)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		// The rows keep leaving.
		g.leftAgo = 0
		w.Frame(time.Second / 60)
	}
	b.StopTimer()
	if len(g.gone) == 0 {
		b.Fatal("the rows have stopped leaving")
	}
}
