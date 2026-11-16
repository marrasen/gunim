package filemanager

import (
	"testing"
	"time"
)

// A program's TimeText says when each item in the list was modified.
func TestTheListSaysTimesAsTheProgramAsks(t *testing.T) {
	const said = "the program's time"
	h := newHarnessWith(t, func(o *Options) { o.TimeText = func(time.Time) string { return said } }, "a.txt", "b/")
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
