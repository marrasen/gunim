package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
)

type (
	dropFocus struct{}
	cursorNav struct{ I int }
)

// keyedPages are navPages with a Key each.
func keyedPages() []NavItem {
	items := navPages()
	for i, k := range []Key{"overview", "storage", "activity", "cantsync", "settings"} {
		items[i].Key = k
	}
	return items
}

// stageKeyedNav stages n with patches that drop focus, and that put its ring on a row.
func stageKeyedNav(t *testing.T, n *NavList) (w *gunim.Window, run func(int)) {
	t.Helper()
	w, run = stageNav(t, n)
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ dropFocus, u *gunim.UI) { u.Focus(nil) })
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, c cursorNav, u *gunim.UI) { n.SetCursor(c.I, u) })
	return w, run
}

func patchNav(t *testing.T, w *gunim.Window, v any) {
	t.Helper()
	if err := w.Client().Patch("stage", v); err != nil {
		t.Fatal(err)
	}
}

// ringAt runs frames until the ring settles, and returns the row it is drawn on, or -1 for between rows.
func ringAt(n *NavList, run func(int)) int {
	run(60)
	for i := range n.items {
		if d := n.ringY.Value() - n.top(i); d < 0.01 && d > -0.01 {
			return i
		}
	}
	return -1
}

func TestANavListsRingAndPillStayOnTheirRowsAsRowsMove(t *testing.T) {
	n := NewNavList(keyedPages()...)
	n.SetSelected(1, nil)
	w, run := stageKeyedNav(t, n)
	run(1)
	patchNav(t, w, cursorNav{2})
	run(1)

	// A row comes in at the top, and Activity and Can't sync change places.
	was := keyedPages()
	items := []NavItem{{Key: "shared", Label: "Shared with me", Icon: icon.Folder}, was[0], was[1], was[3], was[2], was[4]}
	patchNav(t, w, setNavItems{items})
	run(1)
	if got := n.Items()[n.Cursor()].Key; got != "activity" {
		t.Fatalf("after the rows moved the focus ring is on %q, want it still on activity", got)
	}
	if got := n.Items()[n.Selected()].Key; got != "storage" {
		t.Fatalf("after the rows moved the chosen row is %q, want it still storage", got)
	}
	if at := ringAt(n, run); at != n.Cursor() {
		t.Fatalf("the ring is drawn on row %d, want row %d, where activity is now", at, n.Cursor())
	}
	if access := n.Access(); access.Parts[access.Active-1].Name != "Activity" {
		t.Fatalf("a screen reader hears the ring on %q, want Activity", access.Parts[access.Active-1].Name)
	}
	// Can't sync kept its count as it moved, so its badge stays and does not grow in again.
	moved := 3
	if b := n.badges[moved]; b != nil && b.shown.Active() {
		t.Fatal("the badge of the row that moved grows in again")
	}

	// Rows with no Key keep their place, as before.
	plain := NewNavList(navPages()...)
	plain.SetCursor(2, nil)
	plain.SetItems(append([]NavItem{{Label: "Shared with me"}}, navPages()...), nil)
	if plain.Cursor() != 2 {
		t.Fatalf("with no keys the ring went to row %d, want it to keep its place, 2", plain.Cursor())
	}
}

func TestFocusingANavListAgainKeepsTheRingWhereItWasLeft(t *testing.T) {
	n := NewNavList(keyedPages()...)
	w, run := stageKeyedNav(t, n)
	run(1)
	w.Input(input.KeyPress{Key: input.KeyTab})
	run(1)
	w.Input(input.KeyPress{Key: input.KeyDown})
	w.Input(input.KeyPress{Key: input.KeyDown})
	run(1)
	if n.Cursor() != 2 || n.Selected() != 0 {
		t.Fatalf("two Downs left the ring on %d and the choice on %d, want 2 and 0", n.Cursor(), n.Selected())
	}
	// Focus goes elsewhere, and Tab brings it back.
	patchNav(t, w, dropFocus{})
	run(1)
	w.Input(input.KeyPress{Key: input.KeyTab})
	run(1)
	if at := ringAt(n, run); n.Cursor() != 2 || at != 2 {
		t.Fatalf("focused again, the ring is on row %d, drawn on %d; want it where it was left, 2", n.Cursor(), at)
	}
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(1)
	if n.Selected() != 2 {
		t.Fatalf("Enter on the ring's row chose %d, want 2", n.Selected())
	}
}

func TestSetCursorPutsANavListsRingOnARow(t *testing.T) {
	n := NewNavList(keyedPages()...)
	n.SetCursor(3, nil)
	if n.Cursor() != 3 || n.ringY.Value() != n.top(3) {
		t.Fatal("SetCursor before layout did not jump the ring to row 3")
	}
	n.SetCursor(9, nil)
	if n.Cursor() != 3 {
		t.Fatalf("SetCursor to a row past the end moved the ring to %d", n.Cursor())
	}
	w, run := stageKeyedNav(t, n)
	run(1)
	patchNav(t, w, cursorNav{1})
	run(1)
	if !n.ringY.Active() {
		t.Fatal("once laid out, SetCursor jumped the ring rather than gliding it")
	}
	if at := ringAt(n, run); at != 1 || n.Selected() != 0 {
		t.Fatalf("SetCursor(1) put the ring on %d and the choice on %d, want 1 and 0", at, n.Selected())
	}
	if got := sent(w); len(got) != 0 {
		t.Fatalf("SetCursor sent %v", got)
	}

	// A ring on the chosen row goes along when the app chooses another; one left elsewhere stays.
	patchNav(t, w, chooseNav{1})
	run(1)
	patchNav(t, w, chooseNav{4})
	if at := ringAt(n, run); at != 4 {
		t.Fatalf("the ring was on the chosen row, and stayed on %d when row 4 was chosen", at)
	}
	patchNav(t, w, cursorNav{2})
	patchNav(t, w, chooseNav{0})
	if at := ringAt(n, run); at != 2 {
		t.Fatalf("the ring was left on row 2, and went to %d when row 0 was chosen", at)
	}
}

func TestANavListsLabelNamesItWithoutATooltip(t *testing.T) {
	n := NewNavList(navPages()...)
	n.Label = "Pages"
	_, run := stageNav(t, n)
	run(1)
	if name := n.Access().Name; name != "Pages" || n.Tooltip != "" {
		t.Fatalf("with Label set the list reads as %q, with tooltip %q; want Pages and no tooltip", name, n.Tooltip)
	}
	n.Tooltip = "Move between pages"
	if name := n.Access().Name; name != "Pages" {
		t.Fatalf("with both set the list reads as %q, want its Label", name)
	}
	n.Label = ""
	if name := n.Access().Name; name != "Move between pages" {
		t.Fatalf("with Label empty the list reads as %q, want its tooltip", name)
	}
}
