package calendar

import (
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
)

// opened is what the tests' OnOpen sends.
type opened struct{ id string }

// tree publishes w to a listening screen reader, and returns its root after a frame or two.
func tree(t *testing.T, w *gunim.Window, run func(int)) *access.Node {
	t.Helper()
	w.Offscreen().ListenForAccess()
	run(2)
	tr := w.Offscreen().AccessTree()
	if tr == nil {
		t.Fatal("nothing published to a listening screen reader")
	}
	return tr.Root
}

// first returns the first node under n of role, or nil.
func first(n *access.Node, role access.Role) *access.Node {
	if n.Role == role {
		return n
	}
	for _, k := range n.Children {
		if f := first(k, role); f != nil {
			return f
		}
	}
	return nil
}

// The week and the month read as lists of their events, named by their days; pressing an event opens it.
func TestTheViewsReadAsTheirEvents(t *testing.T) {
	review := Event{ID: "a", Title: "Review", Start: monday.Add(34 * time.Hour), End: monday.Add(35 * time.Hour)}
	week := newWeek()
	week.OnOpen = func(id string, _ geom.Rect, _ *gunim.UI) gunim.Intent { return opened{id} }
	month := NewMonth(monday)
	month.events = []Event{review}
	month.OnOpen = week.OnOpen
	for name, n := range map[string]gunim.Node{"week": week, "month": month} {
		w, run, sent := stage(t, n)
		list := first(tree(t, w, run), access.RoleList)
		if list == nil || list.Name == "" || len(list.Children) != 1 {
			t.Fatalf("%s: the view reads as %+v, want a named list of one event", name, list)
		}
		item := list.Children[0]
		if !strings.Contains(item.Name, "Review") || !strings.Contains(item.Name, "10:00") {
			t.Fatalf("%s: the event reads as %q", name, item.Name)
		}
		w.Input(access.Request{ID: item.ID, Action: access.ActionPress})
		run(2)
		if got := sent(); len(got) != 1 || got[0] != (opened{"a"}) {
			t.Fatalf("%s: pressing the event sent %v, want it opened", name, got)
		}
	}
}

// A small month reads as a table of its days, the day marked selected and active; pressing a day picks it.
func TestASmallMonthReadsAsItsDays(t *testing.T) {
	m := NewMiniMonth(monday)
	m.OnPick = func(day time.Time, _ *gunim.UI) gunim.Intent { return picked{day} }
	w, run, sent := stage(t, &frame{child: m, size: geom.Sz(300, 300)})
	table := first(tree(t, w, run), access.RoleTable)
	if table == nil || table.Name != monday.Format("January 2006") {
		t.Fatalf("the small month reads as %+v", table)
	}
	var days []*access.Node
	for _, k := range table.Children {
		if k.Role == access.RoleCell {
			days = append(days, k)
		}
	}
	if len(days) != 42 {
		t.Fatalf("the small month has %d days, want 42", len(days))
	}
	var marked *access.Node
	for _, d := range days {
		if d.State.Has(access.StateSelected) {
			marked = d
		}
	}
	if marked == nil || marked.Name != monday.Format("Monday 2 January 2006") {
		t.Fatalf("the day marked reads as %+v", marked)
	}
	w.Input(access.Request{ID: days[20].ID, Action: access.ActionPress})
	run(2)
	if got := sent(); len(got) != 1 || got[0] != (picked{m.day(20)}) {
		t.Fatalf("pressing day 20 sent %v", got)
	}
}

// A date field reads as a drop-down named by its label and holding its day; a time field says it opens a list.
func TestTheFieldsSayWhatTheyHold(t *testing.T) {
	d := NewDateField(monday)
	d.Label = "Starts"
	if info := d.Access(); info.Role != access.RoleComboBox || info.Name != "Starts" ||
		info.Value != monday.Format("Monday 2 January 2006") || !info.State.Has(access.StateHasPopup) {
		t.Fatalf("the date field reads as %+v", info)
	}
	d.Disabled = true
	if !d.Access().State.Has(access.StateDisabled) {
		t.Fatal("the disabled date field reads as enabled")
	}
	tf := NewTimeField(9*time.Hour + 30*time.Minute)
	if info := tf.Access(); info.Role != access.RoleTextField || info.Value != "09:30" ||
		!info.State.Has(access.StateHasPopup) {
		t.Fatalf("the time field reads as %+v", info)
	}
}
