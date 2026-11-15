//go:build windows && (amd64 || arm64)

package desktop

import (
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/marrasen/gunim/access"
)

func TestTheNewRolesHaveTheirControlTypes(t *testing.T) {
	for r, want := range map[access.Role]int32{
		access.RoleStatus: ctText, access.RoleAlert: ctText, access.RoleRadioButton: ctRadioButton,
	} {
		if got := controlType(&access.Node{Info: access.Info{Role: r}}); got != want {
			t.Errorf("%v is control type %d, want %d", r, got, want)
		}
	}
}

func TestARadioButtonIsSelectedWhenChecked(t *testing.T) {
	on := &access.Node{Info: access.Info{Role: access.RoleRadioButton, State: access.StateCheckable | access.StateChecked}}
	off := &access.Node{Info: access.Info{Role: access.RoleRadioButton, State: access.StateCheckable}}
	box := &access.Node{Info: access.Info{Role: access.RoleCheckbox, State: access.StateCheckable | access.StateChecked}}
	if !isSelected(on) || isSelected(off) {
		t.Errorf("a chosen radio button reads selected %v, and one not chosen %v", isSelected(on), isSelected(off))
	}
	if isSelected(box) {
		t.Error("a ticked checkbox reads as selected")
	}
}

// queued returns a window whose events queue up untouched, and a count of what is queued.
func queued() (uw *uiaWindow, count func() int) {
	uw = &uiaWindow{events: make(chan func(), 64), stop: make(chan struct{}), speak: make(chan struct{}, 1),
		liveAt: map[uint64]time.Time{}, livePending: map[uint64]bool{}}
	return uw, func() int {
		n := 0
		for {
			select {
			case <-uw.events:
				n++
			default:
				return n
			}
		}
	}
}

func TestChoosingARadioButtonRaisesItsSelection(t *testing.T) {
	uw, count := queued()
	was := &access.Node{ID: 7, Info: access.Info{Role: access.RoleRadioButton, State: access.StateCheckable}}
	is := &access.Node{ID: 7, Info: access.Info{Role: access.RoleRadioButton, State: access.StateCheckable | access.StateChecked}}
	uw.diff(was, is, false)
	if n := count(); n != 2 {
		t.Fatalf("choosing a radio button raised %d events, want IsSelected's change and ElementSelected", n)
	}
	uw.diff(is, was, false)
	if n := count(); n != 1 {
		t.Fatalf("leaving a radio button raised %d events, want IsSelected's change alone", n)
	}
}

func TestAStatusThatChangesIsReadAgain(t *testing.T) {
	uw, count := queued()
	was := &access.Node{ID: 3, Info: access.Info{Role: access.RoleStatus, Name: "Syncing 3 files"}}
	is := &access.Node{ID: 3, Info: access.Info{Role: access.RoleStatus, Name: "You're up to date"}}
	uw.diff(was, is, false)
	if n := count(); n != 2 {
		t.Fatalf("a status's new words raised %d events, want the name's change and a live region's", n)
	}
	uw.diff(is, is, false)
	if n := count(); n != 0 {
		t.Fatalf("a status that stayed the same raised %d events", n)
	}
	label := &access.Node{ID: 4, Info: access.Info{Role: access.RoleLabel, Name: "a"}}
	label2 := &access.Node{ID: 4, Info: access.Info{Role: access.RoleLabel, Name: "b"}}
	uw.diff(label, label2, false)
	if n := count(); n != 1 {
		t.Fatalf("a label's new words raised %d events, want the name's change alone", n)
	}
}

// A slider held disabled or read only reads as one whose value takes no change; one that turns does not.
func TestAHeldSliderIsReadOnly(t *testing.T) {
	for _, c := range []struct {
		state access.State
		want  bool
	}{{0, false}, {access.StateDisabled, true}, {access.StateReadOnly, true}} {
		n := &access.Node{Info: access.Info{Role: access.RoleSlider, State: c.state, Range: &access.Range{Max: 1}}}
		if got := rangeReadOnly(n); got != c.want {
			t.Errorf("a slider of state %v reads read only %v, want %v", c.state, got, c.want)
		}
	}
}

// rangeReadOnlySlot is where IRangeValueProvider's get_IsReadOnly stands in its method table: after IUnknown's
// three methods, SetValue and get_Value.
const rangeReadOnlySlot = 5

// A slider's IsReadOnly, as UI Automation reads it through the slider's provider, is true while it is disabled or
// read only.
func TestASliderReadsAsReadOnlyThroughItsProvider(t *testing.T) {
	uw, _ := queued()
	for _, c := range []struct {
		state access.State
		want  int32
	}{{0, 0}, {access.StateDisabled, 1}, {access.StateReadOnly, 1}} {
		slider := &access.Node{ID: 2, Info: access.Info{Role: access.RoleSlider, State: c.state,
			Range: &access.Range{Max: 1}}}
		uw.tree.Store(access.NewTree(&access.Node{ID: 1, Info: access.Info{Role: access.RoleWindow},
			Children: []*access.Node{slider}}))
		e := uw.element(2)
		got := int32(-1)
		hr, _, _ := syscall.SyscallN(uiaTables[ifRange][rangeReadOnlySlot],
			uintptr(unsafe.Pointer(&e.ifaces[ifRange])), uintptr(unsafe.Pointer(&got)))
		e.release()
		if hr != hrOK || got != c.want {
			t.Errorf("a slider of state %v reads IsReadOnly %d (result %#x), want %d", c.state, got, hr, c.want)
		}
	}
}

// A control turning disabled or enabled raises IsEnabled's change, and its reason changing raises HelpText's.
func TestAControlGreyingRaisesItsChanges(t *testing.T) {
	uw, count := queued()
	node := func(st access.State, why string) *access.Node {
		return &access.Node{ID: 5, Info: access.Info{Role: access.RoleButton, Name: "Save", State: st, Description: why}}
	}
	open, held := node(0, ""), node(access.StateDisabled, "Works only with the Echo up")
	for _, c := range []struct {
		what   string
		was    *access.Node
		is     *access.Node
		events int
	}{
		{"turning disabled with a reason", open, held, 2},
		{"with its reason changing", held, node(access.StateDisabled, "Go to the song's start"), 1},
		{"turning disabled with no reason", open, node(access.StateDisabled, ""), 1},
		{"turning enabled", node(access.StateDisabled, ""), open, 1},
		{"nothing changing", held, held, 0},
	} {
		uw.diff(c.was, c.is, false)
		if n := count(); n != c.events {
			t.Errorf("a control %s raised %d events, want %d", c.what, n, c.events)
		}
	}
}
