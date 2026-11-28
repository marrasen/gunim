//go:build !js

package vst3

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/marrasen/gunim/audio/plug"
)

// A Library is the VST3 plugins a host has on disk, which package plug
// finds by their IDs, as [PlugID] gives them, once the host hands
// [Library.Find] to [plug.AddFinder]: a song naming a VST3 plugin loads
// it as the song's plugins are looked up.
//
// A plugin that crashes as it loads takes the host down with it. So
// before the library loads a module, it writes the module's path to a
// journal, a small file, and clears it after. A path still written as
// the next library starts is a module that crashed: the library skips
// it from then on, lists it in [Library.Failed], and loads it again only
// when [Library.Retry] asks.
//
// A bundle that lists nothing has its module loaded to find what it
// makes, and Load keeps a module loaded until [Shutdown]. So the journal
// keeps what each such module made, by the size and time of its binary,
// and the next library lists those from the journal, loading none of
// them till a song asks for one. A program with many plugins installed
// would otherwise hold every one of them loaded, with what each
// allocated, used or not.
type Library struct {
	journal string
	scanned chan struct{}
	// bundles is what the scan found, set before scanned closes.
	bundles []Bundle

	mu sync.Mutex
	// found are the plugins found; failed holds the errors of IDs that
	// would not load; crashed are the modules that crashed as they
	// loaded.
	found   []Found
	failed  map[string]error
	crashed []string
	// made is what the modules of the bundles that list nothing make, as
	// their loads found it.
	made map[string]scannedModule
	// loading serializes the loads, as the journal names one at a time.
	loading sync.Mutex
}

// Found is an effect or an instrument a Library found.
type Found struct {
	// ID is what package plug knows it by.
	ID string
	Class
	// Path is the bundle or file that makes it.
	Path string
}

// journalFile is what a library's journal holds.
type journalFile struct {
	// Loading is the module being loaded, or empty.
	Loading string
	// Crashed are the modules that crashed as they loaded.
	Crashed []string
	// Scanned are the modules of the bundles that list nothing, by path,
	// as their loads found them.
	Scanned map[string]scannedModule `json:",omitempty"`
}

// scannedModule is what a module of a bundle that lists nothing made,
// and the size and time of its binary as it was loaded: a binary of
// another size or time is loaded again.
type scannedModule struct {
	Size, ModTime int64
	Classes       []Class
}

// stamp returns the size and time of the binary of the module at path.
func stamp(path string) (size, modTime int64, ok bool) {
	bin, err := binaryPath(path)
	if err != nil {
		return 0, 0, false
	}
	fi, err := os.Stat(bin)
	if err != nil {
		return 0, 0, false
	}
	return fi.Size(), fi.ModTime().UnixNano(), true
}

// ErrCrashed is the error of a plugin whose module crashed as it loaded,
// as a library's journal tells.
var ErrCrashed = errors.New("vst3: the plugin's module crashed as it loaded, the last time it was tried")

// Journal returns where a program's library keeps its journal, by
// default: in the user's cache folder, named for the program.
func Journal() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	name := "program"
	if exe, err := os.Executable(); err == nil {
		name = strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	}
	return filepath.Join(dir, "gunim", "vst3", name+".json")
}

// NewLibrary returns the library of the plugins in the system's folders,
// as [Dirs] gives them, and in dirs, scanned on a goroutine of its own:
// from what each bundle lists, or, for one that lists nothing, loading
// its module. It keeps its journal where [Journal] says.
func NewLibrary(dirs ...string) *Library { return NewLibraryAt(Journal(), dirs...) }

// NewLibraryAt is NewLibrary, keeping its journal in the file journal;
// an empty journal keeps none.
func NewLibraryAt(journal string, dirs ...string) *Library {
	l := &Library{journal: journal, scanned: make(chan struct{}), failed: map[string]error{}}
	j := l.read()
	if j.Loading != "" && !slices.Contains(j.Crashed, j.Loading) {
		j.Crashed = append(j.Crashed, j.Loading)
	}
	j.Loading = ""
	l.crashed = j.Crashed
	l.made = j.Scanned
	if l.made == nil {
		l.made = map[string]scannedModule{}
	}
	l.write(j)
	go l.scan(append(systemDirs(), dirs...))
	return l
}

// systemDirs gives the system's folders a library scans: [Dirs], or a
// test's own.
var systemDirs = Dirs

// read reads the journal, or an empty one.
func (l *Library) read() journalFile {
	var j journalFile
	if l.journal == "" {
		return j
	}
	if b, err := os.ReadFile(l.journal); err == nil {
		_ = json.Unmarshal(b, &j)
	}
	return j
}

// write writes j to the journal, whole and then renamed over it, so a
// crash leaves the old or the new.
func (l *Library) write(j journalFile) {
	if l.journal == "" {
		return
	}
	b, err := json.Marshal(j)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(l.journal), 0o755); err != nil {
		return
	}
	tmp := l.journal + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, l.journal)
	}
}

// beforeLoad, set by a test, runs as a module is about to load.
var beforeLoad func(path string)

// load runs fn, which loads the module at path, with the path in the
// journal while it runs, or returns ErrCrashed where it crashed before.
func (l *Library) load(path string, fn func() error) error {
	l.loading.Lock()
	defer l.loading.Unlock()
	l.mu.Lock()
	crashed := slices.Contains(l.crashed, path)
	j := l.journalLocked()
	j.Loading = path
	l.mu.Unlock()
	if crashed {
		return fmt.Errorf("%w: %s", ErrCrashed, path)
	}
	l.write(j)
	if beforeLoad != nil {
		beforeLoad(path)
	}
	err := fn()
	j.Loading = ""
	l.write(j)
	return err
}

// journalLocked is what the journal holds now, but the module loading.
// The caller holds l.mu.
func (l *Library) journalLocked() journalFile {
	return journalFile{Crashed: slices.Clone(l.crashed), Scanned: maps.Clone(l.made)}
}

// cached returns what the module at path made, as the journal keeps it,
// where its binary has not changed since and it did not crash.
func (l *Library) cached(path string) ([]Class, bool) {
	size, modTime, ok := stamp(path)
	if !ok {
		return nil, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.made[path]
	if !ok || s.Size != size || s.ModTime != modTime || slices.Contains(l.crashed, path) {
		return nil, false
	}
	return slices.Clone(s.Classes), true
}

// remember keeps what the module at path made, for the journal.
func (l *Library) remember(path string, classes []Class) {
	size, modTime, ok := stamp(path)
	if !ok {
		return
	}
	l.mu.Lock()
	l.made[path] = scannedModule{Size: size, ModTime: modTime, Classes: slices.Clone(classes)}
	l.mu.Unlock()
}

func (l *Library) scan(dirs []string) {
	defer close(l.scanned)
	l.bundles = Scan(dirs...)
	var found []Found
	unlisted := map[string]bool{}
	for _, b := range l.bundles {
		classes := append(slices.Clone(b.Effects), b.Instruments...)
		if !b.Listed {
			unlisted[b.Path] = true
			cs, ok := l.cached(b.Path)
			if !ok {
				var m *Module
				if l.load(b.Path, func() (err error) { m, err = Load(b.Path); return err }) != nil {
					continue
				}
				cs = append(m.Effects(), m.Instruments()...)
				l.remember(b.Path, cs)
			}
			classes = cs
		}
		for _, c := range classes {
			found = append(found, Found{ID: PlugID(c), Class: c, Path: b.Path})
		}
	}
	l.mu.Lock()
	l.found = append(l.found, found...)
	sortFound(l.found)
	// Modules no longer on disk, or that list what they make now, are
	// not kept.
	maps.DeleteFunc(l.made, func(path string, _ scannedModule) bool { return !unlisted[path] })
	j := l.journalLocked()
	l.mu.Unlock()
	l.write(j)
}

// sortFound sorts fs by vendor, then name.
func sortFound(fs []Found) {
	slices.SortFunc(fs, func(a, b Found) int {
		if c := strings.Compare(strings.ToLower(a.Vendor), strings.ToLower(b.Vendor)); c != 0 {
			return c
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
}

// Scanned reports whether the scan is done, without waiting for it.
func (l *Library) Scanned() bool {
	select {
	case <-l.scanned:
		return true
	default:
		return false
	}
}

// Plugins returns the effects and instruments found, by vendor, then
// name, once the scan is done.
func (l *Library) Plugins() []Found {
	<-l.scanned
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.found)
}

// Failed returns the modules that crashed as they loaded, which the
// library loads no more until Retry asks, once the scan is done.
func (l *Library) Failed() []string {
	<-l.scanned
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.crashed)
}

// Retry loads the module at path again, one that crashed before, and
// lists the plugins it makes; should it crash again, the next library
// lists it as failed again. It returns why it would not load, as other
// than a crash.
func (l *Library) Retry(path string) error {
	<-l.scanned
	l.mu.Lock()
	l.crashed = slices.DeleteFunc(l.crashed, func(p string) bool { return p == path })
	for id, err := range l.failed {
		if errors.Is(err, ErrCrashed) {
			delete(l.failed, id)
		}
	}
	l.mu.Unlock()
	var m *Module
	if err := l.load(path, func() (err error) { m, err = Load(path); return err }); err != nil {
		return err
	}
	l.remember(path, append(m.Effects(), m.Instruments()...))
	l.mu.Lock()
	defer func() {
		j := l.journalLocked()
		l.mu.Unlock()
		l.write(j)
	}()
	for _, c := range append(m.Effects(), m.Instruments()...) {
		if !slices.ContainsFunc(l.found, func(f Found) bool { return f.ID == PlugID(c) }) {
			l.found = append(l.found, Found{ID: PlugID(c), Class: c, Path: path})
		}
	}
	sortFound(l.found)
	return nil
}

// Find is a [plug.Finder]: it registers the plugin of id, a VST3
// plugin's ID found, and reports whether it did, once the scan is done.
// An ID it found nothing of, or whose plugin would not load, it
// remembers, with [Library.Err] saying why.
func (l *Library) Find(id string) bool {
	if _, ok := ClassID(id); !ok {
		return false
	}
	<-l.scanned
	l.mu.Lock()
	_, failed := l.failed[id]
	i := slices.IndexFunc(l.found, func(f Found) bool { return f.ID == id })
	var f Found
	if i >= 0 {
		f = l.found[i]
	}
	l.mu.Unlock()
	if failed {
		return false
	}
	err := fmt.Errorf("%w: %s", ErrNotFound, id)
	if i >= 0 {
		err = l.load(f.Path, func() error {
			m, err := Load(f.Path)
			if err != nil {
				return err
			}
			_, err = m.Register(f.Class)
			return err
		})
	}
	if err != nil {
		l.mu.Lock()
		l.failed[id] = err
		l.mu.Unlock()
		return false
	}
	return true
}

// Err returns why the plugin of ID id would not load, or nil.
func (l *Library) Err(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.failed[id]
}

var _ plug.Finder = (*Library)(nil).Find
