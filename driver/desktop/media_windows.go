package desktop

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/input"
)

// The system's media controls on Windows are the System Media Transport
// Controls: the panel that opens beside the volume as a media key is
// pressed, and the media entry in the quick settings. A desktop program
// reaches them for one of its windows through
// ISystemMediaTransportControlsInterop, a WinRT object driven here by
// its method tables. Their buttons arrive at a delegate written here,
// on a thread of the system's, and go on to the window as media keys.
//
// GUNIM_DEBUG_MEDIA=1 logs each step to standard error.

var (
	combase                                = windows.NewLazySystemDLL("combase.dll")
	procRoInitialize                       = combase.NewProc("RoInitialize")
	procRoGetActivationFactory             = combase.NewProc("RoGetActivationFactory")
	procRoActivateInstance                 = combase.NewProc("RoActivateInstance")
	procWindowsCreateString                = combase.NewProc("WindowsCreateString")
	procWindowsDeleteString                = combase.NewProc("WindowsDeleteString")
	procCreateRandomAccessStreamOverStream = windows.NewLazySystemDLL("shcore.dll").NewProc("CreateRandomAccessStreamOverStream")
	procSHCreateMemStream                  = windows.NewLazySystemDLL("shlwapi.dll").NewProc("SHCreateMemStream")
)

// guid parses a GUID written as text, for the tables below.
func guid(s string) windows.GUID {
	g, err := windows.GUIDFromString("{" + s + "}")
	if err != nil {
		panic(err)
	}
	return g
}

var (
	iidUnknown          = guid("00000000-0000-0000-C000-000000000046")
	iidAgileObject      = guid("94ea2b94-e9cc-49e0-c0ff-ee64ca8f5b90")
	iidSMTCInterop      = guid("ddb0472d-c911-4a1f-86d9-dc3d71a95f5a")
	iidSMTC             = guid("99fa3ff4-1742-42a6-902e-087d41f965ec")
	iidSMTC2            = guid("ea98d2f6-7f3c-4af2-a586-72889808efb1")
	iidMusicProperties2 = guid("00368462-97d3-44b9-b00f-008afcefaf18")
	iidTimeline         = guid("5125316a-c3a2-475b-8507-93534dc88f15")
	iidStreamRefStatics = guid("857309dc-3fbf-4e7d-986f-ef3b1a07a964")
	iidRandomAccessStrm = guid("905a0fe1-bc53-11df-8c49-001e4fc686da")
	iidButtonHandler    = guid("0557e996-7b23-5bae-aa81-ea0d671143a4")
	iidSeekHandler      = guid("44e34f15-bdc0-50a7-ace4-39e91fb753f1")
	mediaDebug          = os.Getenv("GUNIM_DEBUG_MEDIA") == "1"
	errMediaUnavailable = errors.New("desktop: the system's media controls are out of reach")
	smtcPlaybackClosed  = uintptr(0)
	smtcPlaybackPlaying = uintptr(3)
	smtcPlaybackPaused  = uintptr(4)
	smtcMusic           = uintptr(1)
	smtcButtonKeys      = map[uint32]input.Key{0: input.KeyMediaPlay, 1: input.KeyMediaPause, 2: input.KeyMediaStop, 6: input.KeyMediaNext, 7: input.KeyMediaPrevious}
	smtcMethodsOnce     sync.Once
	smtcButtonMethods   delegateVtbl
	smtcSeekMethods     delegateVtbl
)

func mediaLog(format string, args ...any) {
	if mediaDebug {
		fmt.Fprintf(os.Stderr, "gunim media: "+format+"\n", args...)
	}
}

// rtCall calls method i of the COM object obj, and returns its HRESULT
// as an error.
func rtCall(obj uintptr, i int, args ...uintptr) error {
	vtbl := *ptr[unsafe.Pointer](obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(i)*unsafe.Sizeof(uintptr(0))))
	hr, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	if int32(hr) < 0 {
		return fmt.Errorf("HRESULT %#x", uint32(hr))
	}
	return nil
}

// ptr is the pointer p a COM call passed as a number.
func ptr[T any](p uintptr) *T {
	return (*T)(unsafe.Pointer(p)) //nolint:govet // a pointer COM passed
}

// rtRelease calls Release on a COM object.
func rtRelease(obj uintptr) {
	if obj != 0 {
		_ = rtCall(obj, 2)
	}
}

// rtQuery asks obj for the interface iid.
func rtQuery(obj uintptr, iid *windows.GUID) (uintptr, error) {
	var out uintptr
	err := rtCall(obj, 0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	return out, err
}

// hstring makes a WinRT string of s; free it with its delete.
func hstring(s string) (h uintptr, del func()) {
	u, _ := windows.UTF16FromString(s)
	n := len(u) - 1
	var p *uint16
	if n > 0 {
		p = &u[0]
	}
	_, _, _ = procWindowsCreateString.Call(uintptr(unsafe.Pointer(p)), uintptr(n), uintptr(unsafe.Pointer(&h)))
	return h, func() { _, _, _ = procWindowsDeleteString.Call(h) }
}

// factory returns the activation factory of the WinRT class name, as
// the interface iid.
func factory(name string, iid *windows.GUID) (uintptr, error) {
	h, del := hstring(name)
	defer del()
	var out uintptr
	hr, _, _ := procRoGetActivationFactory.Call(h, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	if int32(hr) < 0 {
		return 0, fmt.Errorf("RoGetActivationFactory %s: HRESULT %#x", name, uint32(hr))
	}
	return out, nil
}

// delegate is a WinRT event handler: a pointer to its method table
// first, as COM lays an object out. Invoke runs on a thread of the
// system's.
type delegate struct {
	vtbl   *delegateVtbl
	refs   int32
	iid    windows.GUID
	invoke func(args uintptr)
}

type delegateVtbl struct {
	queryInterface, addRef, release, invoke uintptr
}

func delegateTable(name string) delegateVtbl {
	return delegateVtbl{
		queryInterface: syscall.NewCallback(func(this, riid, ppv uintptr) uintptr {
			d := ptr[delegate](this)
			id := *ptr[windows.GUID](riid)
			if id != iidUnknown && id != iidAgileObject && id != d.iid {
				mediaLog("%s: asked for %v, not given", name, id)
				*ptr[uintptr](ppv) = 0
				return 0x80004002 // E_NOINTERFACE
			}
			atomic.AddInt32(&d.refs, 1)
			*ptr[uintptr](ppv) = this
			return 0
		}),
		addRef: syscall.NewCallback(func(this uintptr) uintptr {
			return uintptr(atomic.AddInt32(&ptr[delegate](this).refs, 1))
		}),
		release: syscall.NewCallback(func(this uintptr) uintptr {
			return uintptr(atomic.AddInt32(&ptr[delegate](this).refs, -1))
		}),
		invoke: syscall.NewCallback(func(this, sender, args uintptr) uintptr {
			ptr[delegate](this).invoke(args)
			return 0
		}),
	}
}

// smtc is the application's media controls, bound to one window.
type smtc struct {
	d    *Driver
	hwnd windows.HWND
	// controls is ISystemMediaTransportControls, and controls2 its
	// second interface, for the timeline.
	controls, controls2 uintptr
	// buttons and seeks are the delegates the controls call, kept alive
	// as long as the controls hold them, with their registrations.
	buttons, seeks         *delegate
	buttonToken, seekToken int64
	key                    string
	cover                  []byte
}

// media is the application's media controls, on the main thread.
var media *smtc

// SetNowPlaying implements [driver.NowPlayer]: what plays shows in the
// System Media Transport Controls.
func (d *Driver) SetNowPlaying(np *driver.NowPlaying) error {
	return d.call(func() error {
		if np == nil {
			if media != nil {
				media.close()
				media = nil
			}
			return nil
		}
		w := d.mediaWindow()
		if w == nil {
			return errMediaUnavailable
		}
		hwnd, err := w.gw.GetWin32Window()
		if err != nil {
			return err
		}
		if media != nil && media.hwnd != hwnd {
			media.close()
			media = nil
		}
		if media == nil {
			if media, err = openSMTC(d, hwnd); err != nil {
				mediaLog("open: %v", err)
				return err
			}
		}
		if err := media.show(np); err != nil {
			mediaLog("show: %v", err)
			return err
		}
		return nil
	})
}

// openSMTC binds the media controls to the window hwnd, and listens to
// their buttons and their bar.
func openSMTC(d *Driver, hwnd windows.HWND) (*smtc, error) {
	hr, _, _ := procRoInitialize.Call(0) // single-threaded, as OLE has it
	mediaLog("RoInitialize: HRESULT %#x", uint32(hr))
	interop, err := factory("Windows.Media.SystemMediaTransportControls", &iidSMTCInterop)
	if err != nil {
		return nil, err
	}
	defer rtRelease(interop)
	m := &smtc{d: d, hwnd: hwnd}
	// GetForWindow follows IInspectable's six methods.
	if err = rtCall(interop, 6, uintptr(hwnd), uintptr(unsafe.Pointer(&iidSMTC)), uintptr(unsafe.Pointer(&m.controls))); err != nil {
		return nil, fmt.Errorf("GetForWindow: %w", err)
	}
	mediaLog("bound to window %#x", hwnd)
	for i, on := range map[int]bool{11: true, 13: true, 15: true, 17: true, 25: true, 27: true} {
		// IsEnabled, IsPlayEnabled, IsStopEnabled, IsPauseEnabled,
		// IsPreviousEnabled and IsNextEnabled.
		if err = rtCall(m.controls, i, boolArg(on)); err != nil {
			mediaLog("enable %d: %v", i, err)
		}
	}
	smtcMethodsOnce.Do(func() {
		smtcButtonMethods = delegateTable("buttons")
		smtcSeekMethods = delegateTable("seeks")
	})
	m.buttons = &delegate{vtbl: &smtcButtonMethods, refs: 1, iid: iidButtonHandler, invoke: m.pressed}
	if err = rtCall(m.controls, 32, uintptr(unsafe.Pointer(m.buttons)), uintptr(unsafe.Pointer(&m.buttonToken))); err != nil {
		mediaLog("add_ButtonPressed: %v", err)
	}
	if c2, qerr := rtQuery(m.controls, &iidSMTC2); qerr == nil {
		m.controls2 = c2
		m.seeks = &delegate{vtbl: &smtcSeekMethods, refs: 1, iid: iidSeekHandler, invoke: m.seekAsked}
		if err = rtCall(c2, 13, uintptr(unsafe.Pointer(m.seeks)), uintptr(unsafe.Pointer(&m.seekToken))); err != nil {
			mediaLog("add_PlaybackPositionChangeRequested: %v", err)
		}
	} else {
		mediaLog("ISystemMediaTransportControls2: %v", qerr)
	}
	return m, nil
}

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

// pressed takes a button of the controls to the window, as its media
// key. It runs on a thread of the system's.
func (m *smtc) pressed(args uintptr) {
	var button uint32
	if err := rtCall(args, 6, uintptr(unsafe.Pointer(&button))); err != nil {
		mediaLog("get_Button: %v", err)
		return
	}
	mediaLog("button %d", button)
	if k, ok := smtcButtonKeys[button]; ok {
		m.d.mediaKey(k)
	}
}

// seekAsked takes a move along the controls' bar to the window. It runs
// on a thread of the system's.
func (m *smtc) seekAsked(args uintptr) {
	var ticks int64
	if err := rtCall(args, 6, uintptr(unsafe.Pointer(&ticks))); err != nil {
		mediaLog("get_RequestedPlaybackPosition: %v", err)
		return
	}
	mediaLog("seek to %d ticks", ticks)
	m.d.mediaSeek(time.Duration(ticks) * 100)
}

// show puts np in the controls.
func (m *smtc) show(np *driver.NowPlaying) error {
	status := smtcPlaybackPaused
	if np.Playing {
		status = smtcPlaybackPlaying
	}
	if err := rtCall(m.controls, 7, status); err != nil {
		return fmt.Errorf("put_PlaybackStatus: %w", err)
	}
	if key := np.Title + "\x00" + np.Artist + "\x00" + np.Album; key != m.key || !bytes.Equal(np.Cover, m.cover) {
		m.key, m.cover = key, np.Cover
		if err := m.describe(np); err != nil {
			return err
		}
	}
	m.timeline(np)
	return nil
}

// describe tells the controls the track: its title, artist, album and
// cover.
func (m *smtc) describe(np *driver.NowPlaying) error {
	var updater uintptr
	if err := rtCall(m.controls, 8, uintptr(unsafe.Pointer(&updater))); err != nil {
		return fmt.Errorf("get_DisplayUpdater: %w", err)
	}
	defer rtRelease(updater)
	if err := rtCall(updater, 7, smtcMusic); err != nil {
		return fmt.Errorf("put_Type: %w", err)
	}
	var music uintptr
	if err := rtCall(updater, 12, uintptr(unsafe.Pointer(&music))); err != nil {
		return fmt.Errorf("get_MusicProperties: %w", err)
	}
	defer rtRelease(music)
	set := func(obj uintptr, i int, s string) {
		h, del := hstring(s)
		defer del()
		if err := rtCall(obj, i, h); err != nil {
			mediaLog("put %d: %v", i, err)
		}
	}
	set(music, 7, np.Title)
	set(music, 11, np.Artist)
	if music2, err := rtQuery(music, &iidMusicProperties2); err == nil {
		set(music2, 7, np.Album)
		rtRelease(music2)
	}
	if ref := streamRef(np.Cover); ref != 0 {
		if err := rtCall(updater, 11, ref); err != nil {
			mediaLog("put_Thumbnail: %v", err)
		}
		rtRelease(ref)
	} else if err := rtCall(updater, 11, 0); err != nil {
		mediaLog("put_Thumbnail nil: %v", err)
	}
	if err := rtCall(updater, 17); err != nil {
		return fmt.Errorf("updating the display: %w", err)
	}
	mediaLog("showing %q by %q", np.Title, np.Artist)
	return nil
}

// streamRef makes a stream reference of the bytes of a picture, for a
// thumbnail, or returns 0.
func streamRef(b []byte) uintptr {
	if len(b) == 0 {
		return 0
	}
	stream, _, _ := procSHCreateMemStream.Call(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	if stream == 0 {
		return 0
	}
	defer rtRelease(stream)
	var ras uintptr
	hr, _, _ := procCreateRandomAccessStreamOverStream.Call(stream, 0, uintptr(unsafe.Pointer(&iidRandomAccessStrm)), uintptr(unsafe.Pointer(&ras)))
	if int32(hr) < 0 {
		mediaLog("CreateRandomAccessStreamOverStream: HRESULT %#x", uint32(hr))
		return 0
	}
	defer rtRelease(ras)
	statics, err := factory("Windows.Storage.Streams.RandomAccessStreamReference", &iidStreamRefStatics)
	if err != nil {
		mediaLog("%v", err)
		return 0
	}
	defer rtRelease(statics)
	var ref uintptr
	if err := rtCall(statics, 8, ras, uintptr(unsafe.Pointer(&ref))); err != nil {
		mediaLog("CreateFromStream: %v", err)
		return 0
	}
	return ref
}

// timeline tells the controls how long the track is and how far it has
// played, for their bar.
func (m *smtc) timeline(np *driver.NowPlaying) {
	if m.controls2 == 0 || np.Length <= 0 {
		return
	}
	h, del := hstring("Windows.Media.SystemMediaTransportControlsTimelineProperties")
	defer del()
	var inst uintptr
	if hr, _, _ := procRoActivateInstance.Call(h, uintptr(unsafe.Pointer(&inst))); int32(hr) < 0 {
		mediaLog("RoActivateInstance timeline: HRESULT %#x", uint32(hr))
		return
	}
	defer rtRelease(inst)
	tl, err := rtQuery(inst, &iidTimeline)
	if err != nil {
		mediaLog("timeline: %v", err)
		return
	}
	defer rtRelease(tl)
	ticks := func(d time.Duration) uintptr { return uintptr(int64(d / 100)) }
	_ = rtCall(tl, 7, 0)                   // StartTime
	_ = rtCall(tl, 9, ticks(np.Length))    // EndTime
	_ = rtCall(tl, 11, 0)                  // MinSeekTime
	_ = rtCall(tl, 13, ticks(np.Length))   // MaxSeekTime
	_ = rtCall(tl, 15, ticks(np.Position)) // Position
	if err := rtCall(m.controls2, 12, tl); err != nil {
		mediaLog("UpdateTimelineProperties: %v", err)
	}
}

// close takes what plays out of the controls, and stops listening.
func (m *smtc) close() {
	_ = rtCall(m.controls, 7, smtcPlaybackClosed)
	if m.controls2 != 0 {
		_ = rtCall(m.controls2, 14, uintptr(m.seekToken))
		rtRelease(m.controls2)
	}
	_ = rtCall(m.controls, 33, uintptr(m.buttonToken))
	_ = rtCall(m.controls, 11, 0) // IsEnabled
	rtRelease(m.controls)
	mediaLog("closed")
}
