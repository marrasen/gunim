//go:build windows

package install

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Windows installs, paths ignore letter case, and a running program
// cannot be written over, only moved.
const (
	supported  = true
	caseless   = true
	movesAside = true
	exeSuffix  = ".exe"
)

// Where Windows keeps a user's installed programs, the programs it
// starts with the user, and the user's kinds of file.
const (
	uninstallKeys = `Software\Microsoft\Windows\CurrentVersion\Uninstall\`
	runKey        = `Software\Microsoft\Windows\CurrentVersion\Run`
	classesKey    = `Software\Classes\`
)

// defaultDir is %LOCALAPPDATA%\Programs\<ID>.
func defaultDir(a *App) (string, error) {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		return "", fmt.Errorf("find %%LOCALAPPDATA%%: %w", err)
	}
	return filepath.Join(dir, "Programs", a.id()), nil
}

// shortcutName is the name of the program's shortcuts: its name, as a
// file can take it.
func shortcutName(a *App) string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < ' ' {
			return '-'
		}
		return r
	}, a.Name)
	return strings.TrimRight(name, ". ") + ".lnk"
}

// startMenu is the Start menu shortcut, and desktop the one on the
// desktop.
func startMenu(a *App) (string, error) {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, shortcutName(a)), nil
}

func desktop(a *App) (string, error) {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, shortcutName(a)), nil
}

// register adds the program to Windows: the Start menu shortcut, the
// entry under Installed apps, the kinds of file it opens, and, as
// picked, the shortcut on the desktop and the start with the user. What
// was not picked is taken away, as after an install that had it.
func register(a *App, in Installation) error {
	menu, err := startMenu(a)
	if err != nil {
		return err
	}
	if err := shortcut(menu, in.Exe, a.Description); err != nil {
		return fmt.Errorf("make the Start menu shortcut: %w", err)
	}
	if desk, err := desktop(a); err == nil {
		if in.Chose(PickDesktop) {
			if err := shortcut(desk, in.Exe, a.Description); err != nil {
				return fmt.Errorf("make the desktop shortcut: %w", err)
			}
		} else {
			_ = os.Remove(desk)
		}
	}
	if err := listInstalled(a, in); err != nil {
		return fmt.Errorf("list %s under Installed apps: %w", a.Name, err)
	}
	args := []string(nil)
	if a.Autostart != nil {
		args = a.Autostart.Args
	}
	if err := setAutostart(a, in.Exe, args, a.Autostart != nil && in.Chose(PickAutostart)); err != nil {
		return err
	}
	changed := false
	for _, t := range a.FileTypes {
		var err error
		if in.Chose(FileTypeKey(t)) {
			err = claimType(a, t, in.Exe)
		} else {
			err = dropType(a, t)
		}
		if err != nil {
			return fmt.Errorf("register %s files: %w", strings.Join(t.Exts, " "), err)
		}
		changed = true
	}
	if changed {
		assocChanged()
	}
	return nil
}

// listInstalled writes the program's entry under Installed apps.
func listInstalled(a *App, in Installation) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallKeys+a.id(), registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	size := uint32(0)
	_ = filepath.WalkDir(in.Dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				size += uint32(fi.Size() / 1024)
			}
		}
		return nil
	})
	values := []struct{ name, value string }{
		{"DisplayName", a.Name},
		{"DisplayVersion", strings.TrimPrefix(a.Version, "v")},
		{"DisplayIcon", in.Exe},
		{"InstallLocation", in.Dir},
		{"InstallDate", time.Now().Format("20060102")},
		{"UninstallString", `"` + in.Exe + `" -uninstall`},
		{"QuietUninstallString", `"` + in.Exe + `" -uninstall -quiet`},
	}
	if a.Publisher != "" {
		values = append(values, struct{ name, value string }{"Publisher", a.Publisher})
	}
	if a.URL != "" {
		values = append(values, struct{ name, value string }{"URLInfoAbout", a.URL})
	}
	for _, v := range values {
		if err := k.SetStringValue(v.name, v.value); err != nil {
			return err
		}
	}
	for _, v := range []struct {
		name  string
		value uint32
	}{{"NoModify", 1}, {"NoRepair", 1}, {"EstimatedSize", size}} {
		if err := k.SetDWordValue(v.name, v.value); err != nil {
			return err
		}
	}
	return nil
}

// progID is the name Windows knows the program's handling of t by.
func progID(a *App, t FileType) string {
	return a.id() + "." + strings.ToLower(strings.TrimPrefix(t.Exts[0], "."))
}

// claimType registers the program as one that opens files of kind t,
// and as the one that does, when t is Default and no other program has
// claimed it.
func claimType(a *App, t FileType, exe string) error {
	id := progID(a, t)
	name := t.Name
	if name == "" {
		name = a.Name + " file"
	}
	if err := setDefault(classesKey+id, name); err != nil {
		return err
	}
	if err := setDefault(classesKey+id+`\DefaultIcon`, `"`+exe+`",0`); err != nil {
		return err
	}
	if err := setDefault(classesKey+id+`\shell\open\command`, `"`+exe+`" "%1"`); err != nil {
		return err
	}
	app := classesKey + `Applications\` + filepath.Base(exe)
	if err := setDefault(app+`\shell\open\command`, `"`+exe+`" "%1"`); err != nil {
		return err
	}
	for _, ext := range t.Exts {
		ext = strings.ToLower(ext)
		k, _, err := registry.CreateKey(registry.CURRENT_USER, classesKey+ext+`\OpenWithProgids`, registry.SET_VALUE)
		if err != nil {
			return err
		}
		err = k.SetStringValue(id, "")
		_ = k.Close()
		if err != nil {
			return err
		}
		if s, _, err := registry.CreateKey(registry.CURRENT_USER, app+`\SupportedTypes`, registry.SET_VALUE); err == nil {
			_ = s.SetStringValue(ext, "")
			_ = s.Close()
		}
		if t.Default {
			// Claimed by another program for this user or for the
			// machine, as the merged view shows it.
			now := mergedDefault(ext)
			if now == "" || now == id {
				if err := setDefault(classesKey+ext, id); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// dropType takes away what claimType adds.
func dropType(a *App, t FileType) error {
	id := progID(a, t)
	for _, ext := range t.Exts {
		ext = strings.ToLower(ext)
		if k, err := registry.OpenKey(registry.CURRENT_USER, classesKey+ext+`\OpenWithProgids`, registry.SET_VALUE); err == nil {
			_ = k.DeleteValue(id)
			_ = k.Close()
		}
		if getDefault(classesKey+ext) == id {
			if k, err := registry.OpenKey(registry.CURRENT_USER, classesKey+ext, registry.SET_VALUE); err == nil {
				_ = k.DeleteValue("")
				_ = k.Close()
			}
		}
	}
	return deleteTree(classesKey + id)
}

// setDefault sets the default value of the user's key path, making it.
func setDefault(path, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	return k.SetStringValue("", value)
}

// getDefault is the default value of the user's key path, or "".
func getDefault(path string) string {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	v, _, _ := k.GetStringValue("")
	return v
}

// mergedDefault is the default value of key path under
// HKEY_CLASSES_ROOT, the user's classes over the machine's, or "".
func mergedDefault(path string) string {
	k, err := registry.OpenKey(registry.CLASSES_ROOT, path, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	v, _, _ := k.GetStringValue("")
	return v
}

// deleteTree deletes the user's key path and every key under it.
func deleteTree(path string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	names, err := k.ReadSubKeyNames(-1)
	_ = k.Close()
	if err != nil {
		return err
	}
	for _, n := range names {
		if err := deleteTree(path + `\` + n); err != nil {
			return err
		}
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, path); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// assocChanged tells the shell the kinds of file changed, so Explorer
// shows the new icons and Open with lists.
func assocChanged() {
	const shcneAssocChanged = 0x08000000
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("SHChangeNotify")
	if proc.Find() == nil {
		_, _, _ = proc.Call(shcneAssocChanged, 0, 0, 0)
	}
}

// shortcut makes a shortcut at lnk to exe, through PowerShell's Windows
// Script Host object: the shell's own way to write one, with no COM
// written here. A shortcut there already is left as it is: its target
// is the same, and making one takes a second.
func shortcut(lnk, exe, about string) error {
	if _, err := os.Stat(lnk); err == nil {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = filepath.Dir(exe)
	}
	// The values reach the script through its environment, so no
	// character in a name or a path can end a string in it.
	cmd := powershell("$s = (New-Object -ComObject WScript.Shell).CreateShortcut($env:GUNIM_LNK); "+
		"$s.TargetPath = $env:GUNIM_EXE; $s.WorkingDirectory = $env:GUNIM_HOME; "+
		"$s.IconLocation = $env:GUNIM_EXE + ',0'; $s.Description = $env:GUNIM_ABOUT; $s.Save()",
		"GUNIM_LNK="+lnk, "GUNIM_EXE="+exe, "GUNIM_HOME="+home, "GUNIM_ABOUT="+about)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// powershell is PowerShell running script, with no window, with env
// added to its environment, in a folder that is not the program's, so
// it holds nothing there open.
func powershell(script string, env ...string) *exec.Cmd {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = os.TempDir()
	return cmd
}

// setAutostart starts the program at exe with args when the user signs
// in, or no longer.
func setAutostart(a *App, exe string, args []string, on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open the programs Windows starts: %w", err)
	}
	defer func() { _ = k.Close() }()
	if !on {
		if err := k.DeleteValue(a.id()); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	line := `"` + exe + `"`
	for _, arg := range args {
		line += " " + windows.EscapeArg(arg)
	}
	return k.SetStringValue(a.id(), line)
}

// unregister takes away all that register adds.
func unregister(a *App, in Installation) error {
	if menu, err := startMenu(a); err == nil {
		_ = os.Remove(menu)
	}
	if desk, err := desktop(a); err == nil {
		_ = os.Remove(desk)
	}
	_ = setAutostart(a, in.Exe, nil, false)
	for _, t := range a.FileTypes {
		if err := dropType(a, t); err != nil {
			return err
		}
	}
	if len(a.FileTypes) > 0 {
		_ = deleteTree(classesKey + `Applications\` + filepath.Base(in.Exe))
		assocChanged()
	}
	if err := deleteTree(uninstallKeys + a.id()); err != nil {
		return err
	}
	return nil
}

// removeFiles takes the files away, and the folders under dir and dir
// itself that leaves empty. A running program's file, and a library it
// has loaded, cannot be taken away: PowerShell takes those once this
// program has ended.
func removeFiles(files []string, dir string) error {
	// A folder this program is in cannot be taken away.
	if wd, err := os.Getwd(); err == nil && strings.HasPrefix(strings.ToLower(wd), strings.ToLower(dir)) {
		_ = os.Chdir(os.TempDir())
	}
	var left []string
	for _, f := range files {
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			left = append(left, f)
		}
	}
	pruneEmpty(dir)
	if err := os.Remove(dir); err == nil || len(left) == 0 {
		return nil
	}
	// The paths reach the script through its environment, split on |,
	// which no Windows path holds.
	script := fmt.Sprintf("Wait-Process -Id %d -ErrorAction SilentlyContinue; "+
		"$files = $env:GUNIM_FILES -split '\\|'; "+
		"for ($i = 0; $i -lt 20; $i++) { Remove-Item -LiteralPath $files -Force -ErrorAction SilentlyContinue; "+
		"if (-not (Test-Path -LiteralPath $files[0])) { break }; Start-Sleep -Milliseconds 500 }; "+
		"Get-ChildItem -LiteralPath $env:GUNIM_DIR -Recurse -Directory | Sort-Object { $_.FullName.Length } -Descending | "+
		"Where-Object { -not (Get-ChildItem -LiteralPath $_.FullName -Force) } | Remove-Item -Force; "+
		"if (-not (Get-ChildItem -LiteralPath $env:GUNIM_DIR -Force)) { Remove-Item -LiteralPath $env:GUNIM_DIR -Force }",
		os.Getpid())
	cmd := powershell(script, "GUNIM_FILES="+strings.Join(left, "|"), "GUNIM_DIR="+dir)
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
	return cmd.Start()
}

// freeSpace is how many bytes the user may write to the volume dir is
// on.
func freeSpace(dir string) (int64, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, err
	}
	var free, total, all uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &all); err != nil {
		return 0, err
	}
	return int64(free), nil
}

// running lists the processes running the program at exe.
func running(exe string) ([]int, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var pids []int
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if !strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), filepath.Base(exe)) {
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		n := uint32(len(buf))
		err = windows.QueryFullProcessImageName(h, 0, &buf[0], &n)
		_ = windows.CloseHandle(h)
		if err == nil && samePath(windows.UTF16ToString(buf[:n]), exe) {
			pids = append(pids, int(e.ProcessID))
		}
	}
	return pids, nil
}

// launch starts the program at exe with args, on its own.
func launch(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
