//go:build windows

package desktop

import (
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/internal/glfw"
)

// SetTray implements [driver.Trayer], on the main thread, and waits for
// the answer.
func (d *Driver) SetTray(t driver.Tray) error {
	var icon *glfw.TrayIcon
	if len(t.Icon) > 0 {
		icon = &glfw.TrayIcon{Images: t.Icon, Tooltip: t.Tooltip, Items: trayItems(t.Items)}
		if t.OnPick != nil {
			pick := t.OnPick
			icon.OnPick = func(id int) { go pick(id) }
		}
		if t.OnClick != nil {
			icon.OnClick = d.trayClick(t.OnClick)
		}
		if t.OnNotifyGone != nil {
			gone := t.OnNotifyGone
			icon.OnNotifyGone = func() { go gone() }
		}
		if click, tagged := t.OnNotifyClick, t.OnNotifyTag; click != nil || tagged != nil {
			// One goroutine for both, so OnNotifyTag runs after OnNotifyClick.
			icon.OnNotifyTag = func(tag string) {
				go func() {
					if click != nil {
						click()
					}
					if tagged != nil {
						tagged(tag)
					}
				}()
			}
		}
	}
	done := make(chan error, 1)
	if !d.post(func() { done <- glfw.SetTrayIcon(icon) }) {
		return errClosed
	}
	return <-done
}

// TrayNotify implements [driver.TrayNotifier], on the main thread.
func (d *Driver) TrayNotify(title, body string) error { return d.TrayNotifyTagged(title, body, "") }

// TrayNotifyTagged implements [driver.TagNotifier], on the main thread. The tag stays in the process, kept for a
// click on the balloon, which names no message: the tag heard is that of the message shown last.
func (d *Driver) TrayNotifyTagged(title, body, tag string) error {
	if err := driver.CheckNotifyTag(tag); err != nil {
		return err
	}
	done := make(chan error, 1)
	if !d.post(func() { done <- glfw.TrayNotify(title, body, tag) }) {
		return errClosed
	}
	return <-done
}

// TrayRect implements [driver.TrayLocator], through Shell_NotifyIconGetRect on the main thread.
func (d *Driver) TrayRect() (geom.Rect, error) {
	var r geom.Rect
	err := d.call(func() error {
		l, t, rt, b, err := glfw.TrayIconRect()
		r = geom.Rect{Min: geom.Pt(float32(l), float32(t)), Max: geom.Pt(float32(rt), float32(b))}
		return err
	})
	return r, err
}

// closeTray takes the icon away, as the application ends, on the main
// thread.
func closeTray() { glfw.CloseTrayIcon() }

// trayItems are a menu's lines as glfw has them.
func trayItems(items []driver.TrayItem) []glfw.TrayMenuItem {
	out := make([]glfw.TrayMenuItem, 0, len(items))
	for _, it := range items {
		out = append(out, glfw.TrayMenuItem{Title: it.Title, ID: it.ID, Items: trayItems(it.Items),
			Checked: it.Checked, Disabled: it.Disabled, Separator: it.Separator, Default: it.Default})
	}
	return out
}
