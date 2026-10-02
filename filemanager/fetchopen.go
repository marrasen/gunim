package filemanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The limits of opening files of another file system with the
// computer's programs.
const (
	// fetchAsk is how many bytes a fetch to open may copy before it asks.
	fetchAsk = 200 << 20
	// fetchManyAsk is how many files it may open at once before it asks,
	// as each opens a program's window.
	fetchManyAsk = 10
	// copiesKept is how long another hub's copies stay before a hub that
	// starts takes them away.
	copiesKept = 24 * time.Hour
)

// fetches reports whether the window's files open with the computer's
// programs only by a copy fetched to it first: they are of another file
// system, which cannot open them itself.
func (a *app) fetches() bool {
	if a.fs.ID() == "" {
		return false
	}
	_, ok := a.fs.(SystemOpener)
	return !ok
}

// openFiles opens the items at paths with the system's programs: the
// computer's own at once, those of a SystemOpener through it, and the
// files of another file system by fetching a copy to this computer
// first. Folders of that one stay where they are.
func (a *app) openFiles(paths []string) {
	if len(paths) == 0 {
		return
	}
	if a.fetches() {
		a.fetchToOpen(paths)
		return
	}
	open, ok := a.systemOpen()
	if !ok {
		return
	}
	go func() {
		for _, p := range paths {
			if err := open(p); err != nil {
				a.post(func() { a.fail(fmt.Sprintf("Opening %s: %v", p, err)) })
				return
			}
		}
	}()
}

// remoteFile is a file of another file system to fetch and open.
type remoteFile struct {
	path string
	size int64
	mod  time.Time
}

// fetchToOpen opens the files at paths, of a file system other than the
// computer's own, with the computer's programs: a copy fetched before
// and unchanged since at once, and the others once fetched, as an
// operation of the window. It asks first before it fetches much, or
// many.
func (a *app) fetchToOpen(paths []string) {
	fsys, copies := a.fs, a.hub.openCopies()
	go func() {
		var ready []string
		var want []remoteFile
		dirs := 0
		for _, p := range paths {
			info, err := fsys.Stat(p)
			if err != nil {
				a.post(func() { a.fail(fmt.Sprintf("Opening %s: %v", p, err)) })
				return
			}
			if info.IsDir() {
				dirs++
				continue
			}
			f := remoteFile{path: p, size: info.Size(), mod: info.ModTime()}
			if local, ok := copies.lookup(fsys.ID(), f); ok {
				ready = append(ready, local)
				continue
			}
			want = append(want, f)
		}
		a.post(func() { a.fetchOrOpen(fsys, copies, ready, want, dirs) })
	}()
}

// fetchOrOpen opens the copies ready, and fetches those wanted from fsys
// to open them, after asking where they are many or large. dirs counts
// the folders asked for, which stay where they are.
func (a *app) fetchOrOpen(fsys FS, copies *openCopies, ready []string, want []remoteFile, dirs int) {
	if len(ready)+len(want) == 0 {
		if dirs > 0 {
			a.fail("Folders here cannot open with this computer's programs.")
		}
		return
	}
	if len(ready) > 0 {
		open := a.c.Open
		go func() {
			for _, p := range ready {
				if err := open(p); err != nil {
					a.post(func() { a.fail(fmt.Sprintf("Opening %s: %v", filepath.Base(p), err)) })
					return
				}
			}
		}()
	}
	if len(want) == 0 {
		return
	}
	var total int64
	for _, f := range want {
		total += f.size
	}
	ps := fsys.Paths()
	them := "them"
	if len(want) == 1 {
		them = "it"
	}
	start := func() { a.startFetch(fsys, copies, want) }
	switch n := len(ready) + len(want); {
	case total > fetchAsk:
		a.confirm(Confirm{Title: "Fetch " + humanBytes(total) + " to open " + them + "?",
			Body: a.fetchBody(ps, want), OK: "Fetch"}, start)
	case n > fetchManyAsk:
		a.confirm(Confirm{Title: "Open " + plural(n, "file") + "?",
			Body: "Each opens in its program. " + a.fetchBody(ps, want), OK: "Open"}, start)
	default:
		start()
	}
}

// fetchBody says where the files of want, of a file system writing paths
// as ps, go to open.
func (a *app) fetchBody(ps PathStyle, want []remoteFile) string {
	what := ps.Base(want[0].path) + " is"
	if len(want) > 1 {
		what = plural(len(want), "file") + " are"
	}
	return what + " copied to a temporary folder on this computer and opened from there."
}

// startFetch fetches the files of want from fsys into copies, as one
// operation of the window, and opens each with its program once it is
// here.
func (a *app) startFetch(fsys FS, copies *openCopies, want []remoteFile) {
	ps := fsys.Paths()
	what := ps.Base(want[0].path)
	if len(want) > 1 {
		what = plural(len(want), "file")
	}
	id, ctx, _ := a.newOp("Fetching "+what+" to open", OpCopy)
	var total int64
	for _, f := range want {
		total += f.size
	}
	open := a.c.Open
	a.ops.wg.Go(func() {
		var done int64
		var last time.Time
		var openErr error
		var err error
		for i, f := range want {
			report := func(n int64, now bool) {
				if t := time.Now(); now || t.Sub(last) >= 50*time.Millisecond {
					last = t
					p := progress{items: i, itemsTotal: len(want), bytes: done + n, bytesTotal: total, current: f.path}
					a.post(func() { a.progressed(id, p) })
				}
			}
			report(0, true)
			var local string
			local, err = copies.fetch(ctx, fsys, f, report)
			if err != nil {
				break
			}
			done += f.size
			if oerr := open(local); oerr != nil && openErr == nil {
				openErr = fmt.Errorf("opening %s: %w", ps.Base(f.path), oerr)
			}
		}
		stopped := ctx.Err() != nil
		a.post(func() { a.fetchedToOpen(id, what, err, openErr, stopped) })
	})
}

// fetchedToOpen takes the end of fetch operation id, which fetched what:
// err says why it stopped short, and openErr why a file fetched did not
// open; stopped says the user stopped it, or the window closed.
func (a *app) fetchedToOpen(id int, what string, err, openErr error, stopped bool) {
	r, ok := a.ops.running[id]
	if !ok {
		return
	}
	delete(a.ops.running, id)
	r.cancel()
	a.dropDialogs(id)
	if r.visible {
		a.patch(OpDone{ID: id, OK: err == nil})
		if err == nil {
			r.title = "Fetched " + what
			a.cheer(id, r)
		}
		a.publishOps()
	}
	switch {
	case stopped || errors.Is(err, context.Canceled):
		a.patch(Notice{Title: "Stopped: " + r.title, Kind: "warning"})
	case err != nil:
		a.showError(ErrorBox{Title: "The fetch to open stopped", Body: err.Error()})
	case openErr != nil:
		a.fail(openErr.Error() + ".")
	}
}

// openCopies are the copies a hub fetched of files of other file
// systems to open them with the computer's programs. They are kept in a
// folder of the hub's own in base, and one fetched again while its file
// has not changed opens at once.
type openCopies struct {
	mu sync.Mutex
	// base holds the folders of every hub, and dir this hub's, made with
	// the first copy.
	base, dir string
	kept      map[copyKey]openCopy
}

// copyKey is a file of a file system, by the system's ID and the path.
type copyKey struct{ fs, path string }

// openCopy is a file fetched: where its copy is, the size and the time
// of the file it was fetched from, and the time the copy had once made,
// which a program that changes it changes.
type openCopy struct {
	local    string
	size     int64
	mod      time.Time
	localMod time.Time
}

// copiesBase is the folder in the system's temporary folder that holds
// the hubs' copies, the user's own where the folder is shared.
func copiesBase() string {
	name := "gunim-files-open"
	if uid := os.Getuid(); uid >= 0 {
		name += "-" + strconv.Itoa(uid)
	}
	return filepath.Join(os.TempDir(), name)
}

// openCopies returns the hub's copies of files fetched to open.
func (h *Hub) openCopies() *openCopies {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.copies == nil {
		h.copies = &openCopies{base: copiesBase()}
	}
	return h.copies
}

// removeCopies takes away the hub's copies, those no program holds.
func (h *Hub) removeCopies() {
	h.mu.Lock()
	c := h.copies
	h.mu.Unlock()
	if c != nil {
		c.removeAll()
	}
}

// lookup returns the copy of f, of the file system of ID fsID, where one
// was fetched while f was as it is now and is as it was made.
func (c *openCopies) lookup(fsID string, f remoteFile) (string, bool) {
	c.mu.Lock()
	k, ok := c.kept[copyKey{fsID, f.path}]
	c.mu.Unlock()
	if !ok || k.size != f.size || !k.mod.Equal(f.mod) {
		return "", false
	}
	info, err := os.Stat(k.local)
	if err != nil || info.Size() != k.size || !info.ModTime().Equal(k.localMod) {
		return "", false
	}
	return k.local, true
}

// folder returns the hub's folder of copies, made with its first call,
// which first takes away the folders of hubs gone a while.
func (c *openCopies) folder() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dir != "" {
		// A hub that runs for days keeps its folder fresh, so another
		// starting leaves it be.
		now := time.Now()
		_ = os.Chtimes(c.dir, now, now)
		return c.dir, nil
	}
	if err := os.MkdirAll(c.base, 0o700); err != nil {
		return "", err
	}
	sweep(c.base, time.Now().Add(-copiesKept))
	dir, err := os.MkdirTemp(c.base, "hub-")
	if err != nil {
		return "", err
	}
	c.dir = dir
	return dir, nil
}

// sweep takes away what in base was last changed before cutoff, as much
// of it as it can: a program may still hold a file, which Windows will
// not remove.
func sweep(base string, cutoff time.Time) {
	es, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, e := range es {
		if info, err := e.Info(); err == nil && info.ModTime().Before(cutoff) {
			_ = os.RemoveAll(filepath.Join(base, e.Name()))
		}
	}
}

// removeAll takes away the hub's folder of copies, as much of it as no
// program holds.
func (c *openCopies) removeAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dir != "" {
		_ = os.RemoveAll(c.dir)
	}
	c.dir, c.kept = "", nil
}

// fetch copies f from fsys into the hub's folder, under the name it has
// there, so a program shows the name and knows the file by its
// extension. report hears the bytes copied so far, now set at the end.
// A fetch that fails or stops leaves nothing behind.
func (c *openCopies) fetch(ctx context.Context, fsys FS, f remoteFile, report func(n int64, now bool)) (string, error) {
	dir, err := c.folder()
	if err != nil {
		return "", fmt.Errorf("making a temporary folder: %w", err)
	}
	name := localName(fsys.Paths().Base(f.path))
	sub := filepath.Join(dir, shortHash(fsys.ID()), shortHash(f.path))
	// A copy fetched before goes, unless a program holds it, as on
	// Windows; the new one then goes to a folder beside it.
	target := ""
	for n := 1; n <= 100; n++ {
		d := sub
		if n > 1 {
			d += "-" + strconv.Itoa(n)
		}
		t := filepath.Join(d, name)
		if rerr := os.Remove(t); rerr == nil || errors.Is(rerr, fs.ErrNotExist) {
			target = t
			break
		}
	}
	if target == "" {
		return "", fmt.Errorf("no room for a copy of %s: programs hold the copies fetched before", name)
	}
	if err = os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return "", fmt.Errorf("making a temporary folder: %w", err)
	}
	if err = copyOut(ctx, fsys, f.path, target, report); err != nil {
		_ = os.Remove(filepath.Dir(target))
		return "", err
	}
	_ = os.Chtimes(target, time.Now(), f.mod)
	info, err := os.Stat(target)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	if c.kept == nil {
		c.kept = map[copyKey]openCopy{}
	}
	c.kept[copyKey{fsys.ID(), f.path}] = openCopy{local: target, size: f.size, mod: f.mod, localMod: info.ModTime()}
	c.mu.Unlock()
	return target, nil
}

// copyOut copies the file at src on fsys to target on the computer's own
// disk, through a part file that it takes away where the copy fails or
// ctx ends.
func copyOut(ctx context.Context, fsys FS, src, target string, report func(n int64, now bool)) (err error) {
	in, err := fsys.Open(src)
	if err != nil {
		return fmt.Errorf("reading %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.CreateTemp(filepath.Dir(target), ".fetch-*.part")
	if err != nil {
		return fmt.Errorf("writing %s: %w", target, err)
	}
	part := out.Name()
	fail := func(e error) error {
		_ = out.Close()
		_ = os.Remove(part)
		return e
	}
	buf := make([]byte, copyBuffer)
	var n int64
	for {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		k, rerr := in.Read(buf)
		if k > 0 {
			if _, err := out.Write(buf[:k]); err != nil {
				return fail(fmt.Errorf("writing %s: %w", target, err))
			}
			n += int64(k)
			report(n, false)
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return fail(fmt.Errorf("reading %s: %w", src, rerr))
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(part)
		return fmt.Errorf("writing %s: %w", target, err)
	}
	if err := os.Rename(part, target); err != nil {
		_ = os.Remove(part)
		return fmt.Errorf("writing %s: %w", target, err)
	}
	report(n, true)
	return nil
}

// shortHash names s by a hash of it, short enough for a folder's name.
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}

// localName is name made fit for the computer's own disk: a name of
// another file system may hold what this one does not take in a name.
func localName(name string) string {
	bad := `/\`
	if runtime.GOOS == "windows" {
		bad += `<>:"|?*`
	}
	name = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(bad, r) {
			return '_'
		}
		return r
	}, name)
	if runtime.GOOS == "windows" {
		name = strings.TrimRight(name, ". ")
		stem := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
		switch stem {
		case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
			"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
			name = "_" + name
		}
	}
	if name == "" || name == "." || name == ".." {
		name = "file"
	}
	return name
}
