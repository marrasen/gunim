package gunim

import (
	"testing"

	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// Once every keyed number has been given, the numbers that go again
// are ones missing from the parts being published, so no two parts
// published together share one.
func TestKeyedNumbersGoAgainOnlyFromPartsNotShown(t *testing.T) {
	kp := &keyedParts{at: map[int]int{}}
	for key := uint64(1); key <= partsEnd-keyedFrom; key++ {
		kp.number(key)
	}
	// A publish showing the first ten keys, then a key never seen.
	clear(kp.at)
	for key := uint64(1); key <= 10; key++ {
		kp.at[kp.number(key)] = int(key)
	}
	n := kp.number(1 << 40)
	if _, taken := kp.at[n]; taken || n < keyedFrom || n >= partsEnd {
		t.Fatalf("the new key took %d, a number of a part being published", n)
	}
}

// many is a node of a great many parts, which records the part asked
// for.
type many struct {
	parts  int
	pushed int
}

func (m *many) Layout(c Constraints, _ Frame, _ Children) geom.Size { return c.Max }
func (m *many) Paint(*paint.Painter, Frame, geom.Size, Children)    {}

func (m *many) Access() access.Info {
	info := access.Info{Role: access.RoleList, Parts: make([]access.Info, m.parts)}
	for i := range info.Parts {
		info.Parts[i] = access.Info{Role: access.RoleListItem, Actions: []string{access.ActionPress}}
	}
	return info
}

func (m *many) AccessAct(r access.Request, _ *UI) bool {
	m.pushed = r.Part
	return true
}

// A node of more parts than the numbers parts take by their place still
// gives every part an ID of its own, and a request for a part past
// those numbers reaches it.
func TestEveryPartOfAGreatManyHasItsOwnID(t *testing.T) {
	m := &many{parts: keyedFrom + 200, pushed: -1}
	w := newTestWindow()
	w.ui.Insert(w.ui.Root(), m)
	w.Offscreen().ListenForAccess()
	run(w, 2)
	list := w.Offscreen().AccessTree().Root
	for list != nil && list.Role != access.RoleList {
		if len(list.Children) == 0 {
			t.Fatal("no list in the tree")
		}
		list = list.Children[0]
	}
	seen := map[uint64]bool{}
	for _, p := range list.Children {
		if seen[p.ID] {
			t.Fatalf("two parts share the ID %x", p.ID)
		}
		seen[p.ID] = true
	}
	if len(seen) != m.parts {
		t.Fatalf("%d parts have IDs, want %d", len(seen), m.parts)
	}
	want := keyedFrom + 100
	w.Input(access.Request{ID: list.Children[want].ID, Action: access.ActionPress})
	run(w, 1)
	if m.pushed != want {
		t.Fatalf("a press on part %d reached part %d", want, m.pushed)
	}
}
