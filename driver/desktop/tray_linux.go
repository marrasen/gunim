//go:build linux

package desktop

import (
	"fmt"
	"image"
	"os"
	"sync"
	"sync/atomic"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"

	"github.com/marrasen/gunim/driver"
)

// On Linux the tray is the StatusNotifierItem protocol: the application
// puts an item on the session bus, with its icon, tooltip and menu, and
// tells the watcher, which hands it to whatever draws the tray, as
// Cinnamon's, KDE's and most panels do. The menu is com.canonical's
// dbusmenu, which the panel draws and asks about over the bus.

const (
	sniPath      = dbus.ObjectPath("/StatusNotifierItem")
	sniMenuPath  = dbus.ObjectPath("/MenuBar")
	ifSNI        = "org.kde.StatusNotifierItem"
	ifDBusMenu   = "com.canonical.dbusmenu"
	sniWatcher   = "org.kde.StatusNotifierWatcher"
	sniWatcherAt = dbus.ObjectPath("/StatusNotifierWatcher")
)

// tray is the tray icon shown, one for the process, and mu guards it.
var tray struct {
	mu sync.Mutex
	s  *sniTray
}

// trayCount numbers the items put on the bus by this process.
var trayCount atomic.Int32

// sniTray is the application's item on the bus.
type sniTray struct {
	conn  *dbus.Conn
	name  string
	props *prop.Properties

	mu  sync.Mutex
	t   driver.Tray
	rev uint32
	// items are the menu's lines by their dbusmenu id, and kids each
	// id's children, the root's under 0.
	items map[int32]driver.TrayItem
	kids  map[int32][]int32
}

// SetTray implements [driver.Trayer].
func (d *Driver) SetTray(t driver.Tray) error {
	tray.mu.Lock()
	defer tray.mu.Unlock()
	if len(t.Icon) == 0 {
		if tray.s != nil {
			tray.s.close()
			tray.s = nil
		}
		return nil
	}
	if tray.s != nil {
		tray.s.update(t)
		return nil
	}
	s, err := newSNITray(t)
	if err != nil {
		return err
	}
	tray.s = s
	return nil
}

func newSNITray(t driver.Tray) (*sniTray, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", driver.ErrNoTray, err)
	}
	s := &sniTray{conn: conn, name: fmt.Sprintf("org.kde.StatusNotifierItem-%d-%d", os.Getpid(), trayCount.Add(1))}
	s.setMenu(t)
	fail := func(err error) (*sniTray, error) {
		_ = conn.Close()
		return nil, fmt.Errorf("%w: %w", driver.ErrNoTray, err)
	}
	if e := conn.Export(sniItem{s}, sniPath, ifSNI); e != nil {
		return fail(e)
	}
	props, err := prop.Export(conn, sniPath, prop.Map{ifSNI: s.itemProps(t)})
	if err != nil {
		return fail(err)
	}
	s.props = props
	if err := conn.Export(sniMenu{s}, sniMenuPath, ifDBusMenu); err != nil {
		return fail(err)
	}
	if _, err := prop.Export(conn, sniMenuPath, prop.Map{ifDBusMenu: {
		"Version":       {Value: uint32(3)},
		"TextDirection": {Value: "ltr"},
		"Status":        {Value: "normal"},
		"IconThemePath": {Value: []string{}},
	}}); err != nil {
		return fail(err)
	}
	if reply, err := conn.RequestName(s.name, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		return fail(fmt.Errorf("could not take the name %s", s.name))
	}
	if err := conn.Object(sniWatcher, sniWatcherAt).Call(sniWatcher+".RegisterStatusNotifierItem", 0, s.name).Err; err != nil {
		return fail(err)
	}
	return s, nil
}

// itemProps are the item's properties for t.
func (s *sniTray) itemProps(t driver.Tray) map[string]*prop.Prop {
	return map[string]*prop.Prop{
		"Category":   {Value: "ApplicationStatus"},
		"Id":         {Value: "gunim-tray"},
		"Title":      {Value: t.Tooltip},
		"Status":     {Value: "Active"},
		"WindowId":   {Value: int32(0)},
		"IconName":   {Value: ""},
		"IconPixmap": {Value: pixmaps(t.Icon)},
		"ToolTip":    {Value: tooltip(t)},
		"ItemIsMenu": {Value: t.OnClick == nil},
		"Menu":       {Value: sniMenuPath},
	}
}

// update shows t in place of what the item showed.
func (s *sniTray) update(t driver.Tray) {
	s.setMenu(t)
	s.props.SetMust(ifSNI, "Title", t.Tooltip)
	s.props.SetMust(ifSNI, "IconPixmap", pixmaps(t.Icon))
	s.props.SetMust(ifSNI, "ToolTip", tooltip(t))
	s.props.SetMust(ifSNI, "ItemIsMenu", t.OnClick == nil)
	for _, sig := range []string{"NewTitle", "NewIcon", "NewToolTip"} {
		_ = s.conn.Emit(sniPath, ifSNI+"."+sig)
	}
	s.mu.Lock()
	rev := s.rev
	s.mu.Unlock()
	_ = s.conn.Emit(sniMenuPath, ifDBusMenu+".LayoutUpdated", rev, int32(0))
}

// close takes the item off the bus.
func (s *sniTray) close() {
	_, _ = s.conn.ReleaseName(s.name)
	_ = s.conn.Close()
}

// setMenu numbers t's lines for dbusmenu, afresh.
func (s *sniTray) setMenu(t driver.Tray) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.t = t
	s.rev++
	s.items = map[int32]driver.TrayItem{}
	s.kids = map[int32][]int32{}
	next := int32(0)
	var add func(parent int32, items []driver.TrayItem)
	add = func(parent int32, items []driver.TrayItem) {
		for _, it := range items {
			next++
			id := next
			s.items[id] = it
			s.kids[parent] = append(s.kids[parent], id)
			if len(it.Items) > 0 {
				add(id, it.Items)
			}
		}
	}
	add(0, t.Items)
}

// sniPixmap is an icon as the protocol has it: ARGB, big end first.
type sniPixmap struct {
	W, H int32
	Data []byte
}

// pixmaps turns the icon's images into the protocol's.
func pixmaps(icons []image.Image) []sniPixmap {
	out := make([]sniPixmap, 0, len(icons))
	for _, m := range icons {
		b := m.Bounds()
		px := make([]byte, 0, 4*b.Dx()*b.Dy())
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bl, a := m.At(x, y).RGBA()
				// Straight rather than premultiplied, as the protocol
				// wants.
				if a > 0 {
					r, g, bl = r*0xffff/a, g*0xffff/a, bl*0xffff/a
				}
				px = append(px, byte(a>>8), byte(r>>8), byte(g>>8), byte(bl>>8))
			}
		}
		out = append(out, sniPixmap{W: int32(b.Dx()), H: int32(b.Dy()), Data: px})
	}
	return out
}

// sniTooltip is a tooltip as the protocol has it.
type sniTooltip struct {
	IconName string
	Pixmap   []sniPixmap
	Title    string
	Text     string
}

func tooltip(t driver.Tray) sniTooltip {
	return sniTooltip{Pixmap: []sniPixmap{}, Title: t.Tooltip}
}

// sniItem answers the item's methods.
type sniItem struct{ s *sniTray }

// Activate is a click on the icon.
func (i sniItem) Activate(x, y int32) *dbus.Error {
	i.s.mu.Lock()
	click := i.s.t.OnClick
	i.s.mu.Unlock()
	if click != nil {
		go click()
	}
	return nil
}

// SecondaryActivate is a middle click, which does nothing here.
func (i sniItem) SecondaryActivate(x, y int32) *dbus.Error { return nil }

// ContextMenu asks the item for its menu, which the panel draws itself
// from Menu.
func (i sniItem) ContextMenu(x, y int32) *dbus.Error { return nil }

// Scroll is the wheel over the icon, which does nothing here.
func (i sniItem) Scroll(delta int32, orientation string) *dbus.Error { return nil }

// sniMenu answers dbusmenu's methods.
type sniMenu struct{ s *sniTray }

// menuLayout is a line of the menu and those under it, as dbusmenu
// sends them.
type menuLayout struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []dbus.Variant
}

// lineProps are line id's properties, as dbusmenu names them.
func (s *sniTray) lineProps(id int32) map[string]dbus.Variant {
	out := map[string]dbus.Variant{}
	if id == 0 {
		out["children-display"] = dbus.MakeVariant("submenu")
		return out
	}
	it := s.items[id]
	if it.Separator {
		out["type"] = dbus.MakeVariant("separator")
		return out
	}
	out["label"] = dbus.MakeVariant(it.Title)
	out["enabled"] = dbus.MakeVariant(!it.Disabled)
	if len(it.Items) > 0 {
		out["children-display"] = dbus.MakeVariant("submenu")
	}
	if it.Checked {
		out["toggle-type"] = dbus.MakeVariant("checkmark")
		out["toggle-state"] = dbus.MakeVariant(int32(1))
	}
	return out
}

// layout is line id and depth levels under it, all of them for -1.
func (s *sniTray) layout(id, depth int32) menuLayout {
	l := menuLayout{ID: id, Props: s.lineProps(id), Children: []dbus.Variant{}}
	if depth == 0 {
		return l
	}
	for _, k := range s.kids[id] {
		l.Children = append(l.Children, dbus.MakeVariant(s.layout(k, depth-1)))
	}
	return l
}

// GetLayout sends the menu from parentID down.
func (m sniMenu) GetLayout(parentID, depth int32, names []string) (uint32, menuLayout, *dbus.Error) {
	m.s.mu.Lock()
	defer m.s.mu.Unlock()
	return m.s.rev, m.s.layout(parentID, depth), nil
}

// menuProps are a line's properties by its id.
type menuProps struct {
	ID    int32
	Props map[string]dbus.Variant
}

// GetGroupProperties sends the properties of the lines ids.
func (m sniMenu) GetGroupProperties(ids []int32, names []string) ([]menuProps, *dbus.Error) {
	m.s.mu.Lock()
	defer m.s.mu.Unlock()
	out := make([]menuProps, 0, len(ids))
	for _, id := range ids {
		out = append(out, menuProps{ID: id, Props: m.s.lineProps(id)})
	}
	return out, nil
}

// GetProperty sends one property of one line.
func (m sniMenu) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	m.s.mu.Lock()
	defer m.s.mu.Unlock()
	if v, ok := m.s.lineProps(id)[name]; ok {
		return v, nil
	}
	return dbus.MakeVariant(""), nil
}

// Event hears a line clicked.
func (m sniMenu) Event(id int32, event string, data dbus.Variant, at uint32) *dbus.Error {
	if event != "clicked" {
		return nil
	}
	m.s.mu.Lock()
	it, ok := m.s.items[id]
	pick := m.s.t.OnPick
	m.s.mu.Unlock()
	if ok && pick != nil && !it.Disabled && !it.Separator && len(it.Items) == 0 {
		go pick(it.ID)
	}
	return nil
}

// menuEvent is one of the events EventGroup carries.
type menuEvent struct {
	ID    int32
	Event string
	Data  dbus.Variant
	At    uint32
}

// EventGroup hears several events at once.
func (m sniMenu) EventGroup(events []menuEvent) ([]int32, *dbus.Error) {
	for _, e := range events {
		_ = m.Event(e.ID, e.Event, e.Data, e.At)
	}
	return []int32{}, nil
}

// AboutToShow says the menu under id will show; nothing changes.
func (m sniMenu) AboutToShow(id int32) (bool, *dbus.Error) { return false, nil }

// AboutToShowGroup is AboutToShow for several.
func (m sniMenu) AboutToShowGroup(ids []int32) (updates, notFound []int32, err *dbus.Error) {
	return []int32{}, []int32{}, nil
}
