package widget

import (
	"image/color"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// near reports whether two colours differ by at most one in each channel.
func nearColor(a, b color.NRGBA) bool {
	d := func(x, y uint8) bool { return x-y <= 1 || y-x <= 1 }
	return d(a.R, b.R) && d(a.G, b.G) && d(a.B, b.B) && d(a.A, b.A)
}

func TestHSVRoundTrips(t *testing.T) {
	for _, c := range []color.NRGBA{
		{R: 0xff, A: 0xff}, {G: 0xff, A: 0xff}, {B: 0xff, A: 0xff}, {R: 0x5e, G: 0x9c, B: 0xff, A: 0x80},
		{R: 0x12, G: 0x34, B: 0x56, A: 0xff}, {R: 0x80, G: 0x80, B: 0x80, A: 0xff}, {A: 0xff}, {R: 0xff, G: 0xff, B: 0xff},
	} {
		h, s, v := colorHSV(c)
		if got := hsvColor(h, s, v, float32(c.A)/255); !nearColor(got, c) {
			t.Errorf("%v goes round to %v", c, got)
		}
	}
}

// pickerStage mounts a picker of c and returns it, the window, a way to run frames, and the colours OnChange and
// OnCommit heard.
func pickerStage(t *testing.T, c color.NRGBA) (p *ColorPicker, w *gunim.Window, run func(int), changed, committed *[]color.NRGBA) {
	t.Helper()
	p = NewColorPicker(c)
	changed, committed = &[]color.NRGBA{}, &[]color.NRGBA{}
	p.OnChange = func(c color.NRGBA, _ *gunim.UI) gunim.Intent { *changed = append(*changed, c); return nil }
	p.OnCommit = func(c color.NRGBA, _ *gunim.UI) gunim.Intent { *committed = append(*committed, c); return nil }
	w, run = stage(t, &frame{child: p, size: geom.Sz(200, 300)})
	run(1)
	return p, w, run, changed, committed
}

func TestAColorPickerPicksByPointer(t *testing.T) {
	p, w, run, changed, committed := pickerStage(t, color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff})

	// The square's top right is the hue at full strength: red, as the grey kept hue 0.
	click(w, 199.9, 0.1)
	run(30)
	if got := p.Value(); !nearColor(got, color.NRGBA{R: 0xff, A: 0xff}) {
		t.Fatalf("a click at the square's top right picks %v, want red", got)
	}
	if len(*changed) != 1 || len(*committed) != 1 {
		t.Fatalf("a click told OnChange %d times and OnCommit %d, want once each", len(*changed), len(*committed))
	}
	if p.hex.Text() != "#ff0000" {
		t.Fatalf("the hex field says %q", p.hex.Text())
	}

	// A drag down the square darkens it on every step, and commits once, as it is let go.
	w.Input(input.PointerDown{Pos: geom.Pt(199, 1), Clicks: 1, Time: time.Now()})
	for y := float32(20); y <= 100; y += 20 {
		w.Input(input.PointerMove{Pos: geom.Pt(199, y), Time: time.Now()})
		run(1)
	}
	w.Input(input.PointerUp{Pos: geom.Pt(199, 100), Time: time.Now()})
	run(30)
	if len(*changed) != 7 || len(*committed) != 2 {
		t.Fatalf("a press and five steps of a drag told OnChange %d times and OnCommit %d in all, want 7 and 2", len(*changed), len(*committed))
	}
	if got := p.Value(); got.R < 0x7e || got.R > 0x81 || got.G > 1 {
		t.Fatalf("half way down the square the colour is %v, want a half bright red", got)
	}

	// The middle of the hue strip is cyan.
	click(w, 100, 200+10+7)
	run(30)
	if h, _, _ := colorHSV(p.Value()); h < 0.49 || h > 0.51 {
		t.Fatalf("the middle of the hue strip gives hue %v, want a half", h)
	}
	// The start of the strip of opacity is clear.
	click(w, 0, 200+10+14+10+7)
	run(30)
	if p.Value().A != 0 {
		t.Fatalf("the start of the opacity strip gives alpha %d", p.Value().A)
	}

	// The old half of the swatch brings the first colour back.
	click(w, 10, 200+10+14+10+14+10+10)
	run(30)
	if got := p.Value(); got != (color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff}) {
		t.Fatalf("the old swatch brings back %v", got)
	}
	if last := (*committed)[len(*committed)-1]; last != p.Value() {
		t.Fatalf("bringing the old colour back commits %v", last)
	}
}

func TestAColorPickerWorksFromTheKeyboard(t *testing.T) {
	p, w, run, _, committed := pickerStage(t, color.NRGBA{R: 0xff, A: 0xff})
	u := stageUI(t, w, run)

	u.Focus(p.plane)
	key(w, run, input.KeyDown, input.ModShift)
	run(30)
	if _, _, v := colorHSV(p.Value()); v < 0.89 || v > 0.91 {
		t.Fatalf("Shift+Down on the square gives brightness %v, want 0.9", v)
	}
	if len(*committed) != 1 {
		t.Fatalf("a key committed %d times", len(*committed))
	}
	// The knob glides there.
	if got := p.sv.Value().Y; got != 0.9 && !p.sv.Active() {
		t.Fatalf("the knob is at %v and still", got)
	}

	u.Focus(p.hueBar)
	key(w, run, input.KeyEnd, 0)
	if p.h != 1 {
		t.Fatalf("End on the hue strip puts the hue at %v", p.h)
	}
	key(w, run, input.KeyLeft, input.ModShift)
	if p.h < 0.89 || p.h > 0.91 {
		t.Fatalf("Shift+Left from the end puts the hue at %v", p.h)
	}

	u.Focus(p.alpBar)
	key(w, run, input.KeyHome, 0)
	if p.Value().A != 0 {
		t.Fatalf("Home on the opacity strip leaves alpha %d", p.Value().A)
	}

	// The hex field takes #rgb and commits on Enter.
	u.Focus(p.hex)
	p.hex.SetText("", u)
	w.Input(input.TextInput{Text: "#0f0"})
	run(1)
	if got := p.Value(); got != (color.NRGBA{G: 0xff, A: 0xff}) {
		t.Fatalf("typing #0f0 picks %v", got)
	}
	before := len(*committed)
	key(w, run, input.KeyEnter, 0)
	if len(*committed) != before+1 {
		t.Fatal("Enter in the hex field commits nothing")
	}
	// Text that is no colour is put back once Enter is pressed.
	p.hex.SetText("#12", u)
	key(w, run, input.KeyEnter, 0)
	if p.hex.Text() != "#00ff00" {
		t.Fatalf("after bad text and Enter the field says %q", p.hex.Text())
	}

	// The old swatch answers Space.
	u.Focus(p.swatch)
	key(w, run, input.KeySpace, 0)
	if p.Value() != (color.NRGBA{R: 0xff, A: 0xff}) {
		t.Fatalf("Space on the swatch brings back %v", p.Value())
	}
}

func TestAnOpaqueColorPickerLeavesOutOpacity(t *testing.T) {
	p := NewColorPicker(color.NRGBA{R: 1, G: 2, B: 3, A: 0x40})
	p.Opaque = true
	if p.Value().A != 0xff {
		t.Fatalf("an opaque picker gives alpha %d", p.Value().A)
	}
	if p.alpBar.Focusable() {
		t.Fatal("an opaque picker's opacity strip takes the keyboard")
	}
	w, run := stage(t, &frame{child: p, size: geom.Sz(200, 300)})
	run(1)
	b, ok := stageUI(t, w, run).Bounds(p.swatch)
	if !ok || b.Min.Y != 200+10+14+10 {
		t.Fatalf("the swatch is at %v, want right under the hue strip", b)
	}
}

func TestAColorPickerSpeaks(t *testing.T) {
	p := NewColorPicker(color.NRGBA{R: 0xff, A: 0x80})
	if a := p.hueBar.Access(); a.Role != access.RoleSlider || a.Name != "Hue" || a.Range.Max != 360 {
		t.Fatalf("the hue strip says %+v", a)
	}
	if a := p.alpBar.Access(); a.Name != "Opacity" || a.Range.Value != 50 {
		t.Fatalf("the opacity strip says %+v", a)
	}
	if a := p.plane.Access(); a.Value != "saturation 100%, brightness 100%" {
		t.Fatalf("the square says %q", a.Value)
	}
	if a := p.Access(); a.Value != "#ff000080" {
		t.Fatalf("the picker says %q", a.Value)
	}
}

func TestAColorPickerPaintsInsideItsBox(t *testing.T) {
	p, _, _, _, _ := pickerStage(t, color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0x80})
	for _, part := range []gunim.Node{p.plane, p.hueBar, p.alpBar, p.swatch} {
		var box geom.Size
		switch part {
		case p.plane:
			box = geom.Sz(200, 200)
		case p.swatch:
			box = geom.Sz(72, 36)
		default:
			box = geom.Sz(200, 14)
		}
		// The knobs reach half their size past a strip's ends.
		if s := spills(painted(part, box), box, 12); len(s) > 0 {
			t.Errorf("%T paints outside its box: %v", part, s)
		}
	}
}

func TestAColorButtonOpensItsPicker(t *testing.T) {
	b := NewColorButton(color.NRGBA{B: 0xff, A: 0xff})
	b.Label = "Accent"
	var heard []color.NRGBA
	b.OnChange = func(c color.NRGBA, _ *gunim.UI) gunim.Intent { heard = append(heard, c); return nil }
	w, run := stage(t, &frame{child: b, size: geom.Sz(54, 36)})
	u := stageUI(t, w, run)

	u.Focus(b)
	key(w, run, input.KeyEnter, 0)
	run(10)
	if !b.IsOpen() || b.Picker() == nil {
		t.Fatal("Enter opens no picker")
	}
	if b.Access().State&access.StateExpanded == 0 {
		t.Fatal("the button does not say it is expanded")
	}
	p := b.Picker()
	if u.Focused() != gunim.Node(p.plane) {
		t.Fatalf("the keyboard is on %T, want the picker's square", u.Focused())
	}
	// Tab keeps to the picker.
	for range 6 {
		key(w, run, input.KeyTab, 0)
		if !u.HasFocus(p) {
			t.Fatalf("Tab took the keyboard out of the picker, to %T", u.Focused())
		}
	}
	u.Focus(p.plane)
	key(w, run, input.KeyLeft, input.ModShift)
	if len(heard) != 1 || b.Value() != heard[0] || b.Value() == (color.NRGBA{B: 0xff, A: 0xff}) {
		t.Fatalf("a key in the picker gives the button %v, and OnChange heard %v", b.Value(), heard)
	}
	key(w, run, input.KeyEscape, 0)
	run(30)
	if b.IsOpen() {
		t.Fatal("Escape leaves the picker open")
	}
	if u.Focused() != gunim.Node(b) {
		t.Fatalf("after the picker closed the keyboard is on %T, want the button", u.Focused())
	}

	// A click opens it, and a second click on the button closes it.
	click(w, 20, 18)
	run(10)
	if !b.IsOpen() {
		t.Fatal("a click opens no picker")
	}
	click(w, 20, 18)
	run(30)
	if b.IsOpen() {
		t.Fatal("a second click leaves the picker open")
	}

	// Escape in the hex field closes it too.
	click(w, 20, 18)
	run(10)
	u.Focus(b.Picker().hex)
	key(w, run, input.KeyEscape, 0)
	run(30)
	if b.IsOpen() {
		t.Fatal("Escape in the hex field leaves the picker open")
	}
}

func TestAColorButtonPaintsItsColour(t *testing.T) {
	c := color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}
	b := NewColorButton(color.NRGBA{})
	b.SetValue(c, nil)
	found := false
	for _, r := range rrects(painted(b, geom.Sz(54, 36))) {
		if r.Fill.Solid == c {
			found = true
		}
	}
	if !found {
		t.Fatal("the button does not paint its colour")
	}
}
