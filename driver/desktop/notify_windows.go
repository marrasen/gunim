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
//     toastKept of them. Three handlers serve every toast, made once and kept for the life of the process, as
//     Windows may hold a handler a while after it is taken off.
//   - A COM object of the driver's own, an INotificationActivationCallback, registered for this process with
//     CoRegisterClassObject and kept until it ends. Windows calls it for a click in the notification centre, on a
//     toast of any age.
//
// One click may come both ways, so each toast has an ID, in its launch argument with the tag, and a click is heard
// once per ID.
//
// An unpackaged program shows toasts under an AppUserModelID it registers itself, under the current user's
// Software\Classes\AppUserModelId\<AppID>: the name and picture its toasts show, and the class ID of its COM
// object. The class ID's LocalServer32 names the program file, so a click on a toast after the program ended starts
// it again, with the argument -Embedding, which init takes out of os.Args before the program parses its flags. The
// program registers at its first message, so one that shows none writes nothing, or as its icon goes up where it
// registered on an earlier run or was started by a click, to hear clicks on toasts already shown.
//
// A message whose toast fails shows as a tray balloon, and the next tries a toast again. Where toasts cannot be set
// up at all, every message shows as a balloon: a balloon's click names no message.
//
// GUNIM_DEBUG_NOTIFY=1 logs each step to standard error.

var (
	ole32                         = windows.NewLazySystemDLL("ole32.dll")
	procCoRegisterClassObject     = ole32.NewProc("CoRegisterClassObject")
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
)

const (
	// toastKept is how many toast objects the driver keeps, for their events, and how many clicks it remembers.
	toastKept = 64
	// clsctxLocalServer and regclsMultipleUse are CLSCTX_LOCAL_SERVER and REGCLS_MULTIPLEUSE.
	clsctxLocalServer = 4
	regclsMultipleUse = 1
	eNoInterface      = 0x80004002
	classENoAggregate = 0x80040110
	// The IToastNotification methods: add_ and remove_ for Activated, Dismissed and Failed.
	toastAddActivated, toastRemoveActivated = 11, 12
	toastAddDismissed, toastRemoveDismissed = 9, 10
	toastAddFailed, toastRemoveFailed       = 13, 14
)

// startedByCOM says COM started the program, as for a click on one of its toasts after it ended.
var startedByCOM bool

func init() {
	os.Args, startedByCOM = toast.TakeEmbedding(os.Args)
}

func notifyLog(format string, args ...any) {
	if notifyDebug {
		fmt.Fprintf(os.Stderr, "gunim notify: "+format+"\n", args...)
	}
}

// The calls into glfw and WinRT that a test puts its own in place of.
var (
	trayUp      = glfw.TrayUp
	trayClicked = glfw.TrayNotifyClicked
	trayGone    = glfw.TrayNotifyGone
	balloon     = glfw.TrayNotify
	loadXML     = xmlDocument
	setTrayIcon = glfw.SetTrayIcon
)

// toaster shows the tray's messages as toasts. It lives on the main thread, but for the handlers' Invoke, which
// runs on a thread of the system's and posts to the main thread.
type toaster struct {
	appID string
	// notifier is the IToastNotifier for appID, and factory the IToastNotificationFactory.
	notifier, factory uintptr
	// run starts each toast's ID, apart for each run of the program, and next numbers the toasts.
	run  string
	next uint64
	// shown are the toasts kept, oldest first, and last the ID of the one shown last.
	shown []*shownToast
	last  string
}

// shownToast is a toast shown: its message, its object and that object's identity, its IUnknown, and the tokens of
// its handlers, where added.
type shownToast struct {
	id, title, body, tag string
	toast, unk           uintptr
	tokens               [3]int64
	added                [3]bool
}

// The toaster's state, on the main thread. toasts is the toaster, or nil where none is up yet or toasts cannot be
// set up. toastTray is the tray shown, for the toaster to be made from. toastsFailed is the AppID toasts could not
// be set up for, so they are tried again only for another. heardIDs are the IDs of the toasts whose click was
// heard, oldest first, so a click is heard once.
var (
	toasts       *toaster
	toastTray    *driver.Tray
	toastsFailed string
	heardIDs     []string
)

// setToastTray keeps the tray t for the toasts, as its icon goes up or goes. Where the AppID changes, the toaster
// goes. Where the AppID was registered on an earlier run, or a click on a toast started the program, the toaster
// comes up at once, to hear clicks on toasts already shown. It runs on the main thread.
func setToastTray(d *Driver, t *driver.Tray) {
	if t == nil || len(t.Icon) == 0 {
		toastTray = nil
		return
	}
	toastTray = t
	appID := toastAppID(t)
	if toasts != nil && toasts.appID != appID {
		closeToasts()
	}
	if toasts == nil && (startedByCOM || appIDRegistered(appID)) {
		readyToasts(d)
	}
}

// toastAppID is the AppUserModelID of t's toasts.
func toastAppID(t *driver.Tray) string {
	exe, _ := os.Executable()
	return toast.AppID(t.AppID, exe)
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

// readyToasts makes the toaster for the tray shown, where none is up and toasts were not found out of reach for its
// AppID, and reports whether one is up. It runs on the main thread.
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

// closeToasts lets go of the toaster. The COM object stays registered. It runs on the main thread.
func closeToasts() {
	if toasts != nil {
		toasts.close()
		toasts = nil
	}
}

// notifyOnMain shows a message, as a toast where it can and as a balloon where it cannot. It runs on the main
// thread.
func notifyOnMain(d *Driver, title, body, tag string) error {
	if trayUp() && readyToasts(d) {
		err := toasts.show(title, body, tag)
		if err == nil {
			return nil
		}
		notifyLog("toast: %v", err)
		return toasts.fallBack(title, body, tag)
	}
	return balloon(title, body, tag)
}

// fallBack shows a message whose toast failed as a balloon, which is then the message shown last. Where the user or a
// policy turned the toasts off, the message shows nowhere, as a toast of theirs would.
func (tt *toaster) fallBack(title, body, tag string) error {
	if !tt.allowed() {
		notifyLog("toasts are off; the message shows nowhere")
		return nil
	}
	if err := balloon(title, body, tag); err != nil {
		return err
	}
	tt.last = ""
	return nil
}

// notifyWay says how the next message shows, with nothing written: a toast where the toaster is up and the user
// lets it show, none where the user or a policy turned its toasts off, a balloon where toasts could not be set up,
// and a toast where Windows has them. It runs on the main thread.
func notifyWay() driver.NotifyWay {
	switch {
	case !trayUp():
		return driver.NotifyNone
	case toasts != nil:
		if !toasts.allowed() {
			return driver.NotifyNone
		}
		return driver.NotifyExact
	case toastTray == nil || toastsFailed == toastAppID(toastTray):
		return driver.NotifyLast
	}
	roInitialize()
	statics, err := factory("Windows.UI.Notifications.ToastNotificationManager", &iidToastManagerStatics)
	if err != nil {
		return driver.NotifyLast
	}
	rtRelease(statics)
	return driver.NotifyExact
}

// openToaster registers appID and readies its notifier.
func openToaster(d *Driver, t *driver.Tray, appID, exe string) (*toaster, error) {
	roInitialize()
	name := toast.Clean(t.AppName)
	if name == "" {
		name = toast.ProgramName(exe)
	}
	clsid := guid(toast.ActivatorCLSID(appID))
	if err := registerAppID(appID, name, iconFile(appID, notifyIcon(t)), exe, clsid); err != nil {
		return nil, fmt.Errorf("registering the AppUserModelID: %w", err)
	}
	statics, err := factory("Windows.UI.Notifications.ToastNotificationManager", &iidToastManagerStatics)
	if err != nil {
		return nil, err
	}
	defer rtRelease(statics)
	tt := &toaster{appID: appID, run: runID()}
	h, del := hstring(appID)
	defer del()
	// CreateToastNotifierWithId.
	if err := rtCall(statics, 7, h, uintptr(unsafe.Pointer(&tt.notifier))); err != nil {
		return nil, fmt.Errorf("CreateToastNotifierWithId: %w", err)
	}
	// get_Setting fails for an AppUserModelID Windows does not know.
	if _, err := tt.setting(); err != nil {
		tt.close()
		return nil, fmt.Errorf("get_Setting: %w", err)
	}
	if tt.factory, err = factory("Windows.UI.Notifications.ToastNotification", &iidToastFactory); err != nil {
		tt.close()
		return nil, err
	}
	registerActivator(d, clsid)
	return tt, nil
}

// roInitialize readies WinRT on the thread, single-threaded, as OLE has it.
func roInitialize() {
	hr, _, _ := procRoInitialize.Call(0)
	notifyLog("RoInitialize: HRESULT %#x", uint32(hr))
}

// setting is the notifier's NotificationSetting: 0 where its toasts show, and above where the user, a policy or
// the manifest turned them off.
func (tt *toaster) setting() (int32, error) {
	var setting int32
	err := rtCall(tt.notifier, 8, uintptr(unsafe.Pointer(&setting)))
	return setting, err
}

// allowed reports whether the toasts show, as far as the notifier says.
func (tt *toaster) allowed() bool {
	s, err := tt.setting()
	if err != nil {
		return true
	}
	notifyLog("the notifier's setting is %d (0 is on)", s)
	return s == 0
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

// The three handlers every toast shares, made once and kept for the life of the process.
var (
	toastHandlersOnce sync.Once
	toastHandlers     [3]*delegate
	toastHandlerTbls  [3]delegateVtbl
)

// handlers returns the three handlers: Activated, Dismissed and Failed. Activated reads the click's launch
// argument; the others find their toast by its sender's identity. They post to the driver open now.
func handlers() [3]*delegate {
	toastHandlersOnce.Do(func() {
		toastHandlerTbls = [3]delegateVtbl{delegateTable("toast activated"), delegateTable("toast dismissed"),
			delegateTable("toast failed")}
		toastHandlers = [3]*delegate{
			{vtbl: &toastHandlerTbls[0], refs: 1, iid: iidToastActivatedHandler, invoke: func(args uintptr) {
				if launch, ok := activatedLaunch(args); ok {
					postOpen(func() { clicked(launch) })
				}
			}},
			{vtbl: &toastHandlerTbls[1], refs: 1, iid: iidToastDismissedHandler, from: func(sender, _ uintptr) {
				unk := identity(sender)
				postOpen(func() { dismissed(unk) })
			}},
			{vtbl: &toastHandlerTbls[2], refs: 1, iid: iidToastFailedHandler, from: func(sender, _ uintptr) {
				unk := identity(sender)
				postOpen(func() { toastFailed(unk) })
			}},
		}
	})
	return toastHandlers
}

// identity is a COM object's IUnknown, which names it, or 0.
func identity(obj uintptr) uintptr {
	if obj == 0 {
		return 0
	}
	unk, err := rtQuery(obj, &iidUnknown)
	if err != nil {
		return 0
	}
	rtRelease(unk)
	return unk
}

// show shows a toast with title, body and tag. An error is this message's alone: it shows as a balloon.
func (tt *toaster) show(title, body, tag string) error {
	tt.next++
	st := &shownToast{id: fmt.Sprintf("%s-%x", tt.run, tt.next), title: title, body: body, tag: tag}
	doc, err := loadXML(toast.XML(title, body, toast.Launch(st.id, tag)))
	if err != nil {
		return err
	}
	defer rtRelease(doc)
	// CreateToastNotification.
	if err := rtCall(tt.factory, 6, doc, uintptr(unsafe.Pointer(&st.toast))); err != nil {
		return fmt.Errorf("CreateToastNotification: %w", err)
	}
	st.unk = identity(st.toast)
	hs := handlers()
	for i, at := range []int{toastAddActivated, toastAddDismissed, toastAddFailed} {
		if err := rtCall(st.toast, at, uintptr(unsafe.Pointer(hs[i])), uintptr(unsafe.Pointer(&st.tokens[i]))); err != nil {
			notifyLog("adding handler %d: %v", at, err)
			continue
		}
		st.added[i] = true
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

// drop takes the toast's handlers off and lets go of it. It stays in the notification centre, where a click on it
// reaches the COM object.
func (st *shownToast) drop() {
	for i, at := range []int{toastRemoveActivated, toastRemoveDismissed, toastRemoveFailed} {
		if st.added[i] {
			_ = rtCall(st.toast, at, uintptr(st.tokens[i]))
			st.added[i] = false
		}
	}
	rtRelease(st.toast)
	st.toast = 0
}

// activatedLaunch reads the launch argument of an Activated event's arguments, on a thread of the system's.
func activatedLaunch(args uintptr) (string, bool) {
	ea, err := rtQuery(args, &iidToastActivatedArgs)
	if err != nil {
		notifyLog("IToastActivatedEventArgs: %v", err)
		return "", false
	}
	defer rtRelease(ea)
	var h uintptr
	// get_Arguments.
	if err := rtCall(ea, 6, uintptr(unsafe.Pointer(&h))); err != nil {
		notifyLog("get_Arguments: %v", err)
		return "", false
	}
	launch := hstringText(h)
	_, _, _ = procWindowsDeleteString.Call(h)
	notifyLog("activated: %q", launch)
	return launch, true
}

// byIdentity is the toast kept whose IUnknown is unk, or nil.
func (tt *toaster) byIdentity(unk uintptr) *shownToast {
	if unk == 0 {
		return nil
	}
	for _, st := range tt.shown {
		if st.unk == unk {
			return st
		}
	}
	return nil
}

// dismissed hears the toast unk leave the screen: where it is the one shown last, and was not clicked,
// OnNotifyGone runs. It runs on the main thread.
func dismissed(unk uintptr) {
	if tt := toasts; tt != nil {
		if st := tt.byIdentity(unk); st != nil && st.id == tt.last && !heard(st.id) {
			notifyLog("dismissed %s", st.id)
			trayGone()
		}
	}
}

// toastFailed hears that the toast unk could not show: its message shows as a balloon, and the next tries a toast
// again. It runs on the main thread.
func toastFailed(unk uintptr) {
	if tt := toasts; tt != nil {
		if st := tt.byIdentity(unk); st != nil {
			notifyLog("toast %s failed", st.id)
			_ = tt.fallBack(st.title, st.body, st.tag)
		}
	}
}

// heard reports whether a click on the toast id was heard.
func heard(id string) bool {
	for _, h := range heardIDs {
		if h == id {
			return true
		}
	}
	return false
}

// clicked hears a click on a toast with the launch argument launch, once for each toast. It runs on the main
// thread.
func clicked(launch string) {
	id, tag, ok := toast.ParseLaunch(launch)
	if !ok {
		notifyLog("a click with %q, which is no toast of gunim's", launch)
		return
	}
	if heard(id) {
		return
	}
	heardIDs = append(heardIDs, id)
	if len(heardIDs) > toastKept {
		heardIDs = heardIDs[1:]
	}
	trayClicked(tag)
}

// close lets go of the toasts and the notifier. The COM object stays registered.
func (tt *toaster) close() {
	for _, st := range tt.shown {
		st.drop()
	}
	tt.shown = nil
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
// the process runs, as COM holds pointers to them, and each class ID is registered once and stays so.

type comObject struct {
	vtbl unsafe.Pointer
	refs int32
}

var (
	activatorOnce   sync.Once
	activatorDriver atomic.Pointer[Driver]
	activatorsUp    = map[windows.GUID]bool{}
	classFactoryObj comObject
	activatorObj    comObject
	classFactoryTbl [5]uintptr
	activatorTbl    [4]uintptr
)

// heardThroughCOM takes a click the COM object heard to the main thread; a test puts its own in its place.
var heardThroughCOM = func(launch string) { postOpen(func() { clicked(launch) }) }

// postOpen posts f to the main thread of the driver that opened the toasts last, as a process may open more than one
// in turn.
func postOpen(f func()) {
	if d := activatorDriver.Load(); d != nil {
		d.post(f)
	}
}

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

// registerActivator registers the process's COM object for clsid, once; it stays registered until the process
// ends. It runs on the main thread.
func registerActivator(d *Driver, clsid windows.GUID) {
	activatorOnce.Do(buildActivator)
	activatorDriver.Store(d)
	if activatorsUp[clsid] {
		return
	}
	var cookie uint32
	hr, _, _ := procCoRegisterClassObject.Call(uintptr(unsafe.Pointer(&clsid)), uintptr(unsafe.Pointer(&classFactoryObj)),
		clsctxLocalServer, regclsMultipleUse, uintptr(unsafe.Pointer(&cookie)))
	if int32(hr) < 0 {
		notifyLog("CoRegisterClassObject: HRESULT %#x", uint32(hr))
		return
	}
	activatorsUp[clsid] = true
	notifyLog("COM object %v registered", clsid)
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
