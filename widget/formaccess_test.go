package widget

import (
	"testing"

	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
)

// TestAFormsLabelsNameItsFields: for a screen reader, past a node that only arranges, and only where the field says no
// name of its own. A placeholder gives way, and describes.
func TestAFormsLabelsNameItsFields(t *testing.T) {
	host, port, pass, notes, code := NewTextField(), NewNumberField(1, 65535), NewTextField(), NewTextArea(), NewCodeEditor()
	host.Placeholder, pass.Secret = "example.com", true
	notes.Placeholder, notes.Disabled, notes.Why = "Anything", true, "Not now"
	code.Label = "Start line"
	remember, shared := NewCheckbox("Remember"), NewCheckbox("Share")
	tipped := NewDropdown(Labels("Low", "High"))
	tipped.Label = "Effort"
	form := NewForm().Add("Host", host).Add("Port", NewPad(port)).Add("Password", pass).Add("Notes", notes).Add("Line", code).
		Add("Then", Row(remember, shared)).Add("How hard", tipped).Add("", NewTextField())
	w, run := stage(t, &frame{child: form, size: geom.Sz(500, 560)})
	w.Offscreen().ListenForAccess()
	run(2)
	type said struct{ name, description string }
	var got []said
	var walk func(n *access.Node)
	walk = func(n *access.Node) {
		if n.Role != access.RoleLabel && n.Role != access.RoleWindow && n.Name+n.Description != "" || n.Role == access.RoleTextField {
			got = append(got, said{n.Name, n.Description})
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(w.Offscreen().AccessTree().Root)
	want := []said{{"Host", "example.com"}, {"Port", ""}, {"Password", ""}, {"Show text", ""}, {"Notes", "Not now"}, {"Start line", ""},
		{"Remember", ""}, {"Share", ""}, {"Effort", ""}, {"", ""}}
	if len(got) != len(want) {
		t.Fatalf("a screen reader hears %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("a screen reader hears %q, want %q", got[i], want[i])
		}
	}
}
