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
