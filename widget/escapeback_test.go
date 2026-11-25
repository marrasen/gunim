package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// backStage mounts root in a window that leaves on Back, and returns it with its UI's holder and frames.
func backStage(t *testing.T, root gunim.Node) (*gunim.Window, *grab, func(int)) {
	t.Helper()
	g := &grab{child: root}
	w := gunim.NewOffscreenWith(gunim.WindowOptions{Size: geom.Sz(400, 600), LeaveOnBack: true})
	gunim.RegisterView(w, "stage", func(struct{}) gunim.Node { return g }, nil)
	if err := w.Client().Mount(gunim.Root, "stage", "stage", nil); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(2)
	return w, g, run
}

// A widget that takes Escape while it does something has a phone's Back for that, and then leaves Back to the program.
// Without the keyboard, where Escape does not reach it, it leaves Back at once.
func TestAWidgetThatTakesEscapeTakesBack(t *testing.T) {
	press := func(w *gunim.Window, k input.Key) { w.Input(input.KeyPress{Key: k, Time: time.Now()}) }
	for name, c := range map[string]func() (n gunim.Node, set func(w *gunim.Window, u *gunim.UI), busy func() bool){
		"a clearable field": func() (gunim.Node, func(*gunim.Window, *gunim.UI), func() bool) {
			f := NewTextField()
			f.Clearable = true
			return f, func(_ *gunim.Window, u *gunim.UI) { f.SetText("hello", u) }, func() bool { return f.Text() != "" }
		},
		"a number field": func() (gunim.Node, func(*gunim.Window, *gunim.UI), func() bool) {
			f := NewNumberField(0, 10)
			f.Clearable = true
			f.SetText("", nil)
			return f, func(_ *gunim.Window, u *gunim.UI) { f.SetText("5", u) }, func() bool { return f.Text() != "" }
		},
		"a grid's selection": func() (gunim.Node, func(*gunim.Window, *gunim.UI), func() bool) {
			g := NewDataGrid(GridColumn{Title: "Name"})
			g.SetRows(5, nil)
			return g, func(_ *gunim.Window, u *gunim.UI) { g.SetSelected(2, u) }, func() bool { return g.Selected() >= 0 }
		},
		"a grid's many selected": func() (gunim.Node, func(*gunim.Window, *gunim.UI), func() bool) {
			g := NewDataGrid(GridColumn{Title: "Name"})
			g.Multi = true
			g.SetRows(5, nil)
			return g, func(_ *gunim.Window, u *gunim.UI) { g.SetSelectedRows([][2]int{{0, 2}}, 1, u) },
				func() bool { return g.selected >= 0 || len(g.runs) > 0 }
		},
		"a tile grid's selection": func() (gunim.Node, func(*gunim.Window, *gunim.UI), func() bool) {
			g := NewTileGrid(geom.Sz(80, 60))
			g.SetLen(5, nil)
			return g, func(_ *gunim.Window, u *gunim.UI) { g.SetSelected([][2]int{{0, 2}}, 1, u) },
				func() bool { return g.cursor >= 0 || len(g.runs) > 0 }
		},
		"a PIN's digits": func() (gunim.Node, func(*gunim.Window, *gunim.UI), func() bool) {
			p := NewPinPad(4, "Enter")
			return p, func(w *gunim.Window, _ *gunim.UI) { press(w, input.Key1) }, func() bool { return len(p.digits) > 0 }
		},
	} {
		t.Run(name, func(t *testing.T) {
			n, set, busy := c()
			w, g, run := backStage(t, n)
			off := w.Offscreen()
			back := func() { w.Input(driver.BackAsked{Time: time.Now()}) }
			g.ui.Focus(n)
			run(1)
			if busy() || !off.BackLeaves() {
				t.Fatalf("with nothing for Escape to do, Back leaves: %v", off.BackLeaves())
			}
			set(w, g.ui)
			run(1)
			if !busy() || off.BackLeaves() {
				t.Fatalf("busy %v, and Back leaves: %v", busy(), off.BackLeaves())
			}
			back()
			if busy() || off.AppLeft() != 0 {
				t.Fatalf("after Back it is busy: %v, and the program was left %d times", busy(), off.AppLeft())
			}
			run(1)
			if !off.BackLeaves() {
				t.Fatal("with it done, Back stays")
			}
			// Without the keyboard, Back is not for it, and goes nowhere but out.
			set(w, g.ui)
			g.ui.Focus(nil)
			run(1)
			if !busy() || !off.BackLeaves() {
				t.Fatalf("without the keyboard: busy %v, Back leaves: %v", busy(), off.BackLeaves())
			}
			back()
			if !busy() || off.AppLeft() != 1 {
				t.Fatalf("without the keyboard Back left the program %d times", off.AppLeft())
			}
		})
	}
}

// A table gives up finding a name typed, and an address bar its edit, and a menubar the keyboard.
func TestBackEndsWhatEscapeEndsInATableAnAddressBarAndAMenubar(t *testing.T) {
	back := func(w *gunim.Window) { w.Input(driver.BackAsked{Time: time.Now()}) }
	tbl := NewTable(TableColumn{Title: "Name"})
	tbl.Row = func(k Key) TableRow { return TableRow{Cells: []string{string(k)}} }
	tbl.SetKeys([]Key{"ada", "bo"}, nil)
	w, g, run := backStage(t, tbl)
	g.ui.Focus(tbl)
	run(1)
	w.Input(input.TextInput{Text: "b"})
	if w.Offscreen().BackLeaves() {
		t.Fatal("while a name is being found Back leaves")
	}
	back(w)
	if tbl.typed != "" || w.Offscreen().AppLeft() != 0 || !w.Offscreen().BackLeaves() {
		t.Fatalf("after Back %q is being found, and the program was left %d times", tbl.typed, w.Offscreen().AppLeft())
	}

	bar := NewMenubar(BarMenu{Title: "File", Items: Labels("Open")})
	w, _, run = backStage(t, bar)
	w.Input(input.KeyPress{Key: input.KeyF10, Time: time.Now()})
	run(1)
	if !bar.armed || w.Offscreen().BackLeaves() {
		t.Fatalf("the keyboard is on the bar: %v, and Back leaves: %v", bar.armed, w.Offscreen().BackLeaves())
	}
	back(w)
	run(1)
	if bar.armed || w.Offscreen().AppLeft() != 0 || !w.Offscreen().BackLeaves() {
		t.Fatalf("after Back the keyboard is on the bar: %v", bar.armed)
	}

	a := NewAddressBar()
	w, g, run = backStage(t, a)
	a.SetPath("/home", nil, g.ui)
	a.Edit(g.ui)
	run(2)
	if !a.Editing() || w.Offscreen().BackLeaves() {
		t.Fatalf("the address is edited: %v, and Back leaves: %v", a.Editing(), w.Offscreen().BackLeaves())
	}
	back(w)
	run(2)
	if a.Editing() || w.Offscreen().AppLeft() != 0 || !w.Offscreen().BackLeaves() {
		t.Fatalf("after Back the address is edited: %v, and Back leaves: %v", a.Editing(), w.Offscreen().BackLeaves())
	}

	in := NewChordInput("Press a chord")
	dismissed := 0
	in.OnDismiss = func(*gunim.UI) gunim.Intent { dismissed++; return nil }
	w, g, run = backStage(t, in)
	g.ui.Focus(in)
	run(1)
	if w.Offscreen().BackLeaves() {
		t.Fatal("while a chord is listened for Back leaves")
	}
	back(w)
	if dismissed != 1 || w.Offscreen().AppLeft() != 0 {
		t.Fatalf("after Back the chord was dismissed %d times", dismissed)
	}
	run(1)
	if !w.Offscreen().BackLeaves() {
		t.Fatal("with the chord done Back stays")
	}
}

// Each node says it takes Escape exactly where it does.
func TestANodeSaysItTakesEscapeWhereItDoes(t *testing.T) {
	f := NewTextField()
	f.Clearable = true
	w, g, run := backStage(t, f)
	g.ui.Focus(f)
	for _, text := range []string{"", "x", ""} {
		f.SetText(text, g.ui)
		run(1)
		says := f.TakesEscape(g.ui)
		if did := f.Handle(input.KeyPress{Key: input.KeyEscape, Time: time.Now()}, g.ui); did != says {
			t.Errorf("with %q it says %v, and does %v", text, says, did)
		}
	}
	// Text being composed keeps Escape for the keyboard that composes it.
	f.SetText("x", g.ui)
	w.Input(input.Composing{Text: "y"})
	run(1)
	if f.TakesEscape(g.ui) {
		t.Error("a field with text being composed says it takes Escape")
	}
}
