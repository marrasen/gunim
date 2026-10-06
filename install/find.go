package install

import (
	"errors"
	"path/filepath"
	"time"
)

// ErrNotInstalled says the program is not installed.
var ErrNotInstalled = errors.New("install: not installed")

// Where says where a goes: its folder, and the program there.
func Where(a App) (dir, exe string, err error) {
	if err := a.check(); err != nil {
		return "", "", err
	}
	if !supported {
		return "", "", ErrUnsupported
	}
	dir, err = a.dir()
	if err != nil {
		return "", "", err
	}
	return dir, filepath.Join(dir, a.exe()), nil
}

// Find returns the installed copy of a: as its install kept it, or as
// it is found where it goes or where older versions put it. It returns
// ErrNotInstalled when there is none.
func Find(a App) (*Installation, error) {
	dir, exe, err := Where(a)
	if err != nil {
		return nil, err
	}
	m, err := readManifest(dir, a.id())
	if err != nil {
		return nil, err
	}
	if m != nil {
		return m.installation(&a, dir), nil
	}
	if found := a.found(exe); found != "" {
		return &Installation{Dir: filepath.Dir(found), Exe: found, Picks: detect(&a, found), Updates: a.startMode()}, nil
	}
	return nil, ErrNotInstalled
}

// Change changes what the installed program has on the system, as its
// own settings do: the offers in picks, as [PickAutostart] or
// [PickDesktop], and the rest as they are. It copies nothing and runs
// no hook.
func Change(a App, picks map[string]bool) error {
	in, err := Find(a)
	if err != nil {
		return err
	}
	if a.Icon == nil && a.IconFunc != nil {
		a.Icon = a.IconFunc()
	}
	for k, v := range picks {
		in.Picks[k] = v
	}
	if err := register(&a, *in); err != nil {
		return err
	}
	if dir, _, _ := Where(a); !samePath(in.Dir, dir) {
		// Still where an older version put it, in a folder not its own:
		// there is nowhere to keep the picks until it moves.
		return nil
	}
	m, err := readManifest(in.Dir, a.id())
	if err != nil {
		return err
	}
	if m == nil {
		m = &manifest{ID: a.id(), Version: in.Version, Exe: filepath.Base(in.Exe), When: time.Now().UTC()}
	}
	m.Picks = in.Picks
	return m.write(in.Dir)
}

// SetUpdates sets how the installed program takes newer releases, as an
// Updates setting of the program's own does. The program's own look
// for them takes it up at its next look.
func SetUpdates(a App, mode UpdateMode) error {
	if !mode.valid() {
		return errors.New("install: no update mode " + string(mode))
	}
	dir, _, err := Where(a)
	if err != nil {
		return err
	}
	m, err := readManifest(dir, a.id())
	if err != nil {
		return err
	}
	if m == nil {
		return ErrNotInstalled
	}
	m.Updates = mode
	if m.Picks == nil {
		m.Picks = map[string]bool{}
	}
	m.Picks[PickUpdates] = mode == UpdatesInstall
	return m.write(dir)
}
