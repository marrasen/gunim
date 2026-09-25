package widget

import (
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type saved struct{}

// accessStage lays out a button, a checkbox, a slider and two tabs in a
// window a screen reader listens to.
func accessStage(t *testing.T) (*gunim.Window, *Tabs, *Checkbox, func(int)) {
	t.Helper()
	b := NewButton("Save")
	b.On = saved{}
	c := NewCheckbox("Wrap lines")
	s := NewSlider(0, 10)
	s.Snap = 1
	tabs := NewTabs([]string{"General", "Advanced"}, Column(b, c, s), NewLabel("Nothing here yet"))
	w, run := stage(t, &frame{child: tabs, size: geom.Sz(400, 300)})
	w.Offscreen().ListenForAccess()
	run(2)
	return w, tabs, c, run
}

// dump writes a tree as indented lines of role and name.
func dump(n *access.Node, depth int, b *strings.Builder) {
	b.WriteString(strings.Repeat("  ", depth) + n.Role.String())
	if n.Name != "" {
		b.WriteString(" " + n.Name)
	}
	if n.Focused {
		b.WriteString(" (focused)")
	}
	b.WriteString("\n")
	for _, k := range n.Children {
		dump(k, depth+1, b)
	}
}

func find(n *access.Node, role access.Role, name string) *access.Node {
	if n.Role == role && n.Name == name {
		return n
	}
	for _, k := range n.Children {
		if f := find(k, role, name); f != nil {
			return f
		}
	}
	return nil
}

func TestTheTreeSaysWhatIsOnScreen(t *testing.T) {
	w, _, _, _ := accessStage(t)
	tree := w.Offscreen().AccessTree()
	if tree == nil {
		t.Fatal("nothing published to a listening screen reader")
	}
	var b strings.Builder
	dump(tree.Root, 0, &b)
	got := b.String()
	for _, want := range []string{"tab list", "tab General", "tab Advanced", "button Save", "checkbox Wrap lines", "slider"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the tree lacks %q:\n%s", want, got)
		}
	}
	// The second tab's page is out of sight, so out of the tree.
	if strings.Contains(got, "Nothing here yet") {
		t.Fatalf("the hidden page is in the tree:\n%s", got)
	}
	save := find(tree.Root, access.RoleButton, "Save")
	if save.Bounds.Empty() {
		t.Fatal("the button has no place on screen")
	}
}

func TestAScreenReaderCanPressAndFocus(t *testing.T) {
	w, tabs, c, run := accessStage(t)
	tree := w.Offscreen().AccessTree()
	save := find(tree.Root, access.RoleButton, "Save")
	w.Input(access.Request{ID: save.ID, Action: access.ActionPress})
	check := find(tree.Root, access.RoleCheckbox, "Wrap lines")
	w.Input(access.Request{ID: check.ID, Action: access.ActionPress, Focus: true})
	advanced := find(tree.Root, access.RoleTab, "Advanced")
	w.Input(access.Request{ID: advanced.ID, Action: access.ActionPress})
	run(2)
	if got := sent(w); len(got) == 0 || got[0] != (saved{}) {
		t.Fatalf("pressing Save sent %v", got)
	}
	if !c.On {
		t.Fatal("pressing the checkbox left it unchecked")
	}
	if tabs.Selected() != 1 {
		t.Fatal("pressing the Advanced tab left it unchosen")
	}
	// The checkbox took focus, and says so, checked.
	w.Input(input.PointerMove{Pos: geom.Pt(1, 1), Time: time.Now()})
	run(1)
	tree = w.Offscreen().AccessTree()
	if tree.Focus == nil || tree.Focus.ID != check.ID {
		t.Fatalf("focus is on %+v, want the checkbox", tree.Focus)
	}
}
