package filemanager

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/widget"
)

func registerProps(w *gunim.Window) {
	gunim.RegisterView(w, "props", newPropsDialog, nil)
	gunim.RegisterPatch(w, "props", func(d *propsDialog, c PropsCounted, u *gunim.UI) { d.counted(c, u) })
}

// propsDialog is the Properties dialog: what the items are, where, how
// big, and when they changed, with the attributes that can change.
type propsDialog struct {
	*widget.Dialog
	form             *widget.Form
	size, holds, err *widget.Label
	readOnly, hidden *widget.Checkbox
}

// propsBody is the form the dialog shows, and what Tab moves through.
type propsBody struct {
	*widget.Flex
	focus []gunim.Node
}

// Focusables is what Tab moves through in the dialog's body.
func (b *propsBody) Focusables() []gunim.Node { return b.focus }

func newPropsDialog(s Props) *propsDialog {
	d := &propsDialog{Dialog: widget.NewDialog(s.Title), form: widget.NewForm()}
	value := func(v string) *widget.Label {
		l := widget.NewLabel(v)
		l.Selectable = true
		return l
	}
	d.form.Add("Name", value(s.Name))
	d.form.Add("Type", value(s.Type))
	d.form.Add("Location", value(s.Location))
	d.size = value(s.Size)
	d.form.Add("Size", d.size)
	if s.Counting || s.Holds != "" {
		d.holds = value(s.Holds)
		d.form.Add("Holds", d.holds)
	}
	for _, t := range []struct{ label, v string }{{"Created", s.Created}, {"Modified", s.Modified}, {"Accessed", s.Accessed}} {
		if t.v != "" {
			d.form.Add(t.label, value(t.v))
		}
	}
	d.err = widget.NewLabel("")
	d.err.Color = ErrorInk
	body := &propsBody{}
	if s.Attrs {
		d.readOnly, d.hidden = widget.NewCheckbox("Read-only"), widget.NewCheckbox("Hidden")
		d.readOnly.On, d.hidden.On = s.ReadOnly, s.Hidden
		d.form.Add("Attributes", widget.Row(d.readOnly, d.hidden))
		body.focus = []gunim.Node{d.readOnly, d.hidden}
		d.SetButtons("OK", "Cancel")
		d.OnAccept = func() gunim.Intent {
			return PropsApplied{Token: s.Token, ReadOnly: d.readOnly.On, Hidden: d.hidden.On}
		}
	} else {
		d.SetButtons("Close", "")
		d.Accept = DialogClosed{}
	}
	d.Dismiss = DialogClosed{}
	body.Flex = widget.Column(d.form, d.err)
	body.Cross = widget.CrossStretch
	d.Body = body
	d.show(s.Size, s.Holds, s.Counting, s.Err)
	return d
}

// counted shows how much the items hold, so far or in all.
func (d *propsDialog) counted(c PropsCounted, u *gunim.UI) {
	d.show(c.Size, c.Holds, c.Counting, c.Err)
	u.Invalidate()
}

func (d *propsDialog) show(size, holds string, counting bool, err string) {
	more := ""
	if counting {
		more = " and counting…"
	}
	if size == "" && counting {
		size = "Counting…"
		more = ""
	}
	d.size.SetText(size + more)
	if d.holds != nil {
		d.holds.SetText(holds)
	}
	d.err.SetText(err)
}
