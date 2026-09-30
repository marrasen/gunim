package glfw

import (
	"image"
	"image/draw"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The tray icon, a gunim addition: an icon in the notification area
// with a menu, through Shell_NotifyIcon. Its messages come to a message
// window of gunim's own, on the main thread, which the event loop pumps
// with the rest. It is on Windows alone.

var (
	shell32tray                = windows.NewLazySystemDLL("shell32.dll")
	user32tray                 = windows.NewLazySystemDLL("user32.dll")
	procShellNotifyIconW       = shell32tray.NewProc("Shell_NotifyIconW")
	procCreatePopupMenu        = user32tray.NewProc("CreatePopupMenu")
	procAppendMenuW            = user32tray.NewProc("AppendMenuW")
	procTrackPopupMenuEx       = user32tray.NewProc("TrackPopupMenuEx")
	procDestroyMenu            = user32tray.NewProc("DestroyMenu")
	procSetMenuDefaultItem     = user32tray.NewProc("SetMenuDefaultItem")
	procRegisterWindowMessageW = user32tray.NewProc("RegisterWindowMessageW")
	trayClassRegistered        bool
	trayTaskbarCreated         uint32
	trayNow                    *trayState
	trayProcPtr                       = windows.NewCallback(trayProc)
	trayHWNDMessage                   = windows.HWND(^uintptr(2)) // HWND_MESSAGE, -3
	trayClassName                     = "GunimTray"
	trayMessage                uint32 = 0x0400 + 0x4a1 // WM_USER + 0x4a1
)

const (
	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4
	nifMessage    = 0x1
	nifIcon       = 0x2
	nifTip        = 0x4
	nifShowTip    = 0x80
	ninSelect     = 0x400
	ninKeySelect  = 0x401
	wmContextMenu = 0x007b

	mfString    = 0x0
	mfGrayed    = 0x1
	mfChecked   = 0x8
	mfPopup     = 0x10
	mfSeparator = 0x800

	tpmRightButton = 0x2
	tpmNoNotify    = 0x80
	tpmReturnCmd   = 0x100
)

// notifyIconData is NOTIFYICONDATAW.
type notifyIconData struct {
	cbSize           uint32
	hWnd             windows.HWND
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            _HICON
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         windows.GUID
	hBalloonIcon     _HICON
}

// TrayMenuItem is a line of the tray icon's menu.
type TrayMenuItem struct {
	Title                                 string
	ID                                    int
	Items                                 []TrayMenuItem
	Checked, Disabled, Separator, Default bool
}

// TrayIcon is what the tray shows: the icon at several sizes, its
// tooltip and menu, and what a pick from the menu and a click on the
// icon call. They are called on the main thread.
type TrayIcon struct {
	Images  []image.Image
	Tooltip string
	Items   []TrayMenuItem
	OnPick  func(id int)
	OnClick func()
}

// trayState is the icon shown.
type trayState struct {
	hwnd  windows.HWND
	icon  _HICON
	t     TrayIcon
	added bool
}

// SetTrayIcon shows t in the notification area in place of the icon
// shown before, or takes it away when t is nil. It must be called on
// the main thread.
func SetTrayIcon(t *TrayIcon) error {
	if !_glfw.initialized {
		return NotInitialized
	}
	if t == nil {
		if trayNow != nil {
			trayNow.remove()
			trayNow = nil
		}
		return nil
	}
	if trayNow == nil {
		s, err := newTrayState()
		if err != nil {
			return err
		}
		trayNow = s
	}
	return trayNow.show(*t)
}

func newTrayState() (*trayState, error) {
	if !trayClassRegistered {
		var wc _WNDCLASSEXW
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		wc.lpfnWndProc = _WNDPROC(trayProcPtr)
		wc.hInstance = _glfw.platformWindow.instance
		name, err := windows.UTF16PtrFromString(trayClassName)
		if err != nil {
			return nil, err
		}
		wc.lpszClassName = name
		if _, err := _RegisterClassExW(&wc); err != nil {
			return nil, err
		}
		trayClassRegistered = true
		if p, err := windows.UTF16PtrFromString("TaskbarCreated"); err == nil {
			r, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(p)))
			trayTaskbarCreated = uint32(r)
		}
	}
	h, err := _CreateWindowExW(0, trayClassName, "", 0, 0, 0, 0, 0, trayHWNDMessage, 0, _glfw.platformWindow.instance, nil)
	if err != nil {
		return nil, err
	}
	return &trayState{hwnd: h}, nil
}

// data is the NOTIFYICONDATAW for the icon, with flags.
func (s *trayState) data(flags uint32) *notifyIconData {
	d := &notifyIconData{hWnd: s.hwnd, uID: 1, uFlags: flags, uCallbackMessage: trayMessage, hIcon: s.icon}
	d.cbSize = uint32(unsafe.Sizeof(*d))
	tip, _ := windows.UTF16FromString(s.t.Tooltip)
	copy(d.szTip[:len(d.szTip)-1], tip)
	return d
}

// show puts t in the notification area.
func (s *trayState) show(t TrayIcon) error {
	s.t = t
	if img := trayImage(t.Images); img != nil {
		icon, err := createIcon(img, 0, 0, true)
		if err != nil {
			return err
		}
		if s.icon != 0 {
			_ = _DestroyIcon(s.icon)
		}
		s.icon = icon
	}
	flags := uint32(nifMessage | nifIcon | nifTip | nifShowTip)
	if s.added {
		return shellNotify(nimModify, s.data(flags))
	}
	if err := shellNotify(nimAdd, s.data(flags)); err != nil {
		return err
	}
	s.added = true
	d := s.data(0)
	d.uVersion = 4
	return shellNotify(nimSetVersion, d)
}

// remove takes the icon away, and its window.
func (s *trayState) remove() {
	if s.added {
		_ = shellNotify(nimDelete, s.data(0))
	}
	if s.icon != 0 {
		_ = _DestroyIcon(s.icon)
	}
	_ = _DestroyWindow(s.hwnd)
}

func shellNotify(msg uint32, d *notifyIconData) error {
	r, _, e := procShellNotifyIconW.Call(uintptr(msg), uintptr(unsafe.Pointer(d)))
	if r == 0 {
		return e
	}
	return nil
}

// trayImage is the image nearest the size of a small icon, as glfw
// has images, or nil for none.
func trayImage(images []image.Image) *Image {
	want := 16
	if n, err := _GetSystemMetrics(_SM_CXSMICON); err == nil && n > 0 {
		want = int(n)
	}
	var best image.Image
	gap := func(m image.Image) int {
		d := m.Bounds().Dx() - want
		if d < 0 {
			return -d
		}
		return d
	}
	for _, m := range images {
		if m.Bounds().Dx() > 0 && m.Bounds().Dy() > 0 && (best == nil || gap(m) < gap(best)) {
			best = m
		}
	}
	if best == nil {
		return nil
	}
	b := best.Bounds()
	m := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(m, m.Bounds(), best, b.Min, draw.Src)
	return &Image{Width: b.Dx(), Height: b.Dy(), Pixels: m.Pix}
}

// trayProc hears the tray icon's messages.
func trayProc(hWnd windows.HWND, uMsg uint32, wParam _WPARAM, lParam _LPARAM) uintptr {
	s := trayNow
	switch {
	case s == nil || s.hwnd != hWnd:
	case uMsg == trayMessage:
		switch uint32(lParam) & 0xffff {
		case ninSelect, ninKeySelect, _WM_LBUTTONUP:
			if s.t.OnClick != nil {
				s.t.OnClick()
				return 0
			}
			s.showMenu(int32(int16(wParam&0xffff)), int32(int16(wParam>>16&0xffff)))
		case wmContextMenu, _WM_RBUTTONUP:
			s.showMenu(int32(int16(wParam&0xffff)), int32(int16(wParam>>16&0xffff)))
		}
		return 0
	case uMsg == trayTaskbarCreated && trayTaskbarCreated != 0:
		// Explorer started again, with a taskbar that knows nothing
		// of the icon: it goes back.
		s.added = false
		_ = s.show(s.t)
		return 0
	}
	return uintptr(_DefWindowProcW(hWnd, uMsg, wParam, lParam))
}

// showMenu shows the icon's menu at x, y, and calls OnPick with the
// line picked.
func (s *trayState) showMenu(x, y int32) {
	ids := map[uintptr]int{}
	next := uintptr(0)
	var build func(items []TrayMenuItem) uintptr
	build = func(items []TrayMenuItem) uintptr {
		m, _, _ := procCreatePopupMenu.Call()
		for _, it := range items {
			title, _ := windows.UTF16PtrFromString(it.Title)
			switch {
			case it.Separator:
				_, _, _ = procAppendMenuW.Call(m, mfSeparator, 0, 0)
			case len(it.Items) > 0:
				sub := build(it.Items)
				flags := uintptr(mfPopup)
				if it.Disabled {
					flags |= mfGrayed
				}
				_, _, _ = procAppendMenuW.Call(m, flags, sub, uintptr(unsafe.Pointer(title)))
			default:
				next++
				ids[next] = it.ID
				flags := uintptr(mfString)
				if it.Checked {
					flags |= mfChecked
				}
				if it.Disabled {
					flags |= mfGrayed
				}
				_, _, _ = procAppendMenuW.Call(m, flags, next, uintptr(unsafe.Pointer(title)))
				if it.Default {
					_, _, _ = procSetMenuDefaultItem.Call(m, next, 0)
				}
			}
		}
		return m
	}
	menu := build(s.t.Items)
	defer func() { _, _, _ = procDestroyMenu.Call(menu) }()
	// The menu closes on a click elsewhere only while its window is in
	// front, and needs a message after it to close cleanly.
	_ = _SetForegroundWindow(s.hwnd)
	cmd, _, _ := procTrackPopupMenuEx.Call(menu, tpmRightButton|tpmNoNotify|tpmReturnCmd, uintptr(x), uintptr(y), uintptr(s.hwnd), 0)
	_ = _PostMessageW(s.hwnd, _WM_NULL, 0, 0)
	if id, ok := ids[cmd]; ok && cmd != 0 && s.t.OnPick != nil {
		s.t.OnPick(id)
	}
}
