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

// goneFrom is gone for rows that are some of was, in its order: one pass
// along the two.
func goneFrom(was, rows []entry) []int {
	var out []int
	j := 0
	for i, e := range was {
		if j < len(rows) && rows[j].Name == e.Name {
			j++
			continue
		}
		out = append(out, i)
	}
	return out
}

// goneAlong is gone for was and rows that are both some of all, in its
// order, as a folder's rows filtered two ways are: one pass along all.
func goneAlong(all, was, rows []entry) []int {
	var out []int
	i, j := 0, 0
	for _, e := range all {
		if j < len(rows) && rows[j].Name == e.Name {
			j++
			if i < len(was) && was[i].Name == e.Name {
				i++
			}
			continue
		}
		if i < len(was) && was[i].Name == e.Name {
			out = append(out, i)
			i++
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
