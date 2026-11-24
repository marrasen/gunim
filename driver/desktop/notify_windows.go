package desktop

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/internal/glfw"
	"github.com/marrasen/gunim/internal/toast"
)

// On Windows a message from the tray shows as a toast where it can: a WinRT ToastNotification, driven here by its
// method tables, as media_windows.go drives the media controls. Each toast carries its tag in its launch argument,
// and a click hands that back two ways, both heard here:
//
//   - The toast's Activated event, while the toast object lives in this process. The driver keeps the last
//     toastKept of them.
//   - A COM object of the driver's own, an INotificationActivationCallback, registered for this process with
//     CoRegisterClassObject until the icon goes. Windows calls it for a click in the notification centre, on a
//     toast of any age.
//
// One click may come both ways, so each toast has an ID, in its launch argument with the tag, and a click is heard
// once per ID.
//
// An unpackaged program shows toasts under an AppUserModelID it registers itself, under the current user's
// Software\Classes\AppUserModelId\<AppID>: the name and picture its toasts show, and the class ID of its COM
// object. It registers at its first message, so a program that shows none writes nothing, or as its icon goes up
// where it registered on an earlier run, or was started by a click. The class ID's LocalServer32 names the program file, so a click on a toast after the program ended starts
// it again, with the argument -Embedding, and the click is heard once it puts its icon up.
//
// Where any of that fails, messages show as tray balloons, as before: a balloon's click names no message.
//
// GUNIM_DEBUG_NOTIFY=1 logs each step to standard error.

var (
	ole32                         = windows.NewLazySystemDLL("ole32.dll")
	procCoRegisterClassObject     = ole32.NewProc("CoRegisterClassObject")
	procCoRevokeClassObject       = ole32.NewProc("CoRevokeClassObject")
	procWindowsGetStringRawBuffer = combase.NewProc("WindowsGetStringRawBuffer")

	iidToastManagerStatics   = guid("50ac103f-d235-4598-bbef-98fe4d1a3ad4")
	iidToastFactory          = guid("04124b20-82c6-4229-b109-fd9ed4662b53")
	iidXMLDocument           = guid("f7f3a506-1e87-42d6-bcfb-b8c809fa5494")
	iidXMLDocumentIO         = guid("6cd0e74e-ee65-4489-9ebf-ca43e87ba637")
	iidToastActivatedArgs    = guid("e3bf92f3-c197-436f-8265-0625824f8dac")
	iidToastActivatedHandler = guid("ab54de2d-97d9-5528-b6ad-105afe156530")
	iidToastDismissedHandler = guid("61c2402f-0ed0-5a18-ab69-59f4aa99a368")
	iidToastFailedHandler    = guid("95e3e803-c969-5e3a-9753-ea2ad22a9a33")
	iidClassFactory          = guid("00000001-0000-0000-C000-000000000046")
	iidActivationCallback    = guid("53E31837-6600-4A81-9395-75CFFE746F94")

	notifyDebug = os.Getenv("GUNIM_DEBUG_NOTIFY") == "1"

	toastMethodsOnce                                          sync.Once
	toastActivatedMethods, toastDismissedMethods, toastFailed delegateVtbl
)

const (
	// toastKept is how many toast objects the driver keeps, for their events.
	toastKept = 64
	// clsctxLocalServer and regclsMultipleUse are CLSCTX_LOCAL_SERVER and REGCLS_MULTIPLEUSE.
	clsctxLocalServer = 4
	regclsMultipleUse = 1
	eNoInterface      = 0x80004002
	classENoAggregate = 0x80040110
)

func notifyLog(format string, args ...any) {
	if notifyDebug {
		fmt.Fprintf(os.Stderr, "gunim notify: "+format+"\n", args...)
	}
}

// toaster shows the tray's messages as toasts. It lives on the main thread, but for the delegates' Invoke, which
// runs on a thread of the system's and posts to the main thread.
type toaster struct {
	d     *Driver
	appID string
	// notifier is the IToastNotifier for appID, and factory the IToastNotificationFactory.
	notifier, factory uintptr
	// cookie is the COM object's registration, or 0.
	cookie uint32
	// run starts each toast's ID, apart for each run of the program, and next numbers the toasts.
	run  string
	next uint64
	// shown are the toasts kept, oldest first, and last the ID of the one shown last.
	shown []*shownToast
	last  string
	// heard are the IDs of the toasts whose click was heard, oldest first, so a click is heard once.
	heard []string
}

// shownToast is a toast shown, with its handlers.
type shownToast struct {
	id, title, body, tag string
	toast                uintptr
	handlers             [3]*delegate
	tokens               [3]int64
}

// toasts is the toaster, on the main thread, or nil where none is up yet or messages show as balloons. toastTray is
// the tray shown, for the toaster to be made from at the first message. toastsFailed is the AppID toasts failed
// for, so they are tried again only for another.
var (
	toasts       *toaster
	toastTray    *driver.Tray
	toastsFailed string
)

// setToastTray keeps the tray t for the toasts, as its icon goes up, and takes the toaster down where the icon goes
// or the AppID changes. Where the AppID was registered on an earlier run, or a click on a toast started the
// program, the toaster comes up at once, to hear clicks on toasts already shown. It runs on the main thread.
func setToastTray(d *Driver, t *driver.Tray) {
	if t == nil || len(t.Icon) == 0 {
		toastTray = nil
		closeToasts()
		return
	}
	toastTray = t
	appID := toastAppID(t)
	if toasts != nil && toasts.appID != appID {
		closeToasts()
	}
	if toasts == nil && (startedByToast() || appIDRegistered(appID)) {
		readyToasts(d)
	}
}

// startedByToast reports whether COM started the program, as for a click on one of its toasts after it ended.
func startedByToast() bool {
	for _, a := range os.Args[1:] {
		if strings.EqualFold(a, "-Embedding") || strings.EqualFold(a, "/Embedding") {
			return true
		}
	}
	return false
}

// appIDRegistered reports whether appID is registered for toasts with a COM object, as an earlier run left it.
func appIDRegistered(appID string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId\`+appID, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer func() { _ = k.Close() }()
	_, _, err = k.GetStringValue("CustomActivator")
	return err == nil
}

// toastAppID is the AppUserModelID of t's toasts.
func toastAppID(t *driver.Tray) string {
	exe, _ := os.Executable()
	return toast.AppID(t.AppID, exe)
}

// readyToasts makes the toaster for the tray shown, where none is up and toasts did not fail for its AppID, and
// reports whether one is up. It runs on the main thread, at the first message, as the toaster registers the
// AppUserModelID: a program that shows no message writes nothing.
func readyToasts(d *Driver) bool {
	if toasts != nil {
		return true
	}
	if toastTray == nil {
		return false
	}
	exe, _ := os.Executable()
	appID := toast.AppID(toastTray.AppID, exe)
	if toastsFailed == appID {
		return false
	}
	tt, err := openToaster(d, toastTray, appID, exe)
	if err != nil {
		notifyLog("toasts for %s: %v; messages show as balloons", appID, err)
		toastsFailed = appID
		return false
	}
	notifyLog("toasts for %s", appID)
	toasts = tt
	return true
}

// closeToasts takes the toasts down. It runs on the main thread.
func closeToasts() {
	if toasts != nil {
		toasts.close()
		toasts = nil
	}
}

// openToaster registers appID and readies its notifier.
func openToaster(d *Driver, t *driver.Tray, appID, exe string) (*toaster, error) {
	hr, _, _ := procRoInitialize.Call(0) // single-threaded, as OLE has it
	notifyLog("RoInitialize: HRESULT %#x", uint32(hr))
	name := t.AppName
	if name == "" {
		name = toast.ProgramName(exe)
	}
	clsid := guid(toast.ActivatorCLSID(appID))
	if err := registerAppID(appID, name, iconFile(appID, t.Icon), exe, clsid); err != nil {
		return nil, fmt.Errorf("registering the AppUserModelID: %w", err)
	}
	statics, err := factory("Windows.UI.Notifications.ToastNotificationManager", &iidToastManagerStatics)
	if err != nil {
		return nil, err
	}
	defer rtRelease(statics)
	tt := &toaster{d: d, appID: appID, run: runID()}
	h, del := hstring(appID)
	defer del()
	// CreateToastNotifierWithId.
	if err := rtCall(statics, 7, h, uintptr(unsafe.Pointer(&tt.notifier))); err != nil {
		return nil, fmt.Errorf("CreateToastNotifierWithId: %w", err)
	}
	// get_Setting fails for an AppUserModelID Windows does not know, and says whether the user let it show toasts.
	var setting int32
	if err := rtCall(tt.notifier, 8, uintptr(unsafe.Pointer(&setting))); err != nil {
		tt.close()
		return nil, fmt.Errorf("get_Setting: %w", err)
	}
	notifyLog("the notifier's setting is %d (0 is on)", setting)
	if tt.factory, err = factory("Windows.UI.Notifications.ToastNotification", &iidToastFactory); err != nil {
		tt.close()
		return nil, err
	}
	toastMethodsOnce.Do(func() {
		toastActivatedMethods = delegateTable("toast activated")
		toastDismissedMethods = delegateTable("toast dismissed")
		toastFailed = delegateTable("toast failed")
	})
	tt.cookie = registerActivator(d, clsid)
	return tt, nil
}

// runID is a few random bytes, apart for each run, to start the toasts' IDs.
func runID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// iconFile writes the largest of the icon's pictures as a PNG in the user's cache folder, for the toasts to show,
// and returns its path, or "" where it could not.
func iconFile(appID string, icon []image.Image) string {
	var best image.Image
	for _, m := range icon {
		if best == nil || m.Bounds().Dx() > best.Bounds().Dx() {
			best = m
		}
	}
	dir, err := os.UserCacheDir()
	if best == nil || err != nil {
		return ""
	}
	var b bytes.Buffer
	if err := png.Encode(&b, best); err != nil {
		return ""
	}
	path := filepath.Join(dir, appID, "notify.png")
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, b.Bytes()) {
		return path
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		notifyLog("icon: %v", err)
		return ""
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		notifyLog("icon: %v", err)
		return ""
	}
	return path
}

// registerAppID writes appID's name, picture and COM object under the current user's classes, as an unpackaged
// program registers itself to show toasts.
func registerAppID(appID, name, icon, exe string, clsid windows.GUID) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId\`+appID, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	if err := k.SetStringValue("DisplayName", name); err != nil {
		return err
	}
	if icon != "" {
		if err := k.SetStringValue("IconUri", icon); err != nil {
			return err
		}
	}
	if err := k.SetStringValue("CustomActivator", clsid.String()); err != nil {
		return err
	}
	if exe == "" {
		return nil
	}
	s, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\CLSID\`+clsid.String()+`\LocalServer32`, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	return s.SetStringValue("", `"`+exe+`"`)
}

// show shows a toast with title, body and tag.
func (tt *toaster) show(title, body, tag string) error {
	tt.next++
	st := &shownToast{id: fmt.Sprintf("%s-%x", tt.run, tt.next), title: title, body: body, tag: tag}
	content := toast.XML(title, body, toast.Launch(st.id, tag))
	doc, err := xmlDocument(content)
	if err != nil {
		return err
	}
	defer rtRelease(doc)
	// CreateToastNotification.
	if err := rtCall(tt.factory, 6, doc, uintptr(unsafe.Pointer(&st.toast))); err != nil {
		return fmt.Errorf("CreateToastNotification: %w", err)
	}
	st.handlers = [3]*delegate{
		{vtbl: &toastActivatedMethods, refs: 1, iid: iidToastActivatedHandler, invoke: tt.activated},
		{vtbl: &toastDismissedMethods, refs: 1, iid: iidToastDismissedHandler, invoke: func(uintptr) { tt.dismissed(st.id) }},
		{vtbl: &toastFailed, refs: 1, iid: iidToastFailedHandler, invoke: func(uintptr) { tt.failed(st.id) }},
	}
	// add_Activated, add_Dismissed and add_Failed.
	for i, at := range []int{11, 9, 13} {
		if err := rtCall(st.toast, at, uintptr(unsafe.Pointer(st.handlers[i])), uintptr(unsafe.Pointer(&st.tokens[i]))); err != nil {
			notifyLog("adding handler %d: %v", at, err)
			st.handlers[i] = nil
		}
	}
	// Show.
	if err := rtCall(tt.notifier, 6, st.toast); err != nil {
		st.drop()
		return fmt.Errorf("Show: %w", err)
	}
	notifyLog("shown toast %s", st.id)
	tt.shown = append(tt.shown, st)
	tt.last = st.id
	if len(tt.shown) > toastKept {
		tt.shown[0].drop()
		tt.shown = tt.shown[1:]
	}
	return nil
}

// xmlDocument makes an XmlDocument of content, as its IXmlDocument.
func xmlDocument(content string) (uintptr, error) {
	h, del := hstring("Windows.Data.Xml.Dom.XmlDocument")
	defer del()
	var inst uintptr
	if hr, _, _ := procRoActivateInstance.Call(h, uintptr(unsafe.Pointer(&inst))); int32(hr) < 0 {
		return 0, fmt.Errorf("RoActivateInstance XmlDocument: HRESULT %#x", uint32(hr))
	}
	defer rtRelease(inst)
	io, err := rtQuery(inst, &iidXMLDocumentIO)
	if err != nil {
		return 0, fmt.Errorf("IXmlDocumentIO: %w", err)
	}
	defer rtRelease(io)
	text, delText := hstring(content)
	defer delText()
	// LoadXml.
	if err := rtCall(io, 6, text); err != nil {
		return 0, fmt.Errorf("LoadXml: %w", err)
	}
	doc, err := rtQuery(inst, &iidXMLDocument)
	if err != nil {
		return 0, fmt.Errorf("IXmlDocument: %w", err)
	}
	return doc, nil
}

// drop takes the toast's handlers away and lets go of it. It stays in the notification centre, where a click on it
// reaches the COM object.
func (st *shownToast) drop() {
	for i, at := range []int{12, 10, 14} {
		if st.handlers[i] != nil {
			_ = rtCall(st.toast, at, uintptr(st.tokens[i]))
		}
	}
	rtRelease(st.toast)
	st.toast = 0
}

// activated hears a toast's Activated event, on a thread of the system's.
func (tt *toaster) activated(args uintptr) {
	ea, err := rtQuery(args, &iidToastActivatedArgs)
	if err != nil {
		notifyLog("IToastActivatedEventArgs: %v", err)
		return
	}
	defer rtRelease(ea)
	var h uintptr
	// get_Arguments.
	if err := rtCall(ea, 6, uintptr(unsafe.Pointer(&h))); err != nil {
		notifyLog("get_Arguments: %v", err)
		return
	}
	launch := hstringText(h)
	_, _, _ = procWindowsDeleteString.Call(h)
	notifyLog("activated: %q", launch)
	tt.d.post(func() { clicked(launch) })
}

// dismissed hears a toast leave the screen, on a thread of the system's.
func (tt *toaster) dismissed(id string) {
	tt.d.post(func() {
		if toasts == tt && id == tt.last {
			notifyLog("dismissed %s", id)
			glfw.TrayNotifyGone()
		}
	})
}

// failed hears that a toast could not show, on a thread of the system's: the message shows as a balloon, and the
// rest do too.
func (tt *toaster) failed(id string) {
	tt.d.post(func() {
		if toasts != tt {
			return
		}
		for _, st := range tt.shown {
			if st.id != id {
				continue
			}
			notifyLog("toast %s failed; showing balloons", id)
			toastsFailed = tt.appID
			closeToasts()
			_ = glfw.TrayNotify(st.title, st.body, st.tag)
			return
		}
	})
}

// clicked hears a click on a toast with the launch argument launch, once for each toast. It runs on the main
// thread.
func clicked(launch string) {
	tt := toasts
	id, tag, ok := toast.ParseLaunch(launch)
	if tt == nil || !ok {
		notifyLog("a click with %q, heard by no toaster", launch)
		return
	}
	for _, h := range tt.heard {
		if h == id {
			return
		}
	}
	tt.heard = append(tt.heard, id)
	if len(tt.heard) > toastKept {
		tt.heard = tt.heard[1:]
	}
	trayClicked(tag)
}

// trayClicked runs the tray's calls for a click; a test puts its own in its place.
var trayClicked = glfw.TrayNotifyClicked

// heardThroughCOM takes a click the COM object heard to the main thread; a test puts its own in its place.
var heardThroughCOM = func(launch string) {
	if d := activatorDriver.Load(); d != nil {
		d.post(func() { clicked(launch) })
	}
}

// close lets go of the toasts and the notifier, and revokes the COM object.
func (tt *toaster) close() {
	for _, st := range tt.shown {
		st.drop()
	}
	tt.shown = nil
	if tt.cookie != 0 {
		_, _, _ = procCoRevokeClassObject.Call(uintptr(tt.cookie))
		activatorDriver.Store(nil)
	}
	rtRelease(tt.factory)
	rtRelease(tt.notifier)
}

// hstringText is the text of a WinRT string.
func hstringText(h uintptr) string {
	if h == 0 {
		return ""
	}
	var n uint32
	p, _, _ := procWindowsGetStringRawBuffer.Call(h, uintptr(unsafe.Pointer(&n)))
	if p == 0 || n == 0 {
		return ""
	}
	return string(utf16.Decode(unsafe.Slice(ptr[uint16](p), n)))
}

// The COM object that hears clicks from the notification centre: a class factory, and the
// INotificationActivationCallback it hands out. There is one of each for the process, held here for as long as
// the process runs, as COM holds pointers to them.

type comObject struct {
	vtbl unsafe.Pointer
	refs int32
}

var (
	activatorOnce   sync.Once
	activatorDriver atomic.Pointer[Driver]
	classFactoryObj comObject
	activatorObj    comObject
	classFactoryTbl [5]uintptr
	activatorTbl    [4]uintptr
)

// comQuery answers QueryInterface for an object of the interface iid alone.
func comQuery(iid windows.GUID) uintptr {
	return syscall.NewCallback(func(this, riid, ppv uintptr) uintptr {
		id := *ptr[windows.GUID](riid)
		if id != iidUnknown && id != iid {
			*ptr[uintptr](ppv) = 0
			return eNoInterface
		}
		atomic.AddInt32(&ptr[comObject](this).refs, 1)
		*ptr[uintptr](ppv) = this
		return 0
	})
}

// registerActivator registers the process's COM object for clsid, and returns its cookie, or 0 where COM would not
// take it.
func registerActivator(d *Driver, clsid windows.GUID) uint32 {
	activatorOnce.Do(buildActivator)
	activatorDriver.Store(d)
	var cookie uint32
	hr, _, _ := procCoRegisterClassObject.Call(uintptr(unsafe.Pointer(&clsid)), uintptr(unsafe.Pointer(&classFactoryObj)),
		clsctxLocalServer, regclsMultipleUse, uintptr(unsafe.Pointer(&cookie)))
	if int32(hr) < 0 {
		notifyLog("CoRegisterClassObject: HRESULT %#x", uint32(hr))
		activatorDriver.Store(nil)
		return 0
	}
	notifyLog("COM object %v registered", clsid)
	return cookie
}

// buildActivator fills the method tables of the class factory and the object it hands out.
func buildActivator() {
	addRef := syscall.NewCallback(func(this uintptr) uintptr {
		return uintptr(atomic.AddInt32(&ptr[comObject](this).refs, 1))
	})
	release := syscall.NewCallback(func(this uintptr) uintptr {
		return uintptr(atomic.AddInt32(&ptr[comObject](this).refs, -1))
	})
	classFactoryTbl = [5]uintptr{
		comQuery(iidClassFactory), addRef, release,
		// CreateInstance.
		syscall.NewCallback(func(this, outer, riid, ppv uintptr) uintptr {
			*ptr[uintptr](ppv) = 0
			if outer != 0 {
				return classENoAggregate
			}
			id := *ptr[windows.GUID](riid)
			if id != iidUnknown && id != iidActivationCallback {
				return eNoInterface
			}
			atomic.AddInt32(&activatorObj.refs, 1)
			*ptr[uintptr](ppv) = uintptr(unsafe.Pointer(&activatorObj))
			return 0
		}),
		// LockServer.
		syscall.NewCallback(func(this, lock uintptr) uintptr { return 0 }),
	}
	activatorTbl = [4]uintptr{
		comQuery(iidActivationCallback), addRef, release,
		// Activate, on the main thread, as COM calls an object registered there.
		syscall.NewCallback(func(this, appID, args, data, count uintptr) uintptr {
			launch := ""
			if args != 0 {
				launch = windows.UTF16PtrToString(ptr[uint16](args))
			}
			notifyLog("activated through COM: %q", launch)
			heardThroughCOM(launch)
			return 0
		}),
	}
	classFactoryObj = comObject{vtbl: unsafe.Pointer(&classFactoryTbl), refs: 1}
	activatorObj = comObject{vtbl: unsafe.Pointer(&activatorTbl), refs: 1}
}
