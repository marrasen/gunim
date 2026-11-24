package desktop

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/marrasen/gunim/internal/toast"
)

// The tests here set the package's toaster and hooks, so none of them may run in parallel.

// stubClicks puts a toaster up with the tray's click recorded, and takes both away as the test ends.
func stubClicks(t *testing.T) *[]string {
	t.Helper()
	var heard []string
	wasClicked, wasToasts := trayClicked, toasts
	trayClicked = func(tag string) { heard = append(heard, tag) }
	toasts = &toaster{appID: "gunim.test"}
	t.Cleanup(func() { trayClicked, toasts = wasClicked, wasToasts })
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
	toasts = nil
	clicked(toast.Launch("ab-3", "late"))
	if len(*heard) != 2 {
		t.Fatalf("with no toaster, a click heard %q", *heard)
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

	call := func(obj uintptr, i int, args ...uintptr) uintptr {
		vtbl := *(*unsafe.Pointer)(unsafe.Pointer(obj)) //nolint:govet // a COM object's pointer
		fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(i)*unsafe.Sizeof(uintptr(0))))
		hr, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
		return hr
	}
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
