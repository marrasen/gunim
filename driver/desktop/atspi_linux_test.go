//go:build linux

package desktop

import (
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

// TestScreenReadersReadAndPressThroughATSPI publishes a window with a
// button, and asks for it over the accessibility bus as a screen
// reader does: the window's role and name, its child, and pressing it.
func TestScreenReadersReadAndPressThroughATSPI(t *testing.T) {
	if display == nil {
		t.Skip("no display")
	}
	a := startATSPI()
	if a == nil {
		t.Skip("no accessibility bus")
	}
	defer func() { _ = a.conn.Close() }()
	dw, err := display.NewWindow(driver.Options{Title: "gunim access", Size: geom.Sz(200, 100)})
	if err != nil {
		t.Fatal(err)
	}
	w, isDesktop := dw.(*Window)
	if !isDesktop {
		t.Fatalf("NewWindow returned a %T", dw)
	}
	defer func() { _ = w.Close() }()
	aw := a.add(w, false)
	defer a.remove(aw)
	button := &access.Node{
		Info:   access.Info{Role: access.RoleButton, Name: "Save", Actions: []string{access.ActionPress, "Save a copy"}},
		ID:     2,
		Bounds: geom.Rc(10, 10, 80, 30),
	}
	a.publish(aw, access.NewTree(&access.Node{
		Info: access.Info{Role: access.RoleWindow, Name: "gunim access"}, ID: 1,
		Children: []*access.Node{button},
	}))

	var addr string
	session, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = session.Close() }()
	if err = session.Object("org.a11y.Bus", "/org/a11y/bus").Call("org.a11y.Bus.GetAddress", 0).Store(&addr); err != nil {
		t.Fatal(err)
	}
	reader, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()

	win := reader.Object(a.name, aw.path(0))
	var role uint32
	if err = win.Call(ifAccessible+".GetRole", 0).Store(&role); err != nil || role != 23 {
		t.Fatalf("the window's role is %d (%v), want 23, a frame", role, err)
	}
	name, err := win.GetProperty(ifAccessible + ".Name")
	if err != nil || name.Value() != "gunim access" {
		t.Fatalf("the window's name is %v (%v)", name, err)
	}
	var kids []ref
	if err = win.Call(ifAccessible+".GetChildren", 0).Store(&kids); err != nil || len(kids) != 1 {
		t.Fatalf("the window's children are %v (%v), want the button", kids, err)
	}
	btn := reader.Object(a.name, kids[0].Path)
	// An action of the app's own is offered by its name, beside the press.
	var second string
	if err = btn.Call(ifAction+".GetName", 0, int32(1)).Store(&second); err != nil || second != "Save a copy" {
		t.Fatalf("the button's second action is %q (%v), want \"Save a copy\"", second, err)
	}
	// Each action reaches the window by its name: the press, then the
	// app's own.
	for i, want := range []string{access.ActionPress, "Save a copy"} {
		var ok bool
		if err = btn.Call(ifAction+".DoAction", 0, int32(i)).Store(&ok); err != nil || !ok {
			t.Fatalf("doing action %d: %v, %v", i, ok, err)
		}
		waitRequest(t, w, 2, want)
	}
}

// waitRequest waits for the request a screen reader made of node id with action to reach window w. The window's
// other input, such as a redraw, may come first.
func waitRequest(t *testing.T, w *Window, id uint64, action string) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-w.Input():
			r, isReq := ev.(access.Request)
			if !isReq {
				continue
			}
			if r.ID != id || r.Action != action {
				t.Fatalf("the window got %#v, want %q of node %d", r, action, id)
			}
			return
		case <-timeout:
			t.Fatalf("%q never reached the window", action)
		}
	}
}

// A node's description reaches a screen reader through AT-SPI as the Description property, without a bus.
func TestTheDescriptionReachesATSPI(t *testing.T) {
	o := &object{node: &access.Node{ID: 2, Info: access.Info{Role: access.RoleButton, Name: "Save",
		Description: "Works only with the Echo up", State: access.StateDisabled}}}
	if v, ok := o.property(ifAccessible, "Description"); !ok || v != "Works only with the Echo up" {
		t.Fatalf("AT-SPI reads the description as %v (%v), want the node's", v, ok)
	}
}
