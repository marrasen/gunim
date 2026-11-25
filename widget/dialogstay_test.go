package widget

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

// stageDialog mounts d, with the keyboard, in an offscreen window of 1000 by 600, runs it till it stands, and returns
// the window, its UI and a function that runs frames.
func stageDialog(t *testing.T, d *Dialog) (*gunim.Window, *gunim.UI, func(int)) {
	t.Helper()
	w := gunimtest.New(t, geom.Sz(1000, 600), nil)
	w.PaintWhenLooked(false)
	gunim.RegisterView(w, "stay", func(struct{}) gunim.Node { return d }, nil)
	c := w.Client()
	if err := c.Mount(gunim.Root, "stay", "stay", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Focus("stay"); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(1)
	u := viewUI(t, w, "stay", run)
	run(90)
	return w, u, run
}

// sentNow is what the app was sent till now.
func sentNow(w *gunim.Window) []any {
	var out []any
	for {
		select {
		case env := <-w.Client().Intents():
			out = append(out, env.Intent)
		default:
			return out
		}
	}
}

// With StayOpen, OK and Enter both accept and leave the dialog open; a Check
// still stands in the way, and Escape and Close close it.
func TestADialogThatStaysOpenAcceptsAndStays(t *testing.T) {
	d := NewDialog("Share")
	field := NewTextField()
	d.Body = NewForm().Add("Name", field)
	d.StayOpen = true
	d.OnAccept, d.OnDismiss = Sends("create"), Sends("gone")
	wrong := "Name it."
	d.Check = func() string { return wrong }
	w, u, run := stageDialog(t, d)
	enter := input.KeyPress{Key: input.KeyEnter}
	w.Input(enter)
	run(1)
	if got := sentNow(w); len(got) != 0 || d.problem.Text != wrong {
		t.Fatalf("with something in the way, Enter sent %v, and the dialog says %q", got, d.problem.Text)
	}
	wrong = ""
	w.Input(enter)
	// A held Enter acts once.
	w.Input(input.KeyPress{Key: input.KeyEnter, Repeat: true})
	run(60)
	if got := sentNow(w); len(got) != 1 || got[0] != "create" {
		t.Fatalf("a held Enter sent %v, want one create", got)
	}
	r, _ := u.Bounds(d.OK())
	click(w, r.Center().X, r.Center().Y)
	run(60)
	if got := sentNow(w); len(got) != 1 || got[0] != "create" {
		t.Fatalf("a click on OK sent %v, want one create", got)
	}
	if u.Presence(d) != gunim.Present || d.in.Value() != 1 {
		t.Fatalf("accepted, the dialog is %v, %v in", u.Presence(d), d.in.Value())
	}
	// Told otherwise by its last step, it closes.
	d.OnAccept = func(*gunim.UI) gunim.Intent { d.StayOpen = false; return "done" }
	w.Input(enter)
	if u.Presence(d) != gunim.Exiting {
		t.Fatal("with StayOpen off, OK left the dialog open")
	}
	if got := sentNow(w); len(got) != 1 || got[0] != "done" {
		t.Fatalf("the last step sent %v", got)
	}
}

// OK leaves a dialog and comes back on every frame in step with the others:
// Cancel keeps its gap from it and ends where it stood, nothing overlaps and
// nothing jumps, also turned round on the way.
func TestADialogsOKLeavesAndComesBackAsTheOthersGlide(t *testing.T) {
	d := NewDialog("Share")
	d.Body = NewLabel("What to share")
	d.SetButtons("Create link", "Close")
	more := d.AddAction("More", func(*gunim.UI) gunim.Intent { return nil })
	w, u, run := stageDialog(t, d)
	bounds := func(b *Button) geom.Rect { r, _ := u.Bounds(b); return r }
	okAt, cancelAt := bounds(d.ok), bounds(d.cancel)
	gap := okAt.Min.X - cancelAt.Max.X
	width := okAt.Size().W
	do := func(shown bool) {
		d.SetButtonShown(d.OK(), shown, u)
		if slices.Contains(d.Buttons(), gunim.Node(d.ok)) != shown {
			t.Fatalf("shown %v, the buttons are %v", shown, d.Buttons())
		}
	}
	step := func(when string, frames int) {
		t.Helper()
		was := bounds(d.cancel)
		for i := range frames {
			run(1)
			at := bounds(d.cancel)
			s := d.slots[d.ok].shown()
			// The room OK leaves, and its gap, is what Cancel has moved by.
			if want := okAt.Max.X - (width+gap)*s; abs32(at.Max.X-want) > 0.1 {
				t.Fatalf("%s, frame %d: with OK %v in, Cancel ends at %v, want %v", when, i, s, at.Max.X, want)
			}
			if d := bounds(d.cancel).Min.X - bounds(more).Max.X; abs32(d-gap) > 0.1 {
				t.Fatalf("%s, frame %d: More is %v from Cancel, want %v", when, i, d, gap)
			}
			if d := abs32(at.Min.X - was.Min.X); d > 35 {
				t.Fatalf("%s, frame %d: Cancel jumped %v", when, i, d)
			}
			was = at
		}
	}
	do(false)
	run(1)
	if s := d.slots[d.ok].shown(); s <= 0.5 || s >= 1 {
		t.Fatalf("a frame after it was taken away, OK is %v in", s)
	}
	step("leaving", 6)
	do(true)
	step("turned round", 5)
	do(false)
	step("leaving again", 90)
	if at := bounds(d.cancel); abs32(at.Max.X-okAt.Max.X) > 0.01 {
		t.Fatalf("with OK gone, Cancel ends at %v, want where OK did, %v", at.Max.X, okAt.Max.X)
	}
	if _, drawn := u.Bounds(d.ok); drawn {
		t.Fatal("OK is drawn, gone")
	}
	do(true)
	step("coming", 120)
	if at := bounds(d.ok); at != okAt || bounds(d.cancel) != cancelAt {
		t.Fatalf("back, OK is at %v and Cancel at %v, want %v and %v", at, bounds(d.cancel), okAt, cancelAt)
	}
	// Closing, the dialog takes its buttons along as they are.
	w.Input(input.KeyPress{Key: input.KeyEscape})
	for i := range 10 {
		run(1)
		if s := d.slots[d.ok].in.Value(); s != 1 {
			t.Fatalf("frame %d of the dialog closing, OK is %v in", i, s)
		}
	}
}

// Taken away by its own press, OK takes no second one, its keyboard goes to
// the dialog, and Enter closes it from there as Cancel does.
func TestADialogsOKThatLeavesHandsOverTheKeyboard(t *testing.T) {
	d := NewDialog("Share")
	d.Body = NewLabel("What to share")
	d.StayOpen = true
	d.OnAccept = func(u *gunim.UI) gunim.Intent { d.SetButtonShown(d.OK(), false, u); return "create" }
	d.OnDismiss = Sends("close")
	w, u, run := stageDialog(t, d)
	u.Focus(d.OK())
	run(1)
	w.Input(input.KeyPress{Key: input.KeyEnter})
	if u.Focused() != gunim.Node(d) {
		t.Fatalf("with OK gone, the keyboard is on %T", u.Focused())
	}
	// Still held, the Enter that took it away closes nothing.
	w.Input(input.KeyPress{Key: input.KeyEnter, Repeat: true})
	run(2)
	// Where it stood, a press on it as it goes reaches nothing.
	r, _ := u.Bounds(d.OK())
	click(w, r.Center().X, r.Center().Y)
	run(60)
	if got := sentNow(w); len(got) != 1 || got[0] != "create" || u.Presence(d) != gunim.Present {
		t.Fatalf("OK's press, a held Enter and a press on it as it went sent %v", got)
	}
	// Tab and the arrows go by it.
	w.Input(input.KeyPress{Key: input.KeyTab})
	w.Input(input.KeyPress{Key: input.KeyTab})
	if u.Focused() != gunim.Node(d.Cancel()) {
		t.Fatalf("Tab went to %T", u.Focused())
	}
	u.Focus(nil)
	u.Focus(d)
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(1)
	if got := sentNow(w); len(got) != 1 || got[0] != "close" || u.Presence(d) == gunim.Present {
		t.Fatalf("with OK gone, Enter sent %v", got)
	}
}

// A button taken away before the dialog is laid out is not there from the
// first frame, and comes as it is shown.
func TestADialogOpensWithoutAButtonTakenAwayBefore(t *testing.T) {
	d := NewDialog("Links")
	d.SetButtons("Create", "Close")
	d.OnDismiss = Sends("close")
	d.SetButtonShown(d.OK(), false, nil)
	w, u, run := stageDialog(t, d)
	if _, drawn := u.Bounds(d.OK()); drawn || len(d.Buttons()) != 1 {
		t.Fatalf("taken away, OK is drawn %v, of the buttons %v", drawn, d.Buttons())
	}
	was, _ := u.Bounds(d.Cancel())
	d.SetButtonShown(d.OK(), true, u)
	run(2)
	if s := d.slots[d.ok].shown(); s <= 0 || s >= 1 {
		t.Fatalf("two frames after it was shown, OK is %v in", s)
	}
	run(90)
	ok, _ := u.Bounds(d.OK())
	if abs32(ok.Max.X-was.Max.X) > 0.01 {
		t.Fatalf("come, OK ends at %v, want where Cancel did, %v", ok.Max.X, was.Max.X)
	}
	// All of them away, Tab and Enter find nothing, and Escape closes.
	d.SetButtonShown(d.OK(), false, u)
	d.SetButtonShown(d.Cancel(), false, u)
	run(60)
	w.Input(input.KeyPress{Key: input.KeyTab})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(1)
	if got := sentNow(w); len(got) != 0 || u.Presence(d) != gunim.Present {
		t.Fatalf("with no button, Tab and Enter sent %v", got)
	}
	w.Input(input.KeyPress{Key: input.KeyEscape})
	if got := sentNow(w); len(got) != 1 {
		t.Fatalf("Escape sent %v", got)
	}
}

// Enter does nothing while OK is disabled, as a press on it does.
func TestEnterDoesNothingWhileADialogsOKIsDisabled(t *testing.T) {
	d := NewDialog("Share")
	d.Body = NewForm().Add("Name", NewTextField())
	d.OnAccept, d.OnDismiss = Sends("ok"), Sends("cancel")
	d.OK().Disabled = true
	w, u, run := stageDialog(t, d)
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(1)
	if got := sentNow(w); len(got) != 0 || u.Presence(d) != gunim.Present {
		t.Fatalf("Enter sent %v with OK disabled", got)
	}
}

// A button named anew while the dialog is open glides to its new width, and
// the others along with it.
func TestADialogsButtonNamedAnewGlidesToItsWidth(t *testing.T) {
	d := NewDialog("Share")
	d.SetButtons("Go", "Close")
	_, u, run := stageDialog(t, d)
	old, _ := u.Bounds(d.OK())
	d.SetButtons("Create another link", "Close")
	was := old.Min.X
	for i := range 90 {
		run(1)
		ok, _ := u.Bounds(d.OK())
		cancel, _ := u.Bounds(d.Cancel())
		if ok.Max.X != old.Max.X || abs32(ok.Min.X-cancel.Max.X-DialogGap.Default()) > 0.1 {
			t.Fatalf("frame %d: OK spans %v, Cancel %v", i, ok, cancel)
		}
		if d := abs32(ok.Min.X - was); d > 40 {
			t.Fatalf("frame %d: OK's edge jumped %v", i, d)
		}
		was = ok.Min.X
	}
	if was >= old.Min.X-20 {
		t.Fatalf("named anew, OK starts at %v, and at %v before", was, old.Min.X)
	}
}

// With animation off, a button is there or not on the next frame.
func TestADialogsButtonsAreThereAtOnceWithAnimationOff(t *testing.T) {
	t.Cleanup(func() { anim.SetScale(1) })
	d := NewDialog("Share")
	_, u, run := stageDialog(t, d)
	was, _ := u.Bounds(d.OK())
	anim.SetScale(0)
	d.SetButtonShown(d.OK(), false, u)
	run(1)
	if c, _ := u.Bounds(d.Cancel()); abs32(c.Max.X-was.Max.X) > 0.01 {
		t.Fatalf("with animation off, Cancel ends at %v, want %v", c.Max.X, was.Max.X)
	}
	d.SetButtonShown(d.OK(), true, u)
	run(1)
	if r, _ := u.Bounds(d.OK()); r != was {
		t.Fatalf("with animation off, OK came back to %v, want %v", r, was)
	}
}
