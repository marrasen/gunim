package vst3

import (
	"fmt"
	"unsafe"
)

// The plugin's editor: its view, in a window of the system's that the
// package makes, on the plugins' thread.

type viewRect struct{ left, top, right, bottom int32 }

func (r viewRect) size() (w, h int) { return int(r.right - r.left), int(r.bottom - r.top) }

type editor struct {
	p         *Plugin
	view      uintptr
	frame     uintptr
	win       *window
	resizable bool
}

// The frame a view sits in, which it asks to be resized.

type plugFrame struct {
	object
	e *editor
}

var frameKind = &kind{
	iids: []uid{iidPlugFrame},
	make: func() []uintptr {
		return []uintptr{
			newCallback(func(this, view, rect uintptr) uintptr {
				f := self[plugFrame](this)
				if f == nil || f.e == nil || rect == 0 {
					return resultOf(resultInvalid)
				}
				r := *ptr[viewRect](rect)
				w, h := r.size()
				f.e.win.resize(w, h)
				call(view, mOnSize, uintptr(unsafe.Pointer(&r)))
				return resultOf(resultOK)
			}),
		}
	},
	other: func(this uintptr, id uid) uintptr {
		if id == iidRunLoop {
			return runLoop()
		}
		return 0
	},
}

// HasEditor says whether the plugin has an editor this platform shows.
func (p *Plugin) HasEditor() bool {
	ok := false
	onUI(func() {
		if p.ctrl == 0 {
			return
		}
		v := p.createView()
		if v == 0 {
			return
		}
		ok = true
		release(v)
	})
	return ok
}

// createView makes the controller's editor view, if this platform can
// show it.
func (p *Plugin) createView() uintptr {
	name := []byte("editor\x00")
	v := call(p.ctrl, mCreateView, uintptr(unsafe.Pointer(&name[0])))
	if v == 0 {
		return 0
	}
	pt := []byte(platformType + "\x00")
	if result(call(v, mIsPlatformTypeSupported, uintptr(unsafe.Pointer(&pt[0])))) != resultOK {
		release(v)
		return 0
	}
	return v
}

// OpenEditor shows the plugin's editor, in a window titled title, or
// brings it to the front where it is open.
func (p *Plugin) OpenEditor(title string) error {
	var err error
	onUI(func() {
		if p.ed != nil {
			p.ed.win.raise()
			return
		}
		err = p.attachEditor(title)
	})
	return err
}

func (p *Plugin) attachEditor(title string) error {
	if p.ctrl == 0 {
		return fmt.Errorf("vst3: %s has no editor", p.Class.Name)
	}
	v := p.createView()
	if v == 0 {
		return fmt.Errorf("vst3: %s has no editor here", p.Class.Name)
	}
	var r viewRect
	call(v, mGetSize, uintptr(unsafe.Pointer(&r)))
	w, h := r.size()
	if w <= 0 || h <= 0 {
		w, h = 640, 480
	}
	e := &editor{p: p, view: v, resizable: result(call(v, mCanResize)) == resultOK}
	win, err := newWindow(title, w, h, e)
	if err != nil {
		release(v)
		return err
	}
	e.win = win
	f := &plugFrame{e: e}
	e.frame = keep(f, frameKind, false)
	call(v, mSetFrame, e.frame)
	pt := []byte(platformType + "\x00")
	if r := result(call(v, mAttached, win.handle(), uintptr(unsafe.Pointer(&pt[0])))); r != resultOK {
		call(v, mSetFrame, 0)
		release(v)
		drop(e.frame)
		win.destroy()
		return fmt.Errorf("vst3: %s: its editor would not attach (%d)", p.Class.Name, r)
	}
	win.show()
	p.ed = e
	p.editorOpen.Store(true)
	return nil
}

// resized is the window's own size changed, by the user, to w×h.
func (e *editor) resized(w, h int) {
	if !e.resizable {
		return
	}
	r := viewRect{right: int32(w), bottom: int32(h)}
	if result(call(e.view, mCheckSizeConstraint, uintptr(unsafe.Pointer(&r)))) == resultOK {
		if cw, ch := r.size(); cw != w || ch != h {
			e.win.resize(cw, ch)
		}
	}
	call(e.view, mOnSize, uintptr(unsafe.Pointer(&r)))
}

// close takes the view down and the window with it. It runs on the
// plugins' thread.
func (e *editor) close() {
	if e.p.ed != e {
		return
	}
	e.p.ed = nil
	e.p.editorOpen.Store(false)
	call(e.view, mRemoved)
	call(e.view, mSetFrame, 0)
	release(e.view)
	drop(e.frame)
	e.win.destroy()
}

// CloseEditor closes the plugin's editor, where it is open.
func (p *Plugin) CloseEditor() {
	onUI(func() {
		if p.ed != nil {
			p.ed.close()
		}
	})
}

// EditorOpen says whether the plugin's editor is open.
func (p *Plugin) EditorOpen() bool { return p.editorOpen.Load() }
