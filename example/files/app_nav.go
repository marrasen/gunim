package main

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"
)

// blockRows is how many rows one RowBlock carries.
const blockRows = 256

// smallFolder is the most entries a folder may hold for the poll to list
// it again in full; a larger one is checked by its own time alone.
const smallFolder = 5000

// overviewBands is how many bands the overview strip shows.
const overviewBands = 160

// navState is what the app knows about the folder showing and how the
// user got there.
type navState struct {
	path       string
	back, fwd  []string
	gen        int
	all, rows  []entry
	mod        time.Time
	sig        uint64
	loading    bool
	err        error
	travel     int
	sort       SortBy
	desc       bool
	filter     string
	cancelList context.CancelFunc
	checking   bool
	// sel holds the names selected and cursor the name the keyboard is
	// on; pick is a name to select once the listing arrives.
	sel    map[string]bool
	cursor string
	pick   string
	// space is the free space of the folder's volume, or spaceErr why it
	// is unknown.
	space    space
	spaceErr error
}

func (n *navState) init() { n.sel = map[string]bool{} }

func (n *navState) stop() {
	if n.cancelList != nil {
		n.cancelList()
	}
}

// startNav shows the first folder: dir, or the home folder.
func (a *app) startNav(dir string) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			a.fail("Finding your home folder: " + err.Error())
			return
		}
		dir = home
	}
	a.navigate(dir, 0, true)
}

// handleNav takes the intents about moving between folders and the rows
// of the one showing.
func (a *app) handleNav(in gunim.Intent) bool {
	switch v := in.(type) {
	case NeedRows:
		a.sendRows(v)
	case Selected:
		a.selected(v)
	case Activated:
		if v.Gen == a.nav.gen && v.Row >= 0 && v.Row < len(a.nav.rows) {
			a.activate(a.nav.rows[v.Row])
		}
	case Typed:
		a.typed(v)
	case Navigate:
		if a.newWindowAsked() {
			a.openWindow(v.Path)
			break
		}
		a.navigate(v.Path, 0, true)
	case SortClicked:
		a.sortBy(SortBy(v.Column), true)
	case FilterChanged:
		a.nav.filter = v.Text
		a.refilter()
	case WindowFocused:
		a.poll()
		a.loadPlaces()
	case Command:
		return a.navCommand(v.Name)
	default:
		return false
	}
	return true
}

func (a *app) navCommand(name string) bool {
	n := &a.nav
	switch name {
	case CmdBack:
		if len(n.back) > 0 {
			to := n.back[len(n.back)-1]
			n.back = n.back[:len(n.back)-1]
			n.fwd = append(n.fwd, n.path)
			a.navigate(to, -1, false)
		}
	case CmdForward:
		if len(n.fwd) > 0 {
			to := n.fwd[len(n.fwd)-1]
			n.fwd = n.fwd[:len(n.fwd)-1]
			n.back = append(n.back, n.path)
			a.navigate(to, 1, false)
		}
	case CmdUp:
		if parent := filepath.Dir(n.path); parent != n.path {
			if a.newWindowAsked() {
				a.openWindow(parent)
				break
			}
			from := filepath.Base(n.path)
			a.navigate(parent, -1, true)
			n.pick = from
		}
	case CmdHome:
		home, err := os.UserHomeDir()
		if err != nil {
			a.fail("Finding your home folder: " + err.Error())
			return true
		}
		a.navigate(home, 0, true)
	case CmdRefresh:
		a.relist()
	case CmdOpen:
		for _, e := range a.selectedEntries() {
			a.activate(e)
		}
	case CmdHidden:
		a.shell.ShowHidden = !a.shell.ShowHidden
		a.prefs.ShowHidden = a.shell.ShowHidden
		a.publishShell()
		a.savePrefs()
		a.refilter()
	case CmdSortName, CmdSortSize, CmdSortTime, CmdSortType:
		by := map[string]SortBy{CmdSortName: SortName, CmdSortSize: SortSize, CmdSortTime: SortModified, CmdSortType: SortType}[name]
		a.sortBy(by, false)
	default:
		return false
	}
	return true
}

// navigate shows the folder at path. travel is the way the user went,
// and record keeps the folder left in the history.
func (a *app) navigate(path string, travel int, record bool) {
	n := &a.nav
	abs, err := filepath.Abs(path)
	if err != nil {
		a.fail(fmt.Sprintf("Opening %s: %v", path, err))
		return
	}
	if record && n.path != "" && !samePath(abs, n.path) {
		n.back = append(n.back, n.path)
		n.fwd = nil
	}
	if travel == 0 && n.path != "" {
		travel = direction(n.path, abs)
	}
	n.path, n.travel = abs, travel
	n.all, n.rows, n.err = nil, nil, nil
	n.filter, n.pick = "", ""
	clear(n.sel)
	n.cursor = ""
	a.enteredFolder()
	a.list()
	a.readSpace()
	a.publishPlaces()
	a.showPreview()
}

// direction is 1 when to lies inside from, -1 when from lies inside to,
// and 0 otherwise.
func direction(from, to string) int {
	if rel, err := filepath.Rel(from, to); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return 1
	}
	if rel, err := filepath.Rel(to, from); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return -1
	}
	return 0
}

// list reads the folder showing in the background.
func (a *app) list() {
	n := &a.nav
	n.stop()
	ctx, cancel := context.WithCancel(a.ctx)
	n.cancelList = cancel
	n.gen++
	gen, path := n.gen, n.path
	n.loading = true
	a.publishListing()
	go func() {
		mod, err := dirTime(path)
		var es []entry
		if err == nil {
			es, err = listDir(ctx, path)
		}
		if errors.Is(err, context.Canceled) {
			return
		}
		a.post(func() { a.listed(gen, path, es, mod, err) })
	}()
}

// relist reads the folder showing again, keeping the selection.
func (a *app) relist() {
	n := &a.nav
	path := n.path
	ctx := a.ctx
	gen := n.gen
	go func() {
		mod, err := dirTime(path)
		var es []entry
		if err == nil {
			es, err = listDir(ctx, path)
		}
		a.post(func() {
			if n.gen == gen && samePath(n.path, path) {
				a.listed(gen, path, es, mod, err)
			}
		})
	}()
}

// dirTime returns when the folder at path last changed.
func dirTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	if !info.IsDir() {
		return time.Time{}, fmt.Errorf("%s is not a folder", path)
	}
	return info.ModTime(), nil
}

// listed takes a listing of path made for listing gen.
func (a *app) listed(gen int, path string, es []entry, mod time.Time, err error) {
	n := &a.nav
	if gen != n.gen || !samePath(path, n.path) {
		return
	}
	n.loading = false
	n.err = err
	if err != nil {
		n.all, n.rows = nil, nil
		if errors.Is(err, fs.ErrNotExist) {
			n.err = fmt.Errorf("%s no longer exists", path)
		}
		a.publishListing()
		a.publishStatus()
		return
	}
	n.mod, n.sig = mod, signature(es)
	sortEntries(es, n.sort, n.desc)
	n.all = es
	a.refilter()
	n.travel = 0
	if slices.ContainsFunc(es, func(e entry) bool { return e.Kind == KindLink && e.Dir }) {
		// A link to a folder can lead to another volume.
		a.publishVolumes(Places{Current: path})
	}
	if len(a.script) > 0 && !a.scripting {
		a.scripting = true
		a.runScript()
	}
}

// signature sums up the names, sizes and times of es, to tell a folder
// that changed.
func signature(es []entry) uint64 {
	h := fnv.New64a()
	var buf [16]byte
	sum := uint64(0)
	for _, e := range es {
		h.Reset()
		_, _ = h.Write([]byte(e.Name))
		putInt(buf[:8], uint64(e.Size))
		putInt(buf[8:], uint64(e.Mod.UnixNano()))
		_, _ = h.Write(buf[:])
		// Adding makes the sum the same in any order.
		sum += h.Sum64()
	}
	return sum
}

func putInt(b []byte, v uint64) {
	for i := range 8 {
		b[i] = byte(v >> (8 * i))
	}
}

// refilter works out the rows showing from the entries, and publishes
// them as a new listing gen with the selection kept.
func (a *app) refilter() {
	n := &a.nav
	if n.loading || n.err != nil {
		return
	}
	was := n.rows
	n.rows = filterEntries(n.all, n.filter, a.shell.ShowHidden)
	n.gen++
	a.publishListing()
	if left := gone(was, n.rows); len(left) > 0 {
		a.patch(RowsLeft{Gen: n.gen, Rows: left})
	}
	a.publishBands()
	if n.pick != "" {
		clear(n.sel)
		n.sel[n.pick] = true
		n.cursor = n.pick
		n.pick = ""
	}
	a.publishSelection()
	a.publishStatus()
	a.showPreview()
}

// sortBy sorts the rows by column; toggle turns the order round when they
// are sorted by it already.
func (a *app) sortBy(by SortBy, toggle bool) {
	n := &a.nav
	if toggle && by == n.sort {
		n.desc = !n.desc
	} else {
		n.sort, n.desc = by, false
	}
	a.prefs.Sort, a.prefs.Desc = n.sort, n.desc
	a.savePrefs()
	sortEntries(n.all, n.sort, n.desc)
	a.refilter()
}

func (a *app) publishListing() {
	n := &a.nav
	l := Listing{
		Gen: n.gen, Path: n.path, Title: placeName(n.path), Crumbs: crumbs(n.path),
		Total: len(n.rows), All: len(n.all), Sort: n.sort, Desc: n.desc, Filter: n.filter,
		Travel: n.travel, Loading: n.loading,
		CanBack: len(n.back) > 0, CanForward: len(n.fwd) > 0, CanUp: filepath.Dir(n.path) != n.path,
	}
	if n.err != nil {
		l.Err = n.err.Error()
	}
	a.patch(l)
}

// crumbs splits path into the folders along it.
func crumbs(path string) []Crumb {
	vol := filepath.VolumeName(path)
	rest := strings.TrimPrefix(path, vol)
	root := vol + string(filepath.Separator)
	name := vol
	if name == "" {
		name = string(filepath.Separator)
	}
	out := []Crumb{{Name: name, Path: root}}
	at := root
	for _, part := range strings.Split(strings.Trim(rest, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		at = filepath.Join(at, part)
		out = append(out, Crumb{Name: part, Path: at})
	}
	return out
}

// sendRows answers NeedRows with the blocks it asks for.
func (a *app) sendRows(v NeedRows) {
	n := &a.nav
	if v.Gen != n.gen {
		return
	}
	for _, start := range v.Starts {
		if start < 0 || start >= len(n.rows) {
			continue
		}
		end := min(start+blockRows, len(n.rows))
		rows := make([]Row, 0, end-start)
		for _, e := range n.rows[start:end] {
			rows = append(rows, rowOf(e))
		}
		a.patch(RowBlock{Gen: n.gen, Start: start, Rows: rows})
	}
}

// rowOf is how the grid shows e.
func rowOf(e entry) Row {
	r := Row{Name: e.Name, Kind: e.Kind, Dir: e.Dir, Hidden: e.Hidden, Broken: e.Broken, Online: e.Online, Type: e.Type,
		Modified: e.Mod.Format("2006-01-02 15:04"), Tint: tintOf(e)}
	if !e.Dir && !e.Broken {
		r.Size = humanBytes(e.Size)
	}
	return r
}

// tintOf is the group e's colour comes from.
func tintOf(e entry) Tint {
	if e.Dir {
		return TintFolder
	}
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(e.Name), ".")) {
	case "png", "jpg", "jpeg", "gif", "bmp", "webp", "tif", "tiff", "svg", "ico", "heic":
		return TintImage
	case "mp4", "mkv", "mov", "avi", "webm":
		return TintVideo
	case "mp3", "wav", "flac", "ogg", "m4a":
		return TintAudio
	case "zip", "7z", "rar", "tar", "gz", "xz", "bz2", "zst", "iso":
		return TintArchive
	case "pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "odt", "txt", "md", "csv", "rtf":
		return TintDocument
	case "go", "c", "h", "cpp", "rs", "py", "js", "ts", "html", "css", "json", "xml", "yaml", "yml", "toml", "sh":
		return TintCode
	case "exe", "msi", "bat", "ps1", "dll", "so", "appimage":
		return TintProgram
	}
	return TintOther
}

// humanBytes writes n bytes the way people read them.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		if n == 1 {
			return "1 byte"
		}
		return fmt.Sprintf("%d bytes", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 5; m /= unit {
		div *= unit
		exp++
	}
	v := float64(n) / float64(div)
	if v >= 100 {
		return fmt.Sprintf("%.0f %cB", v, "KMGTPE"[exp])
	}
	return fmt.Sprintf("%.1f %cB", v, "KMGTPE"[exp])
}

// publishBands sends the overview strip for the rows showing.
func (a *app) publishBands() {
	n := &a.nav
	count := min(overviewBands, len(n.rows))
	bands := make([]Band, count)
	for b := range count {
		from, to := b*len(n.rows)/count, (b+1)*len(n.rows)/count
		shares := make([]float32, TintProgram+1)
		all := float32(max(to-from, 1))
		for _, e := range n.rows[from:to] {
			shares[tintOf(e)] += 1 / all
		}
		bands[b] = Band{Shares: shares, First: n.rows[from].Name}
	}
	a.patch(Bands{Gen: n.gen, Bands: bands})
}

// selected takes the rows the grid says are selected.
func (a *app) selected(v Selected) {
	n := &a.nav
	if v.Gen != n.gen {
		return
	}
	clear(n.sel)
	for _, r := range v.Runs {
		for i := max(r[0], 0); i < min(r[1], len(n.rows)); i++ {
			n.sel[n.rows[i].Name] = true
		}
	}
	n.cursor = ""
	if v.Cursor >= 0 && v.Cursor < len(n.rows) {
		n.cursor = n.rows[v.Cursor].Name
	}
	a.publishStatus()
	a.showPreview()
}

// publishSelection sends the rows holding the names selected, for the
// grid to select.
func (a *app) publishSelection() {
	n := &a.nav
	s := Selection{Gen: n.gen, Cursor: -1}
	for i, e := range n.rows {
		if n.sel[e.Name] {
			if k := len(s.Runs); k > 0 && s.Runs[k-1][1] == i {
				s.Runs[k-1][1] = i + 1
			} else {
				s.Runs = append(s.Runs, [2]int{i, i + 1})
			}
		}
		if e.Name == n.cursor {
			s.Cursor = i
		}
	}
	a.patch(s)
}

// newWindowAsked reports whether the folder the intent being handled opens goes in a new window: Ctrl was held, as in
// Explorer.
func (a *app) newWindowAsked() bool { return a.mods.Has(input.ModControl) }

// typed selects the item the text typed at the listing names, as Explorer does.
func (a *app) typed(v Typed) {
	n := &a.nav
	if v.Gen != n.gen {
		return
	}
	from := slices.IndexFunc(n.rows, func(e entry) bool { return e.Name == n.cursor })
	i := widget.FindTyped(v.Text, len(n.rows), from, func(i int) string { return n.rows[i].Name })
	if i < 0 {
		return
	}
	clear(n.sel)
	n.sel[n.rows[i].Name] = true
	n.cursor = n.rows[i].Name
	a.publishSelection()
	a.publishStatus()
	a.showPreview()
}

// selectedEntries returns the rows selected, in order.
func (a *app) selectedEntries() []entry {
	n := &a.nav
	if len(n.sel) == 0 {
		return nil
	}
	var out []entry
	for _, e := range n.rows {
		if n.sel[e.Name] {
			out = append(out, e)
		}
	}
	return out
}

// selectedPaths returns the paths of the rows selected.
func (a *app) selectedPaths() []string {
	es := a.selectedEntries()
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = filepath.Join(a.nav.path, e.Name)
	}
	return out
}

// activate opens e: a folder in the window, and a file with its program.
func (a *app) activate(e entry) {
	path := filepath.Join(a.nav.path, e.Name)
	if e.Dir && a.newWindowAsked() {
		a.openWindow(path)
		return
	}
	if e.Dir {
		a.navigate(path, 1, true)
		return
	}
	if e.Err != "" {
		a.fail(path + " cannot be read: " + e.Err)
		return
	}
	if e.Broken {
		a.fail(path + " is a link to something that is gone.")
		return
	}
	go func() {
		if err := a.c.Open(path); err != nil {
			a.post(func() { a.fail(fmt.Sprintf("Opening %s: %v", path, err)) })
		}
	}()
}

// publishStatus sends the status bar.
func (a *app) publishStatus() {
	n := &a.nav
	var s Status
	switch {
	case n.loading:
		s.Left = "Reading the folder…"
	case n.err != nil:
		s.Left = ""
	default:
		s.Left = plural(len(n.rows), "item")
		if hidden := len(n.all) - len(n.rows); hidden > 0 && n.filter == "" {
			s.Left += " (" + count(hidden) + " hidden)"
		} else if n.filter != "" {
			s.Left += " of " + count(len(n.all)) + " match “" + n.filter + "”"
		}
		if sel := a.selectedEntries(); len(sel) > 0 {
			var size int64
			files := 0
			for _, e := range sel {
				if !e.Dir {
					size += e.Size
					files++
				}
			}
			s.Left += "  ·  " + count(len(sel)) + " selected"
			if files > 0 {
				s.Left += " (" + humanBytes(size) + ")"
			}
		}
	}
	switch {
	case n.spaceErr != nil:
		s.Right = "Free space unknown: " + n.spaceErr.Error()
	case n.space.total > 0:
		s.Right = fmt.Sprintf("%s free of %s", humanBytes(int64(n.space.free)), humanBytes(int64(n.space.total)))
	}
	a.patch(s)
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return count(n) + " " + what + "s"
}

// count writes n with its thousands apart, as 100,000.
func count(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + count(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// readSpace reads the free space of the folder's volume in the
// background.
func (a *app) readSpace() {
	path := a.nav.path
	go func() {
		s, err := volumeSpace(path)
		a.post(func() {
			if samePath(a.nav.path, path) {
				a.nav.space, a.nav.spaceErr = s, err
				a.publishStatus()
			}
		})
	}()
}

// poll checks the folder showing for changes, in the background, and
// lists it again when it has changed.
func (a *app) poll() {
	n := &a.nav
	if n.loading || n.checking || n.path == "" {
		return
	}
	n.checking = true
	path, gen, mod, sig, small := n.path, n.gen, n.mod, n.sig, len(n.all) <= smallFolder
	failed := n.err != nil
	go func() {
		changed, es, newMod, err := checkFolder(a.ctx, path, mod, sig, small || failed)
		a.post(func() {
			n.checking = false
			if n.gen != gen || !samePath(n.path, path) {
				return
			}
			switch {
			case err != nil && failed:
				// It still cannot be read, and says so already.
			case err != nil || failed:
				a.listed(gen, path, es, newMod, err)
			case changed && es != nil:
				a.listed(gen, path, es, newMod, nil)
			case changed:
				a.relist()
			}
			a.readSpace()
		})
	}()
}

// checkFolder reports whether the folder at path changed since it was
// listed with time mod and signature sig. With full, it lists the folder
// and returns the entries.
func checkFolder(ctx context.Context, path string, mod time.Time, sig uint64, full bool) (bool, []entry, time.Time, error) {
	now, err := dirTime(path)
	if err != nil {
		return true, nil, now, err
	}
	if !full {
		return !now.Equal(mod), nil, now, nil
	}
	es, err := listDir(ctx, path)
	if err != nil {
		return true, nil, now, err
	}
	return !now.Equal(mod) || signature(es) != sig, es, now, nil
}
