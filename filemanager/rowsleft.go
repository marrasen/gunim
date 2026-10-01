package filemanager

import "github.com/marrasen/gunim"

func registerRowsLeft(w *gunim.Window) {
	gunim.RegisterPatch(w, "browser", func(b *browser, l RowsLeft, _ *gunim.UI) {
		if pg := b.listing.cur; pg != nil && l.Gen == pg.gen {
			pg.left = &l
		}
	})
}

// gone returns the indexes in was of the entries rows no longer holds, by
// name.
func gone(was, rows []entry) []int {
	if len(was) == 0 {
		return nil
	}
	kept := make(map[string]bool, len(rows))
	for _, e := range rows {
		kept[e.Name] = true
	}
	var out []int
	for i, e := range was {
		if !kept[e.Name] {
			out = append(out, i)
		}
	}
	return out
}

// leave has the rows that went in the listing's last change leave, once
// the new rows arrive, with the grid still drawing the old ones.
func (pg *listingPage) leave(u *gunim.UI) {
	if pg.left != nil && pg.left.Gen == pg.gen {
		pg.grid.Leave(pg.left.Rows, u)
	}
	pg.left = nil
}
