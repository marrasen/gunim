package desktop

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/internal/glfw"
	"github.com/marrasen/gunim/internal/toast"
)

// The tests here set the package's toaster and hooks, so none of them may run in parallel.

// stubClicks records the tray's clicks, with no click heard yet, and puts both back as the test ends.
func stubClicks(t *testing.T) *[]string {
	t.Helper()
	var heard []string
	wasClicked, wasHeard := trayClicked, heardIDs
	trayClicked = func(tag string) { heard = append(heard, tag) }
	heardIDs = nil
	t.Cleanup(func() { trayClicked, heardIDs = wasClicked, wasHeard })
	return &heard
}

// A click on a toast is heard once with its tag, though it come both by the Activated event and by the COM object,
// and a launch argument of anything else is heard as none.
func TestAClickOnAToastIsHeardOnceWithItsTag(t *testing.T) {
	heard := stubClicks(t)
	odd := "<b>&amp;\"'</b> 日本 🎉 %41"
	one, two := toast.Launch("ab-1", "chat:1"), toast.Launch("ab-2", odd)
	for _, launch := range []string{two, one, two, "", "plain argument", "gunim-tag:1:%"} {
		clicked(launch)
	}
	if want := []string{odd, "chat:1"}; len(*heard) != 2 || (*heard)[0] != want[0] || (*heard)[1] != want[1] {
		t.Fatalf("the clicks heard %q, want %q", *heard, want)
	}
}

// The COM object answers as Windows calls it: the class factory hands out the activation callback, and Activate
// hands the launch argument on.
func TestTheCOMObjectHandsTheLaunchArgumentOn(t *testing.T) {
	activatorOnce.Do(buildActivator)
	var got []string
	was := heardThroughCOM
	heardThroughCOM = func(launch string) { got = append(got, launch) }
	t.Cleanup(func() { heardThroughCOM = was })

	factoryObj := uintptr(unsafe.Pointer(&classFactoryObj))
	var out uintptr
	if hr := call(factoryObj, 0, uintptr(unsafe.Pointer(&iidClassFactory)), uintptr(unsafe.Pointer(&out))); hr != 0 || out != factoryObj {
		t.Fatalf("QueryInterface for IClassFactory: HRESULT %#x, object %#x", hr, out)
	}
	if hr := call(factoryObj, 0, uintptr(unsafe.Pointer(&iidActivationCallback)), uintptr(unsafe.Pointer(&out))); hr != eNoInterface || out != 0 {
		t.Fatalf("the class factory answered for INotificationActivationCallback: HRESULT %#x", hr)
	}
	if hr := call(factoryObj, 3, 1, uintptr(unsafe.Pointer(&iidActivationCallback)), uintptr(unsafe.Pointer(&out))); hr != classENoAggregate {
		t.Fatalf("CreateInstance with an outer object: HRESULT %#x, want CLASS_E_NOAGGREGATION", hr)
	}
	if hr := call(factoryObj, 3, 0, uintptr(unsafe.Pointer(&iidToastFactory)), uintptr(unsafe.Pointer(&out))); hr != eNoInterface {
		t.Fatalf("CreateInstance for another interface: HRESULT %#x, want E_NOINTERFACE", hr)
	}
	var act uintptr
	if hr := call(factoryObj, 3, 0, uintptr(unsafe.Pointer(&iidActivationCallback)), uintptr(unsafe.Pointer(&act))); hr != 0 || act == 0 {
		t.Fatalf("CreateInstance: HRESULT %#x, object %#x", hr, act)
	}
	launch := toast.Launch("ab-1", "chat <1> & \"2\"")
	args, _ := windows.UTF16PtrFromString(launch)
	app, _ := windows.UTF16PtrFromString("gunim.test")
	if hr := call(act, 3, uintptr(unsafe.Pointer(app)), uintptr(unsafe.Pointer(args)), 0, 0); hr != 0 {
		t.Fatalf("Activate: HRESULT %#x", hr)
	}
	if len(got) != 1 || got[0] != launch {
		t.Fatalf("Activate handed on %q, want [%q]", got, launch)
	}
}

// The toast's XML loads in Windows's own XML reader, whatever the tag, title and body hold.
func TestWindowsLoadsTheToastsXML(t *testing.T) {
	_, _, _ = procRoInitialize.Call(0)
	if doc, err := xmlDocument("<toast><unclosed></toast>"); err == nil {
		rtRelease(doc)
		t.Fatal("Windows loaded broken XML; the test cannot tell good from bad")
	} else if strings.Contains(err.Error(), "RoActivateInstance") || strings.Contains(err.Error(), "IXmlDocumentIO") {
		t.Skip("no WinRT XmlDocument here:", err)
	}
	for _, tag := range []string{"", "chat:1", `<toast launch="x">&amp;</toast>`, "nul\x00\x01\n\t", "日本 🎉 \uffff", "bad \xff utf-8"} {
		doc, err := xmlDocument(toast.XML(`Tom & "Jerry" <b>`, "line\none\x00", toast.Launch("ab-1", tag)))
		if err != nil {
			t.Fatalf("the toast for tag %q: %v", tag, err)
		}
		rtRelease(doc)
	}
}

// The AppUserModelID's entries name the program, its picture and its COM object, which names the program file.
func TestRegisterAppIDWritesItsEntries(t *testing.T) {
	var b [4]byte
	_, _ = rand.Read(b[:])
	appID := "gunim.test-" + hex.EncodeToString(b[:])
	clsid := guid(toast.ActivatorCLSID(appID))
	appKey, serverKey := `Software\Classes\AppUserModelId\`+appID, `Software\Classes\CLSID\`+clsid.String()
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, appKey)
		_ = registry.DeleteKey(registry.CURRENT_USER, serverKey+`\LocalServer32`)
		_ = registry.DeleteKey(registry.CURRENT_USER, serverKey)
	})
	if err := registerAppID(appID, "Test App", `C:\icons\notify.png`, `C:\Program Files\Test\test.exe`, clsid); err != nil {
		t.Fatal(err)
	}
	read := func(path, name string) string {
		t.Helper()
		k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = k.Close() }()
		v, _, err := k.GetStringValue(name)
		if err != nil {
			t.Fatalf("%s %s: %v", path, name, err)
		}
		return v
	}
	for _, c := range []struct{ path, name, want string }{
		{appKey, "DisplayName", "Test App"},
		{appKey, "IconUri", `C:\icons\notify.png`},
		{appKey, "CustomActivator", clsid.String()},
		{serverKey + `\LocalServer32`, "", `"C:\Program Files\Test\test.exe"`},
	} {
		if got := read(c.path, c.name); got != c.want {
			t.Errorf("%s %q = %q, want %q", c.path, c.name, got, c.want)
		}
	}
}

// fake is a COM object of the test's: a toast, a notifier, a toast factory or an XML document, by its method table.
type fake struct {
	vtbl     *[16]uintptr
	releases int
	// A toast's handlers added, by method, its tokens taken off, and the next token.
	added   map[uintptr]uintptr
	removed []int64
	token   int64
	// A notifier's toasts shown, the errors Show returns, first to last, and its setting.
	shown   []uintptr
	showErr []uintptr
	setting int32
	// A factory's toasts made.
	made []*fake
}

var (
	fakeOnce                                      sync.Once
	fakeToastTbl, fakeNotifierTbl, fakeFactoryTbl [16]uintptr
)

func fakeOf(this uintptr) *fake { return (*fake)(unsafe.Pointer(this)) } //nolint:govet // the test's own object

// fakeTables makes the method tables of the fakes, once.
func fakeTables() {
	fakeOnce.Do(func() {
		query := syscall.NewCallback(func(this, riid, ppv uintptr) uintptr {
			*(*uintptr)(unsafe.Pointer(ppv)) = this //nolint:govet // a pointer COM passed
			return 0
		})
		addRef := syscall.NewCallback(func(this uintptr) uintptr { return 1 })
		release := syscall.NewCallback(func(this uintptr) uintptr {
			fakeOf(this).releases++
			return 0
		})
		add := syscall.NewCallback(func(this, h, tok uintptr) uintptr {
			f := fakeOf(this)
			f.token++
			f.added[h] = h
			*(*int64)(unsafe.Pointer(tok)) = f.token //nolint:govet // a pointer COM passed
			return 0
		})
		remove := syscall.NewCallback(func(this, tok uintptr) uintptr {
			f := fakeOf(this)
			f.removed = append(f.removed, int64(tok))
			return 0
		})
		for _, tbl := range []*[16]uintptr{&fakeToastTbl, &fakeNotifierTbl, &fakeFactoryTbl} {
			tbl[0], tbl[1], tbl[2] = query, addRef, release
		}
		for _, i := range []int{toastAddActivated, toastAddDismissed, toastAddFailed} {
			fakeToastTbl[i] = add
		}
		for _, i := range []int{toastRemoveActivated, toastRemoveDismissed, toastRemoveFailed} {
			fakeToastTbl[i] = remove
		}
		// Show and get_Setting.
		fakeNotifierTbl[6] = syscall.NewCallback(func(this, toastObj uintptr) uintptr {
			f := fakeOf(this)
			if len(f.showErr) > 0 {
				hr := f.showErr[0]
				f.showErr = f.showErr[1:]
				if hr != 0 {
					return hr
				}
			}
			f.shown = append(f.shown, toastObj)
			return 0
		})
		fakeNotifierTbl[8] = syscall.NewCallback(func(this, out uintptr) uintptr {
			*(*int32)(unsafe.Pointer(out)) = fakeOf(this).setting //nolint:govet // a pointer COM passed
			return 0
		})
		// CreateToastNotification.
		fakeFactoryTbl[6] = syscall.NewCallback(func(this, doc, out uintptr) uintptr {
			f := fakeOf(this)
			tf := &fake{vtbl: &fakeToastTbl, added: map[uintptr]uintptr{}}
			f.made = append(f.made, tf)
			*(*uintptr)(unsafe.Pointer(out)) = uintptr(unsafe.Pointer(tf)) //nolint:govet // a pointer COM passed
			return 0
		})
	})
}

func addr(f *fake) uintptr { return uintptr(unsafe.Pointer(f)) }

// stubToaster puts up a toaster of fakes, with the tray up and its balloons and gone calls recorded, and puts
// everything back as the test ends.
func stubToaster(t *testing.T) (tt *toaster, notifier, factory *fake, balloons *[]string, gone *int) {
	t.Helper()
	fakeTables()
	notifier = &fake{vtbl: &fakeNotifierTbl}
	factory = &fake{vtbl: &fakeFactoryTbl}
	doc := &fake{vtbl: &fakeFactoryTbl}
	tt = &toaster{appID: "gunim.test", run: "ab", notifier: addr(notifier), factory: addr(factory)}
	var b []string
	var g int
	wasUp, wasBalloon, wasGone, wasLoad, wasToasts, wasTray := trayUp, balloon, trayGone, loadXML, toasts, toastTray
	trayUp = func() bool { return true }
	balloon = func(title, body, tag string) error { b = append(b, tag); return nil }
	trayGone = func() { g++ }
	loadXML = func(string) (uintptr, error) { return addr(doc), nil }
	toasts = tt
	t.Cleanup(func() {
		trayUp, balloon, trayGone, loadXML, toasts, toastTray = wasUp, wasBalloon, wasGone, wasLoad, wasToasts, wasTray
		runtime.KeepAlive(doc)
	})
	return tt, notifier, factory, &b, &g
}

// A toast that will not show has its message shown as a balloon, and the next message is a toast again.
func TestAToastThatFailsShowsItsMessageAsABalloon(t *testing.T) {
	tt, notifier, factory, balloons, _ := stubToaster(t)
	notifier.showErr = []uintptr{0x80004005}
	for _, tag := range []string{"one", "two"} {
		if err := notifyOnMain(nil, "title", "body", tag); err != nil {
			t.Fatal(err)
		}
	}
	if len(*balloons) != 1 || (*balloons)[0] != "one" {
		t.Fatalf("balloons shown for %q, want [one]", *balloons)
	}
	if toasts != tt || len(notifier.shown) != 1 || len(tt.shown) != 1 || tt.shown[0].tag != "two" {
		t.Fatalf("after one failed toast: toaster kept %v, %d toasts shown", toasts == tt, len(notifier.shown))
	}
	// The failed toast's handlers came off, and it was let go.
	if first := factory.made[0]; len(first.removed) != 3 || first.releases == 0 {
		t.Fatalf("the failed toast had %d handlers taken off and %d releases", len(first.removed), first.releases)
	}
	// A toast Windows reports failed later shows as a balloon too, and the toaster stays.
	toastFailed(tt.shown[0].unk)
	if len(*balloons) != 2 || (*balloons)[1] != "two" || toasts != tt {
		t.Fatalf("after a Failed event, balloons %q, toaster kept %v", *balloons, toasts == tt)
	}
}

// The toaster keeps the last toastKept toasts: the oldest has its handlers taken off and is let go, and every toast
// shares the same three handlers.
func TestTheToasterLetsGoOfTheOldestToast(t *testing.T) {
	tt, _, factory, _, _ := stubToaster(t)
	for i := range toastKept + 1 {
		if err := tt.show("title", "body", strconv.Itoa(i)); err != nil {
			t.Fatal(err)
		}
	}
	if len(tt.shown) != toastKept || tt.shown[0].tag != "1" {
		t.Fatalf("the toaster keeps %d toasts, the oldest %q", len(tt.shown), tt.shown[0].tag)
	}
	first, last := factory.made[0], factory.made[toastKept]
	if len(first.removed) != 3 || first.releases == 0 {
		t.Fatalf("the oldest toast had %d handlers taken off and %d releases", len(first.removed), first.releases)
	}
	for _, tok := range first.removed {
		if tok < 1 || tok > 3 {
			t.Fatalf("the oldest toast's handlers came off with tokens %v", first.removed)
		}
	}
	if len(last.removed) != 0 || len(first.added) != 3 {
		t.Fatalf("the newest toast lost handlers %v; the oldest had %d", last.removed, len(first.added))
	}
	for h := range first.added {
		if _, ok := last.added[h]; !ok {
			t.Fatal("two toasts were given different handlers")
		}
	}
}

// The last toast leaving runs OnNotifyGone, unless it was clicked; an older one leaving does nothing.
func TestTheLastToastLeavingRunsOnNotifyGone(t *testing.T) {
	tt, _, _, _, gone := stubToaster(t)
	stubClicks(t)
	for _, tag := range []string{"one", "two"} {
		if err := tt.show("title", "body", tag); err != nil {
			t.Fatal(err)
		}
	}
	dismissed(tt.shown[0].unk)
	dismissed(0)
	if *gone != 0 {
		t.Fatalf("an older toast leaving ran OnNotifyGone %d times", *gone)
	}
	dismissed(tt.shown[1].unk)
	if *gone != 1 {
		t.Fatalf("the last toast leaving ran OnNotifyGone %d times, want once", *gone)
	}
	if err := tt.show("title", "body", "three"); err != nil {
		t.Fatal(err)
	}
	clicked(toast.Launch(tt.last, "three"))
	dismissed(tt.shown[2].unk)
	if *gone != 1 {
		t.Fatal("the last toast, clicked, ran OnNotifyGone as it left")
	}
}

// Where the user turned the toasts off, a message whose toast fails shows nowhere, in step with TrayNotifyWay.
func TestAFailedToastShowsNoBalloonWhereToastsAreOff(t *testing.T) {
	tt, notifier, _, balloons, _ := stubToaster(t)
	if err := tt.show("title", "body", "shown"); err != nil {
		t.Fatal(err)
	}
	notifier.setting = 2 // DisabledForUser
	notifier.showErr = []uintptr{0x803E0111}
	if err := notifyOnMain(nil, "title", "body", "blocked"); err != nil {
		t.Fatal(err)
	}
	toastFailed(tt.shown[0].unk)
	if len(*balloons) != 0 || notifyWay() != driver.NotifyNone {
		t.Fatalf("with toasts off, balloons showed for %q and the way is %v", *balloons, notifyWay())
	}
}

// Once a balloon shows, it is the message shown last, and an older toast leaving does not run OnNotifyGone.
func TestAnOlderToastLeavingAfterABalloonIsNoNews(t *testing.T) {
	tt, notifier, _, balloons, gone := stubToaster(t)
	stubClicks(t)
	if err := tt.show("title", "body", "older"); err != nil {
		t.Fatal(err)
	}
	notifier.showErr = []uintptr{0x80004005}
	if err := notifyOnMain(nil, "title", "body", "balloon"); err != nil {
		t.Fatal(err)
	}
	dismissed(tt.shown[0].unk)
	if len(*balloons) != 1 || *gone != 0 {
		t.Fatalf("%d balloons showed, and the older toast leaving ran OnNotifyGone %d times", len(*balloons), *gone)
	}
}

// A tray whose icon the taskbar did not take yet is kept for the toasts, as the icon goes up later.
func TestATrayIsKeptForToastsWhereItsIconIsNotUpYet(t *testing.T) {
	stubToaster(t)
	was := setTrayIcon
	setTrayIcon = func(*glfw.TrayIcon) error { return windows.ERROR_TIMEOUT }
	t.Cleanup(func() { setTrayIcon = was })
	tray := &driver.Tray{AppID: "gunim.test", Icon: []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}}
	if err := (&Driver{}).showTray(&glfw.TrayIcon{}, tray); !errors.Is(err, windows.ERROR_TIMEOUT) {
		t.Fatalf("showTray returned %v, want the taskbar's error", err)
	}
	if toastTray != tray {
		t.Fatal("the tray was let go of with its icon not up yet")
	}
}

// TrayNotifyWay readies WinRT on the thread before it asks for anything of it, as at a program's first call.
func TestNotifyWayReadiesWinRT(t *testing.T) {
	stubToaster(t)
	toasts, toastTray = nil, &driver.Tray{AppID: "gunim.test-fresh"}
	hr := make(chan uint32)
	go func() {
		// A thread with no apartment yet, ended with the goroutine.
		runtime.LockOSThread()
		notifyWay()
		r, _, _ := procRoInitialize.Call(0)
		hr <- uint32(r)
	}()
	if r := <-hr; r != 1 {
		t.Fatalf("RoInitialize after notifyWay returned %#x, want S_FALSE: the thread was not readied", r)
	}
}

// The toasts' handlers post to the driver that opened the toasts last.
func TestTheHandlersPostToTheDriverOpenNow(t *testing.T) {
	stubToaster(t)
	was := activatorDriver.Load()
	t.Cleanup(func() { activatorDriver.Store(was) })
	first, second := &Driver{}, &Driver{}
	activatorDriver.Store(first)
	hs := handlers()
	activatorDriver.Store(second)
	sender := &fake{vtbl: &fakeToastTbl}
	for _, h := range hs[1:] {
		call(uintptr(unsafe.Pointer(h)), 3, addr(sender), 0)
	}
	runtime.KeepAlive(sender)
	if len(first.tasks) != 0 || len(second.tasks) != 2 {
		t.Fatalf("the first driver was posted %d events and the second %d, want 0 and 2", len(first.tasks), len(second.tasks))
	}
}

// TrayNotifyWay says none with no icon up or the toasts turned off, exact with toasts, and last where toasts could
// not be set up.
func TestNotifyWaySaysHowTheNextMessageShows(t *testing.T) {
	_, notifier, _, _, _ := stubToaster(t)
	if w := notifyWay(); w != driver.NotifyExact {
		t.Fatalf("with toasts the way is %v, want exact", w)
	}
	notifier.setting = 2 // DisabledForUser
	if w := notifyWay(); w != driver.NotifyNone {
		t.Fatalf("with toasts turned off the way is %v, want none", w)
	}
	toasts = nil
	toastTray = &driver.Tray{AppID: "gunim.test-failed"}
	wasFailed := toastsFailed
	toastsFailed = "gunim.test-failed"
	t.Cleanup(func() { toastsFailed = wasFailed })
	if w := notifyWay(); w != driver.NotifyLast {
		t.Fatalf("where toasts failed the way is %v, want last", w)
	}
	trayUp = func() bool { return false }
	if w := notifyWay(); w != driver.NotifyNone {
		t.Fatalf("with no icon up the way is %v, want none", w)
	}
}
