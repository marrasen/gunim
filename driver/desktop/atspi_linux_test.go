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
		Info:   access.Info{Role: access.RoleButton, Name: "Save", Actions: []string{access.ActionPress}},
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
	var ok bool
	if err = btn.Call(ifAction+".DoAction", 0, int32(0)).Store(&ok); err != nil || !ok {
		t.Fatalf("pressing the button: %v, %v", ok, err)
	}
	// The window's other input, such as a redraw, may come first.
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-w.Input():
			r, isReq := ev.(access.Request)
			if !isReq {
				continue
			}
			if r.ID != 2 || r.Action != access.ActionPress {
				t.Fatalf("the window got %#v, want a press of the button", r)
			}
			return
		case <-timeout:
			t.Fatal("the press never reached the window")
		}
	}
}
