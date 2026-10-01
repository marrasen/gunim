package filemanager

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/widget"
)

func registerDialogs(w *gunim.Window) {
	gunim.RegisterView(w, "clash", newClashDialog, nil)
	gunim.RegisterView(w, "confirm", newConfirmDialog, nil)
	gunim.RegisterView(w, "prompt", newPromptDialog, nil)
	gunim.RegisterView(w, "error", newErrorDialog, nil)
}

// clashBody says what the two items are, and offers to answer the same
// for the rest.
type clashBody struct {
	*widget.Flex
	all *widget.Checkbox
}

// Focusables is what Tab moves through in the dialog's body.
func (b *clashBody) Focusables() []gunim.Node { return []gunim.Node{b.all} }

// newClashDialog asks what to do about an item going where one of its
// name is: replace it, keep both, skip it, or stop.
func newClashDialog(s ClashAsk) *widget.Dialog {
	d := widget.NewDialog("“" + s.Name + "” is already in " + s.Where)
	arriving := widget.NewLabel("Arriving: " + s.New)
	there := widget.NewLabel("Already there: " + s.Old)
	for _, l := range []*widget.Label{arriving, there} {
		l.Size, l.Color = SmallText, Faint
	}
	all := widget.NewCheckbox("Do the same for every clash in this operation")
	col := widget.Column(arriving, there, all)
	col.Cross = widget.CrossStretch
	body := &clashBody{Flex: col, all: all}
	if !s.CanForAll {
		all.Disabled = true
	}
	d.Body = body
	answer := func(c Choice) func() gunim.Intent {
		return func() gunim.Intent { return ClashAnswered{Op: s.Op, Choice: c, All: all.On} }
	}
	d.Dismiss = ClashAnswered{Op: s.Op, Stop: true}
	d.AddButton("Skip", answer(ChoiceSkip))
	if s.SameKind {
		d.AddButton("Keep both", answer(ChoiceKeepBoth))
		d.SetButtons("Replace", "Stop")
		d.OnAccept = answer(ChoiceReplace)
	} else {
		d.SetButtons("Keep both", "Stop")
		d.OnAccept = answer(ChoiceKeepBoth)
	}
	d.Careful = true
	return d
}

// newConfirmDialog asks before something that cannot be taken back.
func newConfirmDialog(s Confirm) *widget.Dialog {
	d := widget.NewDialog(s.Title)
	body := widget.NewLabel(s.Body)
	body.Color = Faint
	d.Body = body
	d.Danger = true
	d.SetButtons(s.OK, "Cancel")
	d.Accept = Confirmed{Token: s.Token, OK: true}
	d.Dismiss = Confirmed{Token: s.Token}
	return d
}

// promptBody is the field a prompt asks in.
type promptBody struct {
	*widget.TextField
}

// Focusables is what Tab moves through in the dialog's body.
func (b *promptBody) Focusables() []gunim.Node { return []gunim.Node{b} }

// newPromptDialog asks for a name, with the name without its extension
// selected, and will not take one an item cannot have.
func newPromptDialog(s Prompt) *widget.Dialog {
	d := widget.NewDialog(s.Title)
	field := &promptBody{TextField: widget.NewTextField()}
	field.SetText(s.Text)
	field.Select(0, s.Stem)
	d.Body = field
	d.SetButtons(s.OK, "Cancel")
	d.Check = func() string {
		if err := checkName(field.Text()); err != nil {
			return err.Error()
		}
		return ""
	}
	d.OnAccept = func() gunim.Intent { return Prompted{Token: s.Token, Text: field.Text(), OK: true} }
	d.Dismiss = Prompted{Token: s.Token}
	return d
}

// newErrorDialog says an operation failed, and why.
func newErrorDialog(s ErrorBox) *widget.Dialog {
	d := widget.NewDialog(s.Title)
	body := widget.NewLabel(s.Body)
	body.Selectable = true
	d.Body = body
	d.SetButtons("OK", "")
	d.Accept, d.Dismiss = DialogClosed{}, DialogClosed{}
	return d
}
