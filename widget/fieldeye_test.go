package widget

import (
	"strings"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// dotted reports whether the field draws dots in place of its text.
func dotted(t *testing.T, f *TextField) bool {
	t.Helper()
	dot := f.line.face.Shape("•", f.line.size).Glyphs[0].ID
	n := 0
	for _, pc := range f.line.pieces {
		for _, g := range pc.run.Glyphs {
			n++
			if g.ID != dot {
				return false
			}
		}
	}
	if n == 0 {
		t.Fatal("the field draws no glyphs")
	}
	return true
}

// clickEye clicks the middle of the field's eye.
func (ty *typer) clickEye() {
	ty.t.Helper()
	if ty.field.eye == nil || !ty.field.eyeOn {
		ty.t.Fatal("the field shows no eye")
	}
	s := ty.field.eye.size
	click(ty.w, ty.field.eyeX+s.W/2, 18)
	ty.run(1)
}

func newSecretTyper(t *testing.T, set func(*TextField)) *typer {
	t.Helper()
	ty := newTyperWith(t, func(f *TextField) {
		f.Secret = true
		set(f)
	})
	ty.typeText("hunter2")
	ty.run(2)
	return ty
}

func TestTheEyeShowsAndHidesTheTextAndKeepsItOffTheClipboard(t *testing.T) {
	ty := newSecretTyper(t, func(*TextField) {})
	if !dotted(t, ty.field) || ty.field.Shown() {
		t.Fatal("a secret field shows its text before the eye is pressed")
	}
	_ = ty.w.Offscreen().SetClipboard("before")
	copyAll := func() {
		ty.key(input.KeyA, input.ModControl)
		ty.key(input.KeyC, input.ModControl)
		ty.key(input.KeyX, input.ModControl)
		ty.key(input.KeyEnd, 0)
	}

	// Select "t" in the middle of the text: the anchor at 3, the caret at 4.
	u := stageUI(t, ty.w, ty.run)
	ty.key(input.KeyHome, 0)
	for range 3 {
		ty.key(input.KeyRight, 0)
	}
	ty.key(input.KeyRight, input.ModShift)
	ty.clickEye()
	if dotted(t, ty.field) || !ty.field.Shown() {
		t.Fatal("pressed, the eye left the text hidden")
	}
	if !u.HasFocus(ty.field) {
		t.Fatal("a click on the eye took the keyboard out of the field")
	}
	if ty.field.anchor != 3 || ty.field.caret != 4 {
		t.Fatalf("a click on the eye moved the selection to %d..%d, want 3..4", ty.field.anchor, ty.field.caret)
	}
	copyAll()
	ty.want("hunter2", 7)

	ty.clickEye()
	if !dotted(t, ty.field) || ty.field.Shown() {
		t.Fatal("pressed again, the eye left the text shown")
	}
	copyAll()
	if c, _ := ty.w.Offscreen().Clipboard(); c != "before" {
		t.Fatalf("the secret field put %q on the clipboard", c)
	}
	ty.want("hunter2", 7)
}

func TestTabPassesTheEyeAndAltF8FlipsIt(t *testing.T) {
	field := NewTextField()
	field.Secret = true
	other := NewButton("Other")
	w, run := stage(t, &frame{child: Column(field, other), size: geom.Sz(300, 100)})
	u := stageUI(t, w, run)
	u.Focus(field)
	w.Input(input.TextInput{Text: "hunter2"})
	run(2)
	for i := range 4 {
		w.Input(input.KeyPress{Key: input.KeyF8, Mods: input.ModAlt})
		run(1)
		if want := i%2 == 0; field.Shown() != want || dotted(t, field) == want {
			t.Fatalf("after Alt+F8 number %d the text shows: %v, want %v", i+1, field.Shown(), want)
		}
	}
	if field.Text() != "hunter2" {
		t.Fatalf("Alt+F8 changed the text to %q", field.Text())
	}
	w.Input(input.KeyPress{Key: input.KeyTab})
	run(1)
	if u.Focused() != gunim.Node(other) {
		t.Fatalf("Tab from the field went to %T, want the next control past the eye", u.Focused())
	}
}

func TestAScreenReaderHearsTheEyesNameAndWhetherItIsPressed(t *testing.T) {
	ty := newSecretTyper(t, func(f *TextField) { f.EyeLabel = "Show password" })
	eye := ty.field.eye
	info := eye.Access()
	if info.Role != access.RoleButton || info.Name != "Show password" || !info.State.Has(access.StateCheckable) ||
		info.State.Has(access.StateChecked) {
		t.Fatalf("the eye says %+v, want a toggle button named by EyeLabel, not pressed", info)
	}
	if v := ty.field.Access().Value; v != "" {
		t.Fatalf("the hidden field reads out %q", v)
	}
	if !eye.AccessAct(access.Request{Action: access.ActionPress}, nil) {
		t.Fatal("a screen reader cannot press the eye")
	}
	ty.run(1)
	if !ty.field.Shown() || !eye.Access().State.Has(access.StateChecked) {
		t.Fatal("pressed by a screen reader, the eye did not show the text, or does not say it is pressed")
	}
	if v := ty.field.Access().Value; v != "hunter2" {
		t.Fatalf("with its text shown the field reads out %q", v)
	}
	// The tooltip says what a press does now: hide the text.
	shownTip := eye.Tooltip
	ty.field.EyeHideLabel = "Hide password"
	ty.run(1)
	if !strings.Contains(eye.Tooltip, ty.field.EyeHideLabel) {
		t.Fatalf("while the text shows the eye's tooltip is %q, want it to name EyeHideLabel", eye.Tooltip)
	}
	ty.field.EyeHideLabel = ""
	eye.AccessAct(access.Request{Action: access.ActionPress}, nil)
	ty.run(1)
	if eye.Tooltip == shownTip || !strings.Contains(eye.Tooltip, ty.field.EyeLabel) {
		t.Fatalf("while the text hides the eye's tooltip is %q, the same as while it shows, or without EyeLabel", eye.Tooltip)
	}
	ty.field.EyeLabel = ""
	if eye.Access().Name == "" {
		t.Fatal("the eye has no name without an EyeLabel")
	}
}

func TestTheTextHidesAgainWhenTheKeyboardLeavesOrTheWindowHides(t *testing.T) {
	var other *Button
	setup := func(t *testing.T, keep bool) (*TextField, *gunim.Window, func(int), *gunim.UI) {
		field := NewTextField()
		other = NewButton("Other")
		field.Secret, field.KeepShown = true, keep
		w, run := stage(t, &frame{child: Column(field, other), size: geom.Sz(300, 100)})
		u := stageUI(t, w, run)
		u.Focus(field)
		w.Input(input.TextInput{Text: "hunter2"})
		run(1)
		return field, w, run, u
	}
	for _, keep := range []bool{false, true} {
		field, _, run, u := setup(t, keep)
		field.SetShown(true, u)
		run(1)
		u.Focus(other)
		run(1)
		if field.Shown() != keep {
			t.Fatalf("KeepShown %v: after the keyboard left, the text shows: %v", keep, field.Shown())
		}
	}
	for _, keep := range []bool{false, true} {
		field, w, run, u := setup(t, keep)
		field.SetShown(true, u)
		run(1)
		w.Input(driver.WindowShown{Shown: false})
		w.Input(driver.WindowShown{Shown: true})
		run(1)
		if field.Shown() != keep {
			t.Fatalf("KeepShown %v: after the window hid, the text shows: %v", keep, field.Shown())
		}
	}
}

func TestTheTextHidesAgainWhenAnotherWindowTakesTheKeyboard(t *testing.T) {
	for _, keep := range []bool{false, true} {
		field := NewTextField()
		field.Secret, field.KeepShown = true, keep
		w, run := stage(t, &frame{child: field, size: geom.Sz(300, 100)})
		u := stageUI(t, w, run)
		u.Focus(field)
		w.Input(input.TextInput{Text: "hunter2"})
		run(1)
		field.SetShown(true, u)
		run(1)
		w.Input(driver.WindowFocus{Focused: false})
		run(5)
		if field.Shown() != keep || dotted(t, field) == keep {
			t.Fatalf("KeepShown %v: after another window took the keyboard, the text shows: %v", keep, field.Shown())
		}
		w.Input(driver.WindowFocus{Focused: true})
		run(1)
		if field.Shown() != keep {
			t.Fatalf("KeepShown %v: after the window got the keyboard back, the text shows: %v", keep, field.Shown())
		}
	}
}

func TestAFieldWithSkipEyeHasNoEye(t *testing.T) {
	ty := newSecretTyper(t, func(f *TextField) { f.SkipEye = true })
	u := stageUI(t, ty.w, ty.run)
	if ty.field.eyeOn || len(iconsDrawn(t, ty.w)) != 0 {
		t.Fatal("a field with SkipEye shows an eye")
	}
	ty.key(input.KeyTab, 0)
	if u.Focused() != gunim.Node(ty.field) {
		t.Fatalf("Tab went to %T, want round to the field alone", u.Focused())
	}
	ty.key(input.KeyF8, input.ModAlt)
	if !dotted(t, ty.field) || ty.field.Shown() {
		t.Fatal("the field shows its text")
	}
}

func TestAFieldMadeSecretLaterGrowsAnEyeAndLosesItAgain(t *testing.T) {
	ty := newTyper(t)
	ty.typeText("hunter2")
	ty.field.Secret = true
	ty.run(3)
	if !ty.field.eyeOn {
		t.Fatal("a field made secret after it was laid out shows no eye")
	}
	ty.clickEye()
	if !ty.field.Shown() || dotted(t, ty.field) {
		t.Fatal("the eye of a field made secret later shows nothing")
	}
	ty.field.Secret = false
	ty.run(2)
	if ty.field.eyeOn || ty.field.Shown() || ty.field.eye.Access().Role == access.RoleButton {
		t.Fatal("a field no longer secret still shows its eye")
	}
	ty.field.Secret = true
	ty.run(2)
	if !dotted(t, ty.field) {
		t.Fatal("made secret again, the field shows its text")
	}
}

func TestTheEyeStaysInsideTheFieldWithTheX(t *testing.T) {
	ty := newSecretTyper(t, func(f *TextField) { f.Clearable = true })
	ty.run(30)
	icons := iconsDrawn(t, ty.w)
	if len(icons) != 2 {
		t.Fatalf("the field drew %d icons, want the X and the eye", len(icons))
	}
	x := maskOps(ty.w.Offscreen())[0].Rect
	s := ty.field.eye.size
	eye := geom.Rc(ty.field.eyeX, (36-s.H)/2, s.W, s.H)
	if x.Max.X > eye.Min.X || eye.Max.X > 300 || eye.Min.Y < 0 || eye.Max.Y > 36 {
		t.Fatalf("the X is at %v and the eye at %v, want both inside the field, the X first", x, eye)
	}
	if spill := spills(painted(ty.field, geom.Sz(300, 36)), geom.Sz(300, 36), 3); len(spill) > 0 {
		t.Fatalf("the field paints outside its box: %v", spill)
	}
	// The X still clears, and leaves the eye alone.
	click(ty.w, x.Center().X, x.Center().Y)
	ty.run(1)
	ty.want("", 0)
	if ty.field.Shown() {
		t.Fatal("the X pressed the eye")
	}
}

func TestAClickOnTheEyeWhileAnotherControlHasTheKeyboardStillHidesTheTextLater(t *testing.T) {
	field := NewTextField()
	field.Secret = true
	other := NewButton("Other")
	w, run := stage(t, &frame{child: Column(field, other), size: geom.Sz(300, 100)})
	u := stageUI(t, w, run)
	u.Focus(field)
	w.Input(input.TextInput{Text: "hunter2"})
	run(2)
	u.Focus(other)
	run(1)

	click(w, field.eyeX+field.eye.size.W/2, 18)
	run(2)
	if !field.Shown() {
		t.Fatal("a click on the eye left the text hidden")
	}
	if !u.HasFocus(field) {
		t.Fatalf("a click on the eye left the keyboard on %T, want the field", u.Focused())
	}
	// The text hides once the keyboard leaves, and when the window hides.
	u.Focus(other)
	run(2)
	if field.Shown() {
		t.Fatal("the text still shows after the keyboard left the field")
	}
	click(w, field.eyeX+field.eye.size.W/2, 18)
	run(2)
	w.Input(driver.WindowShown{Shown: false})
	w.Input(driver.WindowShown{Shown: true})
	run(2)
	if field.Shown() {
		t.Fatal("the text still shows after the window hid")
	}
}

func TestTextShownFromCodeHidesAgainWhileTheFieldLacksTheKeyboard(t *testing.T) {
	field := NewTextField()
	field.Secret = true
	other := NewButton("Other")
	w, run := stage(t, &frame{child: Column(field, other), size: geom.Sz(300, 100)})
	u := stageUI(t, w, run)
	u.Focus(other)
	field.SetText("hunter2", u)
	run(2)
	field.SetShown(true, u)
	run(2)
	if field.Shown() {
		t.Fatal("text shown while another control has the keyboard stays shown")
	}
}

func TestANarrowFieldDropsPartsRatherThanDrawOutside(t *testing.T) {
	for _, width := range []float32{30, 40, 60, 76, 90, 110, 300} {
		field := NewTextField()
		field.Secret, field.Clearable, field.Icon = true, true, icon.Search
		w, run := stage(t, &frame{child: field, size: geom.Sz(width, 36)})
		u := stageUI(t, w, run)
		u.Focus(field)
		w.Input(input.TextInput{Text: "hunter2"})
		run(30)
		box := geom.Sz(width, 36)
		if spill := spills(w.Offscreen().Ops(), box, 3); len(spill) > 0 {
			t.Fatalf("at %v px the field paints outside its box: %v", width, spill)
		}
		// No two parts overlap, and the text keeps room between them.
		ms := maskOps(w.Offscreen())
		at := func(m *paint.MaskOp) geom.Rect {
			return geom.Rect{Min: m.Transform.Apply(m.Rect.Min), Max: m.Transform.Apply(m.Rect.Max)}
		}
		for i := range ms {
			for j := i + 1; j < len(ms); j++ {
				if a, b := at(ms[i]), at(ms[j]); a.Min.X < b.Max.X && b.Min.X < a.Max.X {
					t.Fatalf("at %v px two parts overlap: %v and %v", width, a, b)
				}
			}
		}
		if field.lead+field.trail > width {
			t.Fatalf("at %v px the parts take %v px, more than the field", width, field.lead+field.trail)
		}
		if width >= 110 && len(ms) != 3 {
			t.Fatalf("at %v px the field drew %d parts, want the icon, the X and the eye", width, len(ms))
		}
	}
}
