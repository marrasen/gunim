//go:build linux && !android

package install

import (
	"bytes"
	"errors"
	"fmt"
	"image/png"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// Linux installs, paths count letter case, and a running program is
// replaced by moving another over it.
const (
	supported  = true
	caseless   = false
	movesAside = false
	exeSuffix  = ""
)

// home is the user's home folder.
func home() (string, error) { return os.UserHomeDir() }

// dataHome is $XDG_DATA_HOME, or ~/.local/share, and configHome
// $XDG_CONFIG_HOME, or ~/.config.
func dataHome() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(d) {
		return d, nil
	}
	h, err := home()
	return filepath.Join(h, ".local", "share"), err
}

func configHome() (string, error) {
	if d := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(d) {
		return d, nil
	}
	h, err := home()
	return filepath.Join(h, ".config"), err
}

// defaultDir is ~/.local/share/<ID>: the program and its files, with a
// link to the program in ~/.local/bin.
func defaultDir(a *App) (string, error) {
	d, err := dataHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, a.id()), nil
}

// places are where an install puts what it adds to the desktop.
type places struct {
	// link is the link in ~/.local/bin, app the desktop file, icon its
	// icon, autostart the desktop file that starts the program with the
	// session, desk the shortcut on the desktop, and mime the program's
	// own kinds of file.
	link, app, icon, autostart, desk, mime string
	// apps and mimes are the folders the desktop's databases cover.
	apps, mimes string
}

func where(a *App) (places, error) {
	d, err := dataHome()
	if err != nil {
		return places{}, err
	}
	c, err := configHome()
	if err != nil {
		return places{}, err
	}
	h, err := home()
	if err != nil {
		return places{}, err
	}
	id := a.id()
	return places{
		link:      filepath.Join(h, ".local", "bin", a.exe()),
		app:       filepath.Join(d, "applications", id+".desktop"),
		icon:      filepath.Join(d, "icons", "hicolor", "256x256", "apps", id+".png"),
		autostart: filepath.Join(c, "autostart", id+".desktop"),
		desk:      filepath.Join(desktopDir(h), id+".desktop"),
		mime:      filepath.Join(d, "mime", "packages", id+".xml"),
		apps:      filepath.Join(d, "applications"),
		mimes:     filepath.Join(d, "mime"),
	}, nil
}

// desktopDir is the user's desktop folder, as user-dirs.dirs names it,
// or ~/Desktop.
func desktopDir(h string) string {
	c, err := configHome()
	if err == nil {
		raw, err := os.ReadFile(filepath.Join(c, "user-dirs.dirs"))
		if err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				v, ok := strings.CutPrefix(strings.TrimSpace(line), "XDG_DESKTOP_DIR=")
				if !ok {
					continue
				}
				v = strings.Trim(v, `"`)
				v = strings.Replace(v, "$HOME", h, 1)
				if filepath.IsAbs(v) {
					return v
				}
			}
		}
	}
	return filepath.Join(h, "Desktop")
}

// write writes data to path, making its folder.
func write(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeFile(path, data, mode)
}

// register adds the program to the desktop: the link in ~/.local/bin,
// the desktop file and its icon, the kinds of file it opens, and,
// as picked, the shortcut on the desktop and the start with the
// session. What was not picked is taken away, as after an install that
// had it.
func register(a *App, in Installation) error {
	p, err := where(a)
	if err != nil {
		return err
	}
	if !samePath(filepath.Dir(p.link), in.Dir) {
		if err := link(p.link, in.Exe); err != nil {
			return fmt.Errorf("link %s: %w", p.link, err)
		}
	}
	icon := ""
	if a.Icon != nil {
		var b bytes.Buffer
		if err := png.Encode(&b, square(a.Icon, 256)); err != nil {
			return err
		}
		if err := write(p.icon, b.Bytes(), 0o644); err != nil {
			return fmt.Errorf("write the icon: %w", err)
		}
		icon = a.id()
	}
	var types []string
	var own []FileType
	for _, t := range a.FileTypes {
		if !in.Chose(FileTypeKey(t)) {
			continue
		}
		types = append(types, mimeOf(a, t))
		if t.MIME == "" {
			own = append(own, t)
		}
	}
	if len(own) > 0 {
		if err := write(p.mime, mimeXML(a, own), 0o644); err != nil {
			return fmt.Errorf("write the kinds of file: %w", err)
		}
	} else {
		_ = os.Remove(p.mime)
	}
	if err := write(p.app, desktopFile(a, in.Exe, icon, nil, types), 0o644); err != nil {
		return fmt.Errorf("write the desktop file: %w", err)
	}
	if in.Chose(PickDesktop) {
		if fi, err := os.Stat(filepath.Dir(p.desk)); err == nil && fi.IsDir() {
			if err := write(p.desk, desktopFile(a, in.Exe, icon, nil, nil), 0o755); err != nil {
				return fmt.Errorf("put %s on the desktop: %w", a.Name, err)
			}
			// GNOME runs a desktop file on the desktop only once it is
			// marked trusted; without gio this is left for the user.
			if gio, err := exec.LookPath("gio"); err == nil {
				_ = exec.Command(gio, "set", p.desk, "metadata::trusted", "true").Run()
			}
		}
	} else {
		_ = os.Remove(p.desk)
	}
	if a.Autostart != nil && in.Chose(PickAutostart) {
		if err := write(p.autostart, desktopFile(a, in.Exe, icon, a.Autostart.Args, nil), 0o644); err != nil {
			return fmt.Errorf("start %s with the session: %w", a.Name, err)
		}
	} else {
		_ = os.Remove(p.autostart)
	}
	refresh(p)
	for _, t := range a.FileTypes {
		if t.Default && in.Chose(FileTypeKey(t)) {
			if xdg, err := exec.LookPath("xdg-mime"); err == nil {
				_ = exec.Command(xdg, "default", a.id()+".desktop", mimeOf(a, t)).Run()
			}
		}
	}
	return nil
}

// unregister takes away all that register adds.
func unregister(a *App, in Installation) error {
	p, err := where(a)
	if err != nil {
		return err
	}
	for _, f := range []string{p.app, p.icon, p.autostart, p.desk, p.mime} {
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	// The link, only while it is the program's: a file of that name
	// put there by something else stays.
	if to, err := os.Readlink(p.link); err == nil && samePath(to, in.Exe) {
		_ = os.Remove(p.link)
	}
	forgetDefaults(a)
	refresh(p)
	return nil
}

// forgetDefaults takes the program's desktop file out of the user's
// mimeapps.list, where xdg-mime named it the one to open its kinds of
// file, so nothing is left pointing at a program that is gone.
func forgetDefaults(a *App) {
	c, err := configHome()
	if err != nil {
		return
	}
	path := filepath.Join(c, "mimeapps.list")
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	desk := a.id() + ".desktop"
	var out []string
	changed := false
	for _, line := range strings.Split(string(raw), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.Contains(value, desk) || strings.HasPrefix(strings.TrimSpace(line), "[") {
			out = append(out, line)
			continue
		}
		var keep []string
		for _, e := range strings.Split(value, ";") {
			if e = strings.TrimSpace(e); e != "" && e != desk {
				keep = append(keep, e)
			}
		}
		changed = true
		if len(keep) > 0 {
			out = append(out, key+"="+strings.Join(keep, ";")+";")
		}
	}
	if changed {
		_ = writeFile(path, []byte(strings.Join(out, "\n")), 0o644)
	}
}

// refresh has the desktop read its databases of applications and kinds
// of file again, where it has the tools.
func refresh(p places) {
	if tool, err := exec.LookPath("update-mime-database"); err == nil {
		if _, err := os.Stat(filepath.Join(p.mimes, "packages")); err == nil {
			_ = exec.Command(tool, p.mimes).Run()
		}
	}
	if tool, err := exec.LookPath("update-desktop-database"); err == nil {
		_ = exec.Command(tool, p.apps).Run()
	}
}

// link makes at a link to exe, in place of one there before. A file at
// that place that is not a link is left, and said.
func link(at, exe string) error {
	if fi, err := os.Lstat(at); err == nil {
		if fi.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("%s is a file of its own", at)
		}
		if err := os.Remove(at); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		return err
	}
	return os.Symlink(exe, at)
}

// mimeOf is t's media type: as the application gave it, or the
// program's own.
func mimeOf(a *App, t FileType) string {
	if t.MIME != "" {
		return t.MIME
	}
	return "application/x-" + a.id() + "-" + strings.ToLower(strings.TrimPrefix(t.Exts[0], "."))
}

// mimeXML teaches the desktop the program's own kinds of file.
func mimeXML(a *App, types []FileType) []byte {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<mime-info xmlns=\"http://www.freedesktop.org/standards/shared-mime-info\">\n")
	for _, t := range types {
		fmt.Fprintf(&b, "  <mime-type type=%q>\n", mimeOf(a, t))
		name := t.Name
		if name == "" {
			name = a.Name + " file"
		}
		fmt.Fprintf(&b, "    <comment>%s</comment>\n", xmlText(name))
		for _, e := range t.Exts {
			fmt.Fprintf(&b, "    <glob pattern=\"*%s\"/>\n", xmlText(strings.ToLower(e)))
		}
		b.WriteString("  </mime-type>\n")
	}
	b.WriteString("</mime-info>\n")
	return []byte(b.String())
}

// xmlText is s with the characters XML gives a meaning escaped.
func xmlText(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// desktopFile is a desktop file for the program at exe, started with
// args, opening the kinds of file types.
func desktopFile(a *App, exe, icon string, args, types []string) []byte {
	var b strings.Builder
	b.WriteString("[Desktop Entry]\nType=Application\n")
	b.WriteString("Name=" + oneLine(a.Name) + "\n")
	if a.Description != "" {
		b.WriteString("Comment=" + oneLine(a.Description) + "\n")
	}
	b.WriteString("Exec=" + execArg(exe))
	for _, arg := range args {
		b.WriteString(" " + execArg(arg))
	}
	if len(types) > 0 {
		b.WriteString(" %F")
	}
	b.WriteString("\n")
	if icon != "" {
		b.WriteString("Icon=" + icon + "\n")
	}
	b.WriteString("Terminal=false\n")
	cats := a.Categories
	if cats == "" {
		cats = "Utility;"
	}
	b.WriteString("Categories=" + oneLine(cats) + "\n")
	if len(types) > 0 {
		b.WriteString("MimeType=" + strings.Join(types, ";") + ";\n")
	}
	if a.Version != "" {
		b.WriteString("X-Gunim-Version=" + oneLine(a.Version) + "\n")
	}
	return []byte(b.String())
}

// oneLine is s as a desktop file's value: on one line, its backslashes
// escaped.
func oneLine(s string) string {
	return strings.NewReplacer("\\", `\\`, "\n", " ", "\r", " ", "\t", " ").Replace(s)
}

// execArg is an argument as an Exec line holds it: quoted when it has a
// space or a character the line gives a meaning, with the escapes the
// desktop entry spec asks for inside quotes, and its backslashes doubled
// again as a desktop file's strings have them.
func execArg(arg string) string {
	if arg != "" && !strings.ContainsAny(arg, " \t\n\"'\\><~|&;$*?#()`%") {
		return arg
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range arg {
		switch r {
		case '"', '`', '$':
			b.WriteString(`\\`)
		case '\\':
			b.WriteString(`\\\`)
		case '%':
			b.WriteByte('%')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// removeFiles takes the files away, and the folders under dir and dir
// itself that leaves empty. A running program on Linux can be taken
// away under itself.
func removeFiles(files []string, dir string) error {
	for _, f := range files {
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	pruneEmpty(dir)
	_ = os.Remove(dir)
	return nil
}

// freeSpace is how many bytes the user may write to the file system
// dir is on.
func freeSpace(dir string) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}

// running lists the processes running the program at exe.
func running(exe string) ([]int, error) {
	want := resolve(exe)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		to, err := os.Readlink(filepath.Join("/proc", e.Name(), "exe"))
		if err != nil {
			continue
		}
		// A program replaced while it runs reads as deleted.
		to = strings.TrimSuffix(to, " (deleted)")
		if to == want {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}

// launch starts the program at exe with args, on its own.
func launch(exe string, args []string) error {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
