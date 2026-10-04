package desktop

import "github.com/marrasen/gunim/driver"

// tellUnseen tells the window whether it is out of sight while open:
// covered by other windows, on another desktop, or on a screen that is
// off or locked. It runs on the main thread, and tells only changes.
func (w *Window) tellUnseen() {
	unseen := w.hiddenBy || w.d.screenAway
	if unseen == w.unseen {
		return
	}
	w.unseen = unseen
	w.debugf("out of sight %v", unseen)
	w.in.Push(driver.WindowCovered{Covered: unseen})
}
