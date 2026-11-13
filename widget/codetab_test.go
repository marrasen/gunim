package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// codeForm is a code editor as one field of a form, between a text
// field above and a button below.
type codeForm struct {
	code    *CodeEditor
	field   *TextField
	save    *Button
	w       *gunim.Window
	run     func(int)
	focused func() gunim.Node
}

// codeFormStage shows a codeForm, with the keyboard on the field.
func codeFormStage(t *testing.T, movesOn bool) codeForm {
	t.Helper()
	field, code, save := NewTextField(), NewCodeEditor(), NewButton("Save")
	code.Highlight = nil
	code.MaxRows = 3
	code.TabMovesOn = movesOn
	code.SetText("ls", nil)
	w, run := stage(t, &frame{child: Column(field, code, save), size: geom.Sz(600, 300)})
	focused := focusProbe(t, w, run)
	click(w, 20, 10)
	run(1)
	if f := focused(); f != field {
		t.Fatalf("a click on the field put the keyboard on %T", f)
	}
	return codeForm{code, field, save, w, run, focused}
}

// With TabMovesOn, Tab and Shift+Tab move through the editor as through
// any field, and its focus fades out on every frame as the keyboard
// leaves.
func TestCodeTabMovesOnLeavesTabToFocus(t *testing.T) {
	form := codeFormStage(t, true)
	code, field, save, w, run, focused := form.code, form.field, form.save, form.w, form.run, form.focused
	steps := []struct {
		mods input.Mods
		want gunim.Node
	}{
		{0, code},
		{0, save},
		{input.ModShift, code},
		{input.ModShift, field},
	}
	for i, s := range steps {
		w.Input(input.KeyPress{Key: input.KeyTab, Mods: s.mods})
		run(1)
		if f := focused(); f != s.want {
			t.Fatalf("step %d: Tab put the keyboard on %T, want %T", i, f, s.want)
		}
		if s.want == field {
			// The editor's focus fades out, never rising, frame by frame.
			last := code.focus.Value()
			for range 40 {
				run(1)
				v := code.focus.Value()
				if v > last+1e-6 {
					t.Fatalf("the editor's focus rose from %v to %v as the keyboard left", last, v)
				}
				last = v
			}
			if last > 0.01 {
				t.Fatalf("the editor's focus rests at %v after the keyboard left, want 0", last)
			}
		}
	}
	if got := code.Text(); got != "ls" {
		t.Fatalf("Tab through the editor changed its code to %q", got)
	}
}

// Without TabMovesOn, Tab stays in the editor and indents.
func TestCodeTabIndentsWithoutTabMovesOn(t *testing.T) {
	form := codeFormStage(t, false)
	code, w, run, focused := form.code, form.w, form.run, form.focused
	w.Input(input.KeyPress{Key: input.KeyTab})
	run(1)
	if f := focused(); f != code {
		t.Fatalf("Tab from the field put the keyboard on %T, want the editor", f)
	}
	typeKeys(w, run, input.ModControl, input.KeyHome)
	typeKeys(w, run, 0, input.KeyTab)
	if f := focused(); f != code {
		t.Fatalf("Tab in the editor moved the keyboard to %T", f)
	}
	if got := code.Text(); got != "\tls" {
		t.Fatalf("Tab in the editor made %q, want a tab typed", got)
	}
	typeKeys(w, run, input.ModShift, input.KeyTab)
	if got, f := code.Text(), focused(); got != "ls" || f != code {
		t.Fatalf("Shift+Tab in the editor made %q with the keyboard on %T, want the tab taken back", got, f)
	}
}

// With TabMovesOn, a tab typed or pasted becomes a space.
func TestCodeTabMovesOnTurnsTabsToSpaces(t *testing.T) {
	c, w, run := codeStage(t, "")
	c.TabMovesOn = true
	w.Input(input.TextInput{Text: "a\tb"})
	run(1)
	if got := c.Text(); got != "a b" {
		t.Fatalf("typing a tab made %q", got)
	}
	if err := w.Offscreen().SetClipboard("\n\tc\td"); err != nil {
		t.Fatal(err)
	}
	typeKeys(w, run, input.ModControl, input.KeyV)
	if got := c.Text(); got != "a b\n c d" {
		t.Fatalf("pasting tabs made %q", got)
	}
}

// With TabMovesOn, Enter keeps the line's indent and adds none after an
// opening bracket or a colon, and a closing brace takes none back.
func TestCodeTabMovesOnEnterAddsNoIndent(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"if x {", "if x {\n"},
		{"  case a:", "  case a:\n  "},
		{"\tf(", "\tf(\n\t"},
		{"  [", "  [\n  "},
	} {
		c, w, run := codeStage(t, tc.src)
		c.TabMovesOn = true
		typeKeys(w, run, input.ModControl, input.KeyEnd)
		typeKeys(w, run, 0, input.KeyEnter)
		if got := c.Text(); got != tc.want {
			t.Errorf("Enter after %q made %q, want %q", tc.src, got, tc.want)
		}
	}
	c, w, run := codeStage(t, "\tx")
	c.TabMovesOn = true
	typeKeys(w, run, input.ModControl, input.KeyEnd)
	typeKeys(w, run, 0, input.KeyEnter)
	w.Input(input.TextInput{Text: "}"})
	run(1)
	if got := c.Text(); got != "\tx\n\t}" {
		t.Fatalf("a closing brace made %q, want the indent kept", got)
	}
}
