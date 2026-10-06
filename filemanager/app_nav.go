package filemanager

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
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
	path      string
	back, fwd []visited
	// hop is a step back or forward to another file system, asked for
	// and not yet shown, and dropped are those the user went on from
	// before they showed, whose Show is not taken when it comes late.
	hop        *hop
	dropped    []hop
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
	// moves counts the folders gone to, so a path typed whose true case
	// comes after the user went elsewhere is left alone.
	moves int
}

// visited is a folder in the history, and the file system it is on.
type visited struct {
	fs   FS
	path string
}

// hop is a step through the history to a folder on another file system:
// step is -1 back and 1 forward.
type hop struct {
	id, path string
	step     int
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
		home, err := a.home()
		if err != nil {
			return
		}
		dir = home
	}
	a.navigate(dir, 0, true)
}

// home is the home folder of the file system showing, and says so in the
// window when it cannot be found.
func (a *app) home() (string, error) {
	home, err := a.fs.Home()
	if err != nil {
		a.fail("Finding your home folder: " + err.Error())
	}
	return home, err
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
		if v.Typed {
			a.goTyped(v.Path)
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
			a.goHistory(-1)
		}
	case CmdForward:
		if len(n.fwd) > 0 {
			a.goHistory(1)
		}
	case CmdUp:
		if parent := a.ps.Dir(n.path); parent != n.path {
			if a.newWindowAsked() {
				a.openWindow(parent)
				break
			}
			from := a.ps.Base(n.path)
			a.navigate(parent, -1, true)
			n.pick = from
		}
	case CmdHome:
		home, err := a.home()
		if err != nil {
			return true
		}
		a.navigate(home, 0, true)
	case CmdRefresh:
		a.relist()
	case CmdOpen:
		// The files open together, so a fetch to open them asks once.
		var files []string
		for _, e := range a.selectedEntries() {
			if e.Dir || e.Err != "" || e.Broken {
				a.activate(e)
				continue
			}
			files = append(files, a.ps.Join(a.nav.path, e.Name))
		}
		a.openFiles(files)
	case CmdSystemIcons:
		if !iconsHere {
			break
		}
		on := !a.systemIconsOn()
		a.prefs.SystemIcons = &on
		a.shell.SystemIcons = on
		a.publishShell()
		a.savePrefs(func(p *prefs) { p.SystemIcons = &on })
		a.keyIcons(a.nav.path, a.nav.all)
		a.refilter()
	case CmdHidden:
		a.shell.ShowHidden = !a.shell.ShowHidden
		show := a.shell.ShowHidden
		a.publishShell()
		a.savePrefs(func(p *prefs) { p.ShowHidden = show })
		a.refilter()
	case CmdSortName, CmdSortSize, CmdSortTime, CmdSortType:
		by := map[string]SortBy{CmdSortName: SortName, CmdSortSize: SortSize, CmdSortTime: SortModified, CmdSortType: SortType}[name]
		a.sortBy(by, false)
	default:
		return false
	}
	return true
}

// goHistory goes one step through the history: -1 back, 1 forward. A
// folder on another file system is asked for as a Visit, and the history
// steps once the window shows it, so a Visit that fails leaves it as it
// was.
func (a *app) goHistory(step int) {
	n := &a.nav
	from, to := &n.back, &n.fwd
	if step > 0 {
		from, to = &n.fwd, &n.back
	}
	at := (*from)[len(*from)-1]
	if id := at.fs.ID(); id != a.fs.ID() {
		n.dropHop()
		n.hop = &hop{id: id, path: at.path, step: step}
		if a.opts.Visit == nil {
			// Shown here before, so the window has the file system.
			a.showFS(at.fs, at.path)
			return
		}
		go a.opts.Visit(a.win, id, at.path, false)
		return
	}
	*from = (*from)[:len(*from)-1]
	if n.path != "" {
		*to = append(*to, visited{a.fs, n.path})
	}
	a.navigate(at.path, step, false)
}

// maxDropped is how many dropped hops a window keeps: a Visit that fails
// never shows, and its hop would stay forever.
const maxDropped = 8

// dropHop drops the hop on its way, as the user went on without it.
func (n *navState) dropHop() {
	if n.hop == nil {
		return
	}
	n.dropped = append(n.dropped, *n.hop)
	if len(n.dropped) > maxDropped {
		n.dropped = n.dropped[1:]
	}
	n.hop = nil
}

// late reports whether a Show of the folder at dir on the file system of
// ID id is a dropped hop's, come late, and forgets that hop.
func (n *navState) late(id, dir string) bool {
	i := slices.IndexFunc(n.dropped, func(h hop) bool { return h.id == id && h.path == dir })
	if i < 0 {
		return false
	}
	n.dropped = slices.Delete(n.dropped, i, i+1)
	return true
}

// hopped steps the history as hop h asked, now the window shows its
// folder, having left the folder at from. It reports false when the
// history no longer leads there.
func (a *app) hopped(h *hop, from visited) bool {
	n := &a.nav
	src, dst := &n.back, &n.fwd
	if h.step > 0 {
		src, dst = &n.fwd, &n.back
	}
	if len(*src) == 0 {
		return false
	}
	if at := (*src)[len(*src)-1]; at.fs.ID() != h.id || at.path != h.path {
		return false
	}
	*src = (*src)[:len(*src)-1]
	if from.path != "" {
		*dst = append(*dst, from)
	}
	return true
}

// navigate shows the folder at path. travel is the way the user went,
// and record keeps the folder left in the history.
func (a *app) navigate(path string, travel int, record bool) {
	n := &a.nav
	abs, err := a.ps.Abs(path)
	if err != nil {
		a.fail(fmt.Sprintf("Opening %s: %v", a.ps.Show(path), err))
		return
	}
	if record && n.path != "" && !a.ps.Same(abs, n.path) {
		n.back = append(n.back, visited{a.fs, n.path})
		n.fwd = nil
	}
	n.dropHop()
	if travel == 0 && n.path != "" {
		travel = direction(a.ps, n.path, abs)
	}
	n.path, n.travel = abs, travel
	// The window forgets the files' own icons of the folder left.
	for k := range a.iconsSent {
		if strings.HasPrefix(k, "file:") {
			delete(a.iconsSent, k)
		}
	}
	n.moves++
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

// goTyped shows the folder at the path the user typed. On a file system
// that can say how a path is really spelled, it goes there as spelled,
// once the file system says; where it cannot say, the path goes as typed,
// to show why there.
func (a *app) goTyped(typed string) {
	tc, ok := a.fs.(TrueCaser)
	abs, err := a.ps.Abs(typed)
	if !ok || err != nil {
		a.navigate(typed, 0, true)
		return
	}
	moves := a.nav.moves
	go func() {
		p, err := tc.TrueCase(abs)
		if err != nil || p == "" {
			p = abs
		}
		a.post(func() {
			if a.nav.moves == moves {
				a.navigate(p, 0, true)
			}
		})
	}()
}

// direction is 1 when to lies inside from, -1 when from lies inside to,
// and 0 otherwise, for paths of style ps.
func direction(ps PathStyle, from, to string) int {
	if rel, err := ps.Rel(from, to); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return 1
	}
	if rel, err := ps.Rel(to, from); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
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
	gen, path, fsys := n.gen, n.path, a.fs
	n.loading = true
	a.publishListing()
	go func() {
		mod, err := dirTime(fsys, path)
		var es []entry
		if err == nil {
			es, err = listDir(ctx, fsys, path)
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
	gen, fsys := n.gen, a.fs
	go func() {
		mod, err := dirTime(fsys, path)
		var es []entry
		if err == nil {
			es, err = listDir(ctx, fsys, path)
		}
		a.post(func() {
			if n.gen == gen && a.ps.Same(n.path, path) {
				a.listed(gen, path, es, mod, err)
			}
		})
	}()
}

// dirTime returns when the folder at path on fsys last changed.
func dirTime(fsys FS, path string) (time.Time, error) {
	info, err := fsys.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	if !info.IsDir() {
		return time.Time{}, fmt.Errorf("%s is not a folder", fsys.Paths().Show(path))
	}
	return info.ModTime(), nil
}

// listed takes a listing of path made for listing gen.
func (a *app) listed(gen int, path string, es []entry, mod time.Time, err error) {
	n := &a.nav
	if gen != n.gen || !a.ps.Same(path, n.path) {
		return
	}
	n.loading = false
	n.err = err
	if err != nil {
		n.all, n.rows = nil, nil
		if errors.Is(err, fs.ErrNotExist) {
			n.err = fmt.Errorf("%s no longer exists", a.ps.Show(path))
		}
		a.publishListing()
		a.publishStatus()
		return
	}
	n.mod, n.sig = mod, signature(es)
	a.keyIcons(path, es)
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
	by, desc := n.sort, n.desc
	a.savePrefs(func(p *prefs) { p.Sort, p.Desc = by, desc })
	sortEntries(n.all, n.sort, n.desc)
	a.refilter()
}

func (a *app) publishListing() {
	n := &a.nav
	l := Listing{
		Gen: n.gen, Path: n.path, Title: a.ps.placeName(n.path), Crumbs: crumbs(a.ps, n.path),
		Total: len(n.rows), All: len(n.all), Sort: n.sort, Desc: n.desc, Filter: n.filter,
		Travel: n.travel, Loading: n.loading,
		CanBack: len(n.back) > 0, CanForward: len(n.fwd) > 0, CanUp: a.ps.Dir(n.path) != n.path,
	}
	if n.err != nil {
		l.Err = n.err.Error()
	}
	a.patch(l)
}

// crumbs splits path, of style ps, into the folders along it.
func crumbs(ps PathStyle, path string) []Crumb {
	if ps == DrivePaths {
		parts := splitSlash(ps.Clean(path))
		out := make([]Crumb, 1, 1+len(parts))
		out[0] = Crumb{Name: drivesName, Path: "/"}
		for _, part := range parts {
			at := ps.Join(out[len(out)-1].Path, part)
			out = append(out, Crumb{Name: part, Path: at})
		}
		return out
	}
	vol := ps.VolumeName(path)
	rest := strings.TrimPrefix(path, vol)
	root := vol + ps.Sep()
	name := vol
	if name == "" {
		name = ps.Sep()
	}
	out := []Crumb{{Name: name, Path: root}}
	at := root
	for _, part := range strings.Split(strings.Trim(rest, ps.Sep()), ps.Sep()) {
		if part == "" {
			continue
		}
		at = ps.Join(at, part)
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
	r := Row{Name: e.Name, Kind: e.Kind, Dir: e.Dir, Hidden: e.Hidden, Broken: e.Broken, Online: e.Online, Cloud: e.Cloud, IconKey: e.iconKey, Type: e.Type,
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
		out[i] = a.ps.Join(a.nav.path, e.Name)
	}
	return out
}

// activate opens e: a folder in the window, and a file with its program.
func (a *app) activate(e entry) {
	path := a.ps.Join(a.nav.path, e.Name)
	if e.Dir && a.newWindowAsked() {
		a.openWindow(path)
		return
	}
	if e.Dir {
		a.navigate(path, 1, true)
		return
	}
	if e.Err != "" {
		a.fail(a.ps.Show(path) + " cannot be read: " + e.Err)
		return
	}
	if e.Broken {
		a.fail(a.ps.Show(path) + " is a link to something that is gone.")
		return
	}
	a.openFiles([]string{path})
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
// background, on a file system that knows it.
func (a *app) readSpace() {
	sr, ok := a.fs.(SpaceReporter)
	if !ok {
		return
	}
	path := a.nav.path
	go func() {
		var s space
		var err error
		s.free, s.total, err = sr.Space(path)
		if errors.Is(err, errors.ErrUnsupported) {
			// The file system cannot say after all, and the status bar
			// says nothing of it.
			s, err = space{}, nil
		}
		a.post(func() {
			if a.ps.Same(a.nav.path, path) {
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
	failed, fsys := n.err != nil, a.fs
	go func() {
		changed, es, newMod, err := checkFolder(a.ctx, fsys, path, mod, sig, small || failed)
		a.post(func() {
			n.checking = false
			if n.gen != gen || !a.ps.Same(n.path, path) {
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

// checkFolder reports whether the folder at path on fsys changed since it
// was listed with time mod and signature sig. With full, it lists the
// folder and returns the entries.
func checkFolder(ctx context.Context, fsys FS, path string, mod time.Time, sig uint64, full bool) (bool, []entry, time.Time, error) {
	now, err := dirTime(fsys, path)
	if err != nil {
		return true, nil, now, err
	}
	if !full {
		return !now.Equal(mod), nil, now, nil
	}
	es, err := listDir(ctx, fsys, path)
	if err != nil {
		return true, nil, now, err
	}
	return !now.Equal(mod) || signature(es) != sig, es, now, nil
}
