package desktop

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/marrasen/gunim/driver"
)

var (
	chooseOle32        = windows.NewLazySystemDLL("ole32.dll")
	procChooseCoInit   = chooseOle32.NewProc("CoInitializeEx")
	procChooseCoUninit = chooseOle32.NewProc("CoUninitialize")
	procChooseCoCreate = chooseOle32.NewProc("CoCreateInstance")
	procChooseCoFree   = chooseOle32.NewProc("CoTaskMemFree")

	clsidFileOpenDialog = windows.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE,
		Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidFileOpenDialog = windows.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768,
		Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
)

// COM and file dialog constants.
const (
	coinitApartmentThreaded = 0x2
	coinitDisableOLE1DDE    = 0x4
	clsctxInprocServer      = 0x1
	fosPickFolders          = 0x20
	fosForceFileSystem      = 0x40
	fosAllowMultiSelect     = 0x200
	fosPathMustExist        = 0x800
	fosFileMustExist        = 0x1000
	sigdnFileSysPath        = 0x80058000
	hresultCancelled        = 0x800704C7
	rpcEChangedMode         = 0x80010106
)

// The methods called, by their place in each interface's table.
const (
	vtRelease        = 2
	vtShow           = 3
	vtSetFileTypes   = 4
	vtSetOptions     = 9
	vtGetOptions     = 10
	vtSetTitle       = 17
	vtGetResults     = 27
	vtArrayGetCount  = 7
	vtArrayGetItemAt = 8
	vtItemGetName    = 5
)

// filterSpec is COMDLG_FILTERSPEC.
type filterSpec struct {
	name, spec *uint16
}

// comCall calls method index of the COM object obj.
func comCall(obj unsafe.Pointer, index int, args ...uintptr) uint32 {
	table := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(table, uintptr(index)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return uint32(r)
}

func hrFailed(hr uint32) bool { return int32(hr) < 0 }

var _ driver.FileChooser = (*Window)(nil)

// ChooseFiles implements [driver.FileChooser] with the Windows file
// dialog, IFileOpenDialog.
func (w *Window) ChooseFiles(o driver.ChooseOptions) ([]string, error) {
	var hwnd uintptr
	if err := w.d.call(func() error {
		h, err := w.gw.GetWin32Window()
		hwnd = uintptr(h)
		return err
	}); err != nil {
		return nil, fmt.Errorf("desktop: finding the window for a file dialog: %w", err)
	}

	// COM belongs to a thread, and the dialog runs its own message loop on it.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := procChooseCoInit.Call(0, coinitApartmentThreaded|coinitDisableOLE1DDE)
	switch {
	case uint32(hr) == rpcEChangedMode:
	case hrFailed(uint32(hr)):
		return nil, fmt.Errorf("desktop: starting COM for a file dialog: HRESULT %#x", uint32(hr))
	default:
		defer func() { _, _, _ = procChooseCoUninit.Call() }()
	}

	var dlg unsafe.Pointer
	hr, _, _ = procChooseCoCreate.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidFileOpenDialog)), uintptr(unsafe.Pointer(&dlg)))
	if hrFailed(uint32(hr)) {
		return nil, fmt.Errorf("desktop: making a file dialog: HRESULT %#x", uint32(hr))
	}
	defer comCall(dlg, vtRelease)

	var opts uint32
	if hr := comCall(dlg, vtGetOptions, uintptr(unsafe.Pointer(&opts))); hrFailed(hr) {
		return nil, fmt.Errorf("desktop: reading the file dialog's options: HRESULT %#x", hr)
	}
	opts |= fosForceFileSystem | fosPathMustExist | fosFileMustExist
	if o.Multiple {
		opts |= fosAllowMultiSelect
	}
	if o.Folders {
		opts |= fosPickFolders
	}
	if hr := comCall(dlg, vtSetOptions, uintptr(opts)); hrFailed(hr) {
		return nil, fmt.Errorf("desktop: setting the file dialog's options: HRESULT %#x", hr)
	}
	if o.Title != "" {
		title, err := windows.UTF16PtrFromString(o.Title)
		if err != nil {
			return nil, fmt.Errorf("desktop: the file dialog's title: %w", err)
		}
		if hr := comCall(dlg, vtSetTitle, uintptr(unsafe.Pointer(title))); hrFailed(hr) {
			return nil, fmt.Errorf("desktop: setting the file dialog's title: HRESULT %#x", hr)
		}
	}
	if len(o.Filters) > 0 && !o.Folders {
		specs := make([]filterSpec, len(o.Filters))
		for i, f := range o.Filters {
			name, err := windows.UTF16PtrFromString(f.Name)
			if err != nil {
				return nil, fmt.Errorf("desktop: the file dialog's filter %q: %w", f.Name, err)
			}
			spec, err := windows.UTF16PtrFromString(strings.Join(f.Patterns, ";"))
			if err != nil {
				return nil, fmt.Errorf("desktop: the file dialog's filter %q: %w", f.Name, err)
			}
			specs[i] = filterSpec{name, spec}
		}
		if hr := comCall(dlg, vtSetFileTypes, uintptr(len(specs)), uintptr(unsafe.Pointer(&specs[0]))); hrFailed(hr) {
			return nil, fmt.Errorf("desktop: setting the file dialog's filters: HRESULT %#x", hr)
		}
	}

	switch hr := comCall(dlg, vtShow, hwnd); {
	case hr == hresultCancelled:
		return nil, nil
	case hrFailed(hr):
		return nil, fmt.Errorf("desktop: showing the file dialog: HRESULT %#x", hr)
	}
	return dialogResults(dlg)
}

// dialogResults returns the paths chosen in an IFileOpenDialog.
func dialogResults(dlg unsafe.Pointer) ([]string, error) {
	var items unsafe.Pointer
	if hr := comCall(dlg, vtGetResults, uintptr(unsafe.Pointer(&items))); hrFailed(hr) {
		return nil, fmt.Errorf("desktop: reading what the file dialog chose: HRESULT %#x", hr)
	}
	defer comCall(items, vtRelease)
	var n uint32
	if hr := comCall(items, vtArrayGetCount, uintptr(unsafe.Pointer(&n))); hrFailed(hr) {
		return nil, fmt.Errorf("desktop: counting what the file dialog chose: HRESULT %#x", hr)
	}
	out := make([]string, 0, n)
	for i := range int(n) {
		var item unsafe.Pointer
		if hr := comCall(items, vtArrayGetItemAt, uintptr(i), uintptr(unsafe.Pointer(&item))); hrFailed(hr) {
			return nil, fmt.Errorf("desktop: reading choice %d of the file dialog: HRESULT %#x", i, hr)
		}
		var name *uint16
		hr := comCall(item, vtItemGetName, sigdnFileSysPath, uintptr(unsafe.Pointer(&name)))
		comCall(item, vtRelease)
		if hrFailed(hr) {
			return nil, fmt.Errorf("desktop: reading the path of choice %d: HRESULT %#x", i, hr)
		}
		out = append(out, windows.UTF16PtrToString(name))
		_, _, _ = procChooseCoFree.Call(uintptr(unsafe.Pointer(name)))
	}
	return out, nil
}
