package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

func TestDialogAnimatesInAndOut(t *testing.T) {
	w := gunim.NewOffscreen(geom.Sz(800, 600), nil)
	var d *Dialog
	gunim.RegisterView(w, "confirm", func(title string) *Dialog {
		d = NewDialog(title)
		return d
	}, nil)
	c := w.Client()
	if err := c.Mount(gunim.Root, "confirm", "confirm", "Delete?"); err != nil {
		t.Fatal(err)
	}
	step := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}

	step(3)
	if v := d.in.Value(); v <= 0 {
		t.Fatalf("three frames after mounting, in = %v; the dialog is still invisible", v)
	}
	step(120)
	if v := d.in.Value(); v != 1 {
		t.Fatalf("once settled, in = %v, want 1", v)
	}

	if err := c.Unmount("confirm"); err != nil {
		t.Fatal(err)
	}
	step(3)
	if v := d.in.Value(); v <= 0 || v >= 1 {
		t.Fatalf("three frames into the exit, in = %v, want part way out", v)
	}
	step(120)
	if v := d.in.Value(); v != 0 {
		t.Fatalf("once gone, in = %v, want 0", v)
	}
}

func TestADangerButtonFadesInFromThePlainColours(t *testing.T) {
	w := gunim.NewOffscreen(geom.Sz(800, 600), nil)
	var d *Dialog
	gunim.RegisterView(w, "confirm", func(title string) *Dialog {
		d = NewDialog(title)
		d.Danger = true
		return d
	}, nil)
	if err := w.Client().Mount(gunim.Root, "confirm", "confirm", "Delete?"); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	b := d.ok
	if b.was != ButtonPlain || b.is != ButtonDanger {
		t.Fatalf("the button fades from kind %d to %d, want plain to danger", b.was, b.is)
	}
	last := b.tone.Value()
	if last > 0.2 {
		t.Fatalf("one frame in, the button is %v of the way to red", last)
	}
	for i := range 40 {
		w.Frame(time.Second / 60)
		v := b.tone.Value()
		if v < last || v > 1 {
			t.Fatalf("frame %d: the tone went from %v to %v", i, last, v)
		}
		last = v
	}
	if last != 1 {
		t.Fatalf("settled, the button is %v of the way to red", last)
	}
}

func TestADangerDialogOpensOnCancelSoEnterCancels(t *testing.T) {
	w := gunim.NewOffscreen(geom.Sz(800, 600), nil)
	var d *Dialog
	gunim.RegisterView(w, "confirm", func(title string) *Dialog {
		d = NewDialog(title)
		d.Danger = true
		d.Body = NewForm().Add("Name", NewTextField())
		d.Accept, d.Dismiss = "delete", "keep"
		return d
	}, nil)
	c := w.Client()
	if err := c.Mount(gunim.Root, "confirm", "confirm", "Delete?"); err != nil {
		t.Fatal(err)
	}
	if err := c.Focus("confirm"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		w.Frame(time.Second / 60)
	}
	w.Input(input.KeyPress{Key: input.KeyEnter})
	w.Frame(time.Second / 60)
	select {
	case env := <-c.Intents():
		if env.Intent != "keep" {
			t.Fatalf("Enter on a danger dialog sent %v, want keep", env.Intent)
		}
	case <-time.After(time.Second):
		t.Fatal("Enter on a danger dialog sent nothing")
	}
}

func TestADialogWithNoCancelHasOKAlone(t *testing.T) {
	w := gunim.NewOffscreen(geom.Sz(800, 600), nil)
	var d *Dialog
	gunim.RegisterView(w, "note", func(title string) *Dialog {
		d = NewDialog(title)
		d.Body = NewLabel("Written down.")
		d.SetButtons("Done", "")
		d.Accept = "done"
		return d
	}, nil)
	c := w.Client()
	if err := c.Mount(gunim.Root, "note", "note", "A note"); err != nil {
		t.Fatal(err)
	}
	if err := c.Focus("note"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		w.Frame(time.Second / 60)
	}
	for _, n := range d.Children() {
		if n == d.cancel {
			t.Fatal("with no cancel label, the dialog still has Cancel")
		}
	}
	w.Input(input.KeyPress{Key: input.KeyEnter})
	w.Frame(time.Second / 60)
	select {
	case env := <-c.Intents():
		if env.Intent != "done" {
			t.Fatalf("Enter sent %v, want done", env.Intent)
		}
	case <-time.After(time.Second):
		t.Fatal("Enter sent nothing")
	}
}

func TestAnActionLeavesTheDialogOpen(t *testing.T) {
	w := gunim.NewOffscreen(geom.Sz(800, 600), nil)
	var d *Dialog
	var box *Checkbox
	field := NewTextField()
	gunim.RegisterView(w, "form", func(title string) *Dialog {
		d = NewDialog(title)
		box = NewCheckbox("Show")
		box.OnFlip(func(on bool, _ *gunim.UI) { field.Secret = !on })
		field.Secret = true
		d.Body = NewForm().Add("Password", field).Add("", box)
		d.AddAction("Generate", func(*gunim.UI) { field.SetText("made") })
		return d
	}, nil)
	c := w.Client()
	if err := c.Mount(gunim.Root, "form", "form", "Add"); err != nil {
		t.Fatal(err)
	}
	if err := c.Focus("form"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		w.Frame(time.Second / 60)
	}
	// Tab from the field to the box, and Space ticks it.
	w.Input(input.KeyPress{Key: input.KeyTab})
	w.Input(input.KeyPress{Key: input.KeySpace})
	// Tab on to the action, and Enter presses it.
	w.Input(input.KeyPress{Key: input.KeyTab})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	for range 3 {
		w.Frame(time.Second / 60)
	}
	if field.Secret {
		t.Fatal("ticked, the box left the field hidden")
	}
	if field.Text() != "made" {
		t.Fatalf("after the action, the field holds %q", field.Text())
	}
	select {
	case env := <-c.Intents():
		t.Fatalf("the action closed the dialog, sending %v", env.Intent)
	default:
	}
}

func TestADialogWidensForItsButtons(t *testing.T) {
	w := gunim.NewOffscreen(geom.Sz(1000, 600), nil)
	var d *Dialog
	var u *gunim.UI
	gunim.RegisterView(w, "many", func(title string) *Dialog {
		d = NewDialog(title)
		d.Body = NewLabel("Several things can be done here.")
		d.AddAction("Copy Prompt", func(*gunim.UI) {})
		d.AddAction("Copy Setup", func(*gunim.UI) {})
		d.AddButton("Stop Sharing", func() gunim.Intent { return nil })
		d.SetButtons("Done", "Cancel")
		return d
	}, func(_ *Dialog, _ string, got *gunim.UI) { u = got })
	c := w.Client()
	if err := c.Mount(gunim.Root, "many", "many", "Share", "many"); err != nil {
		t.Fatal(err)
	}
	for range 60 {
		w.Frame(time.Second / 60)
	}
	if err := c.Publish("many", "Share"); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	panel, ok := u.Bounds(d)
	if !ok {
		t.Fatal("the dialog has no bounds")
	}
	inner := d.panel(panel.Size(), gunim.Frame{Theme: u.Theme()}).Add(panel.Min)
	for _, b := range append(d.extra, d.cancel, d.ok) {
		r, ok := u.Bounds(b)
		if !ok {
			t.Fatalf("%s has no bounds", b.Label)
		}
		if r.Min.X < inner.Min.X || r.Max.X > inner.Max.X {
			t.Fatalf("%s spans %v, outside the panel %v", b.Label, r, inner)
		}
	}
}

// A careful dialog opens on Cancel too, and keeps OK its usual colour.
func TestACarefulDialogOpensOnCancelInItsUsualColours(t *testing.T) {
	w := gunim.NewOffscreen(geom.Sz(800, 600), nil)
	var d *Dialog
	gunim.RegisterView(w, "trust", func(title string) *Dialog {
		d = NewDialog(title)
		d.Careful = true
		d.Accept, d.Dismiss = "trust", "leave"
		return d
	}, nil)
	c := w.Client()
	if err := c.Mount(gunim.Root, "trust", "trust", "Trust this key?"); err != nil {
		t.Fatal(err)
	}
	if err := c.Focus("trust"); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		w.Frame(time.Second / 60)
	}
	if d.ok.Kind == ButtonDanger {
		t.Fatal("a careful dialog's OK is red")
	}
	w.Input(input.KeyPress{Key: input.KeyEnter})
	w.Frame(time.Second / 60)
	select {
	case env := <-c.Intents():
		if env.Intent != "leave" {
			t.Fatalf("Enter on a careful dialog sent %v, want leave", env.Intent)
		}
	case <-time.After(time.Second):
		t.Fatal("Enter sent nothing")
	}
}
