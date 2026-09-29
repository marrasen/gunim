package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

type signIn struct {
	Host, Password string
	Remember       bool
}

// newSignInStage mounts a dialog with a form of a host, a password and
// a checkbox, and gives it the keyboard.
func newSignInStage(t *testing.T) (*gunim.Window, *Form, [3]gunim.Node, func(int)) {
	t.Helper()
	host, pass, remember := NewTextField(), NewTextField(), NewCheckbox("Remember the password")
	pass.Secret = true
	form := NewForm().Add("Host", host).Add("Password", pass).Add("", remember)
	d := NewDialog("Connect to a server")
	d.Body = form
	d.SetButtons("Connect", "Cancel")
	d.OnAccept = func() gunim.Intent {
		return signIn{Host: host.Text(), Password: pass.Text(), Remember: remember.On}
	}
	w := gunimtest.New(t, geom.Sz(800, 600), nil)
	gunim.RegisterView(w, "d", func(struct{}) gunim.Node { return d }, nil)
	if err := w.Client().Mount(gunim.Root, "d", "d", nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Client().Focus("d"); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(30)
	return w, form, [3]gunim.Node{host, pass, remember}, run
}

func TestAFormDialogTakesTypingAndConfirmsWithEnter(t *testing.T) {
	w, _, fields, run := newSignInStage(t)
	w.Input(input.TextInput{Text: "example.com"})
	w.Input(input.KeyPress{Key: input.KeyTab})
	w.Input(input.TextInput{Text: "hunter2"})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(30)
	got := sent(w)
	want := signIn{Host: "example.com", Password: "hunter2"}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("intents %v, want %v", got, want)
	}
	if host, ok := fields[0].(*TextField); !ok || host.Text() != "example.com" {
		t.Fatal("the host field lost its text")
	}
}

func TestASecretFieldShowsDotsAndKeepsOffTheClipboard(t *testing.T) {
	w, _, fields, run := newSignInStage(t)
	pass, ok := fields[1].(*TextField)
	if !ok {
		t.Fatal("the second field is no text field")
	}
	w.Input(input.KeyPress{Key: input.KeyTab})
	w.Input(input.TextInput{Text: "hunter2"})
	run(1)
	if got := string([]rune(pass.shaped.s)); got != "•••••••" {
		t.Fatalf("the field shows %q, want seven dots", got)
	}
	_ = w.Offscreen().SetClipboard("before")
	w.Input(input.KeyPress{Key: input.KeyA, Mods: input.ModControl})
	w.Input(input.KeyPress{Key: input.KeyC, Mods: input.ModControl})
	run(1)
	if c, _ := w.Offscreen().Clipboard(); c != "before" {
		t.Fatalf("copying a secret field put %q on the clipboard", c)
	}
}

func TestTabGoesRoundTheFieldsAndButtons(t *testing.T) {
	w, _, fields, run := newSignInStage(t)
	// Host, password, checkbox, Cancel, Connect, and round to the host.
	for range 5 {
		w.Input(input.KeyPress{Key: input.KeyTab})
	}
	w.Input(input.TextInput{Text: "x"})
	run(1)
	if host, ok := fields[0].(*TextField); !ok || host.Text() != "x" {
		t.Fatal("after Tab went round, typing missed the host field")
	}
}

func TestAFormLinesItsLabelsUpAgainstItsFields(t *testing.T) {
	_, form, _, _ := newSignInStage(t)
	// Every label ends at one line, a gap short of where every field
	// starts.
	first := form.rows[0]
	for i, r := range form.rows {
		if r.labelRight != first.labelRight || r.fieldLeft != first.fieldLeft {
			t.Fatalf("row %d: label ends at %v and field starts at %v, want %v and %v",
				i, r.labelRight, r.fieldLeft, first.labelRight, first.fieldLeft)
		}
	}
	if gap := first.fieldLeft - first.labelRight; gap != FormGap.Default() {
		t.Fatalf("the gap between labels and fields is %v, want %v", gap, FormGap.Default())
	}
}

func TestADialogWithAProblemStaysOpenAndShakes(t *testing.T) {
	host := NewTextField()
	d := NewDialog("Connect to a server")
	d.Body = NewForm().Add("Host", host)
	d.OnAccept = func() gunim.Intent { return signIn{Host: host.Text()} }
	d.Check = func() string {
		if host.Text() == "" {
			return "Say which server."
		}
		return ""
	}
	w := gunimtest.New(t, geom.Sz(800, 600), nil)
	gunim.RegisterView(w, "d", func(struct{}) gunim.Node { return d }, nil)
	if err := w.Client().Mount(gunim.Root, "d", "d", nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Client().Focus("d"); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(30)
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(3)
	if d.problem.Text != "Say which server." {
		t.Fatalf("the dialog says %q", d.problem.Text)
	}
	if d.shake.Value() == 0 {
		t.Fatal("the dialog kept still")
	}
	run(90)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("an empty form sent %v", got)
	}
	if v := d.shake.Value(); v != 0 {
		t.Fatalf("the shake settled at %v, want 0", v)
	}
	w.Input(input.TextInput{Text: "example.com"})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(30)
	if got := sent(w); len(got) != 1 || got[0] != (signIn{Host: "example.com"}) {
		t.Fatalf("intents %v, want the host", got)
	}
}

type answered struct{ What string }

func TestADialogsExtraButtonSendsItsAnswer(t *testing.T) {
	d := NewDialog("Replace notes.txt?")
	d.SetButtons("Replace", "Stop")
	d.Accept = answered{"replace"}
	d.AddButton("Leave It", func() gunim.Intent { return answered{"leave"} })
	w := gunimtest.New(t, geom.Sz(800, 600), nil)
	gunim.RegisterView(w, "d", func(struct{}) gunim.Node { return d }, nil)
	if err := w.Client().Mount(gunim.Root, "d", "d", nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Client().Focus("d"); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(30)
	// Tab goes to the first button, Leave It, then Enter presses it.
	w.Input(input.KeyPress{Key: input.KeyTab})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(30)
	if got := sent(w); len(got) != 1 || got[0] != (answered{"leave"}) {
		t.Fatalf("intents %v, want leave", got)
	}
}

// With DefaultFirst, Tab from the fields reaches OK before an extra
// button, so Tab then Enter does what Enter alone does.
func TestADefaultFirstDialogTabsToOKFirst(t *testing.T) {
	d := NewDialog("Replace notes.txt?")
	d.SetButtons("Leave It", "Stop")
	d.Accept = answered{"leave"}
	d.AddButton("Replace", func() gunim.Intent { return answered{"replace"} })
	d.DefaultFirst = true
	w := gunimtest.New(t, geom.Sz(800, 600), nil)
	gunim.RegisterView(w, "d", func(struct{}) gunim.Node { return d }, nil)
	if err := w.Client().Mount(gunim.Root, "d", "d", nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Client().Focus("d"); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(30)
	w.Input(input.KeyPress{Key: input.KeyTab})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(30)
	if got := sent(w); len(got) != 1 || got[0] != (answered{"leave"}) {
		t.Fatalf("intents %v, want leave", got)
	}
}
