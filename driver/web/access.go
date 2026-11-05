//go:build js && wasm

package web

import (
	"html"
	"strconv"
	"strings"
	"syscall/js"
	"time"
	"unicode/utf8"

	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

// A browser doesn't say whether a screen reader is there, so the page holds every
// window's tree whoever is reading, and that must cost those with none next to
// nothing. A frame never pays for it unless something has moved for a long time:
//
//   - A window hands over its tree accessQuiet after its last frame, where a pause
//     costs nothing that shows, and that wakes no frame.
//   - While frames go on coming it waits accessLongest, with elements left where they
//     are until things rest, so what a scroll moves is moved once. A screen reader, or
//     someone moving about by keys, might be reading along, so for them it waits
//     accessMost. So does a first publish, for a page that never rests. Keys that type
//     don't count: gathering a long document's text between them would drop frames.
//   - A tree that came out the same doubles the wait, up to accessLongest, so a caret
//     costs little. The user's input ends that wait.
//   - As focus moves, and after a screen reader's own request, it doesn't wait.
const (
	accessQuiet    = 250 * time.Millisecond
	accessMost     = time.Second
	accessLongest  = 8 * time.Second
	accessKeyLasts = 10 * time.Second
)

// accessTextMost is how much of a field's text goes on its element. Chrome lays that
// text out again each time it changes, and the field with focus has all of its text
// in the text mirror, where a screen reader reads it.
const accessTextMost = 4096

// The attributes an element can have, in the order accessEl keeps them.
const (
	attrRole = iota
	attrLabel
	attrDescription
	attrChecked
	attrPressed
	attrSelected
	attrCurrent
	attrExpanded
	attrDisabled
	attrReadOnly
	attrMultiline
	attrHasPopup
	attrModal
	attrLevel
	attrValueMin
	attrValueMax
	attrValueStep
	attrValueNow
	attrValueText
	numAttrs
)

var attrNames = [numAttrs]string{
	"role", "aria-label", "aria-description", "aria-checked", "aria-pressed", "aria-selected", "aria-current",
	"aria-expanded", "aria-disabled", "aria-readonly", "aria-multiline", "aria-haspopup", "aria-modal", "aria-level",
	"aria-valuemin", "aria-valuemax", "", "aria-valuenow", "aria-valuetext",
}

// rangeAttrs is what a slider's input calls the attributes that differ.
var rangeAttrs = map[int]string{attrValueMin: "min", attrValueMax: "max", attrValueStep: "step", attrValueNow: "value"}

// roleNames is each role's ARIA role. A role left out here is decided in describe: a
// group has one only with a name, as a nameless one is only layout, and a label is
// plain text.
var roleNames = [...]string{
	access.RoleButton: "button", access.RoleCheckbox: "checkbox", access.RoleSwitch: "switch",
	access.RoleSlider: "slider", access.RoleTextField: "textbox", access.RoleHeading: "heading",
	access.RoleImage: "img", access.RoleList: "list", access.RoleListItem: "listitem",
	access.RoleTabList: "tablist", access.RoleTab: "tab", access.RoleMenu: "menu", access.RoleMenuItem: "menuitem",
	access.RoleComboBox: "combobox", access.RoleDialog: "dialog", access.RoleTooltip: "tooltip",
	access.RoleLink: "link", access.RoleTable: "grid", access.RoleRow: "row", access.RoleCell: "gridcell",
	access.RoleColumnHeader: "columnheader", access.RoleMenuBar: "menubar", access.RoleProgressBar: "progressbar",
	access.RoleScrollBar: "scrollbar", access.RoleScrollArea: "",
}

// accessEl is a node's element, and what the element shows now, so a publish that
// changes nothing touches nothing on the page. el is found the first time it's needed,
// as most elements are made together from one string and most never change.
type accessEl struct {
	el    js.Value
	id    string
	attrs [numAttrs]string
	text  string
	at    geom.Rect
	kids  []uint64
	// seen is the publish that last held the node.
	seen uint64
	// isRange says the element is a range input, for a slider, and inSight that it was
	// last placed where the window shows it.
	isRange, inSight bool
	// What a screen reader can ask of it.
	press, focusable bool
	role             access.Role
}

func (e *accessEl) element() js.Value {
	if !e.el.Truthy() {
		e.el = js.Global().Get("document").Call("getElementById", e.id)
	}
	return e.el
}

// canvasAttrs are what the driver sets on the canvas, and pageAttrs what the page had
// there, to put back.
var canvasAttrs = [...]string{"role", "aria-label", "aria-owns", "aria-activedescendant"}

const (
	canvasRole = iota
	canvasLabel
	canvasOwns
	canvasFocus
)

// accessLayerLocked returns the element the windows' trees go in, made the first time. It
// lies over the canvas, unseen and out of the pointer's way: it is there for what
// reads the page's elements. A role and a name the page gave the canvas stay. It runs
// with mu held.
func (d *Driver) accessLayerLocked() js.Value {
	if d.accessRoot.Truthy() {
		return d.accessRoot
	}
	doc := js.Global().Get("document")
	root := doc.Call("createElement", "div")
	root.Set("id", "gunim-access")
	root.Get("style").Set("cssText", "position:fixed;opacity:0;pointer-events:none;overflow:hidden;contain:strict;user-select:none;-webkit-user-select:none")
	doc.Get("body").Call("appendChild", root)
	d.accessRoot = root
	for i, name := range canvasAttrs {
		d.pageAttrs[i] = d.canvas.Call("getAttribute", name)
	}
	if d.pageAttrs[canvasRole].IsNull() {
		d.canvas.Call("setAttribute", "role", "application")
	}
	owns := "gunim-access"
	if p := d.pageAttrs[canvasOwns]; !p.IsNull() {
		owns = p.String() + " " + owns
	}
	d.canvas.Call("setAttribute", "aria-owns", owns)
	d.on(root, "click", false, d.accessClick)
	d.on(root, "input", false, d.accessInput)
	d.on(root, "focusin", false, d.accessFocused)
	d.placeAccessLocked()
	return root
}

// restoreCanvasAttr puts back what the page had in attribute i, or nothing.
func (d *Driver) restoreCanvasAttr(i int) {
	if p := d.pageAttrs[i]; p.Type() == js.TypeString {
		d.canvas.Call("setAttribute", canvasAttrs[i], p)
	} else {
		d.canvas.Call("removeAttribute", canvasAttrs[i])
	}
}

// placeAccess keeps the layer over the canvas, as the page moves it. A page with no
// layer yet pays nothing.
func (d *Driver) placeAccess() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.placeAccessLocked()
}

func (d *Driver) placeAccessLocked() {
	if !d.accessRoot.Truthy() {
		return
	}
	r := d.canvas.Call("getBoundingClientRect")
	at := geom.Rect{
		Min: geom.Pt(float32(r.Get("left").Float()), float32(r.Get("top").Float())),
		Max: geom.Pt(float32(r.Get("right").Float()), float32(r.Get("bottom").Float())),
	}
	if at != d.accessAt {
		d.accessAt = at
		s := d.accessRoot.Get("style")
		s.Set("left", cssPx(at.Min.X))
		s.Set("top", cssPx(at.Min.Y))
		s.Set("width", cssPx(at.Max.X-at.Min.X))
		s.Set("height", cssPx(at.Max.Y-at.Min.Y))
	}
}

// placeCSS is what puts an element at r, in its parent's space.
func placeCSS(r geom.Rect) string {
	return "position:absolute;left:" + cssPx(r.Min.X) + ";top:" + cssPx(r.Min.Y) + ";width:" + cssPx(r.Max.X-r.Min.X) + ";height:" + cssPx(r.Max.Y-r.Min.Y)
}

func cssPx(v float32) string { return strconv.FormatFloat(float64(v), 'f', -1, 32) + "px" }

// AccessWanted implements [driver.AccessPublisher]. The engine asks after each frame,
// so this is also how the window hears that one was drawn.
func (w *Window) AccessWanted() bool {
	d := w.d
	d.mu.Lock()
	defer d.mu.Unlock()
	if w.hidden || w.closed || d.stopped {
		return false
	}
	if w.accessSoon {
		return true
	}
	now := time.Now()
	moving := now.Sub(w.accessAsked) < accessQuiet
	w.accessAsked = now
	if w.accessOwed == nil {
		w.accessOwed = time.AfterFunc(accessQuiet, w.accessRested)
	}
	wait := accessLongest
	if w.accessEls == nil || d.accessUsed || now.Sub(d.accessKeyAt) < accessKeyLasts {
		wait = max(accessMost, w.accessGap)
	}
	return moving && now.Sub(w.accessLast) >= wait
}

// AccessFocusWanted implements [driver.AccessFocusTaker].
func (w *Window) AccessFocusWanted() bool {
	w.d.mu.Lock()
	defer w.d.mu.Unlock()
	return !w.hidden && !w.closed && !w.d.stopped
}

// accessRested runs once a frame's wait is over. With no frame since, and no reason to
// wait on, it asks for the tree, which wakes no frame.
func (w *Window) accessRested() {
	d := w.d
	d.mu.Lock()
	defer d.mu.Unlock()
	w.accessOwed = nil
	if w.closed || d.stopped {
		return
	}
	wait := accessQuiet - time.Since(w.accessAsked)
	if !w.accessUnplaced {
		wait = max(wait, w.accessGap-time.Since(w.accessLast))
	}
	if wait > 0 {
		w.accessOwed = time.AfterFunc(wait, w.accessRested)
		return
	}
	w.accessSoon = true
	w.in.Push(driver.AccessDue{})
}

// accessInputCame ends the wait a tree that came out the same put in place, as the
// user's input can change anything. It runs with mu held.
func (w *Window) accessInputCame() { w.accessGap = 0 }

// PublishAccess implements [driver.AccessPublisher].
func (w *Window) PublishAccess(t *access.Tree) {
	d := w.d
	d.mu.Lock()
	defer d.mu.Unlock()
	if w.hidden || w.closed || d.stopped || t == nil || t.Root == nil {
		return
	}
	now := time.Now()
	w.accessMoving = now.Sub(w.accessAsked) < accessQuiet
	w.accessLast, w.accessSoon = now, false
	w.accessChanged, w.accessHot = false, 0
	layer := d.accessLayerLocked()
	if w.accessEls == nil {
		w.accessEls = map[uint64]*accessEl{}
		top := js.Global().Get("document").Call("createElement", "div")
		top.Get("style").Set("cssText", "position:absolute")
		layer.Call("appendChild", top)
		w.accessTop = &accessEl{el: top}
	}
	if !w.accessMoving {
		d.placeAccessLocked()
	}
	w.accessSight = geom.Rect{Max: w.rectLocked().Size().Point()}
	w.accessSeen++
	if at := w.rectLocked(); at != w.accessTop.at {
		w.accessTop.at = at
		w.accessTop.el.Call("setAttribute", "style", placeCSS(at))
	}
	if w.fills && t.Root.Name != d.accessTitle && d.pageAttrs[canvasLabel].IsNull() {
		d.accessTitle = t.Root.Name
		d.canvas.Call("setAttribute", "aria-label", t.Root.Name)
	}
	w.accessOne[0] = t.Root
	w.showKids(w.accessTop, w.accessOne[:], geom.Point{})
	for id, e := range w.accessEls {
		if e.seen != w.accessSeen {
			w.accessChanged = true
			// One whose parent has gone went with it.
			if el := e.element(); el.Truthy() {
				el.Call("remove")
			}
			delete(w.accessEls, id)
		}
	}
	d.showFocusLocked(w, t)
	if !w.accessMoving {
		w.accessUnplaced = false
	}
	if w.accessChanged || d.accessUsed {
		w.accessGap = 0
	} else {
		w.accessGap = min(max(accessMost, 2*w.accessGap), accessLongest)
	}
}

// showKids brings the elements of kids up to date under parent, and those under them.
// from is where parent lies on the window. New ones go in together, from one string.
func (w *Window) showKids(parent *accessEl, kids []*access.Node, from geom.Point) {
	var fresh strings.Builder
	var made []uint64
	same := len(parent.kids) == len(kids)
	for i, k := range kids {
		if e := w.accessEls[k.ID]; e != nil {
			w.showAccess(e, k, from)
		} else {
			w.writeNew(&fresh, k, from)
			made = append(made, k.ID)
		}
		same = same && parent.kids[i] == k.ID
	}
	if fresh.Len() > 0 {
		parent.element().Call("insertAdjacentHTML", "beforeend", fresh.String())
	}
	if same {
		return
	}
	w.accessChanged = true
	// The page holds those of the old kids still here, in order, then the new ones.
	in := make([]uint64, 0, len(kids))
	for _, id := range parent.kids {
		if e := w.accessEls[id]; e != nil && e.seen == w.accessSeen {
			in = append(in, id)
		}
	}
	in = append(in, made...)
	ordered := len(in) == len(kids)
	for i, k := range kids {
		ordered = ordered && in[i] == k.ID
	}
	parent.kids = parent.kids[:0]
	for _, k := range kids {
		parent.kids = append(parent.kids, k.ID)
		if !ordered {
			parent.element().Call("appendChild", w.accessEls[k.ID].element())
		}
	}
}

// showAccess brings n's element and those under it up to date. from is where its
// parent's element lies on the window. While things move, an element stays where it is
// and is placed once they rest.
func (w *Window) showAccess(e *accessEl, n *access.Node, from geom.Point) {
	e.seen = w.accessSeen
	w.noteHot(n)
	e.show(n, w)
	if at := n.Bounds.Add(geom.Pt(-from.X, -from.Y)); at != e.at {
		// What scrolled from out of sight to out of sight stays where it is.
		inSight := overlaps(w.accessSight, n.Bounds)
		switch {
		case w.accessMoving:
			w.accessUnplaced = true
		case inSight || e.inSight:
			e.at, e.inSight = at, inSight
			w.accessChanged = true
			e.element().Call("setAttribute", "style", placeCSS(at))
		}
	}
	if len(n.Children) > 0 || len(e.kids) > 0 {
		w.showKids(e, n.Children, n.Bounds.Min)
	}
}

// noteHot notes the menu item a window has lit up, which the canvas names as it does
// the focused node: Chrome shows no selected state on a menu item.
func (w *Window) noteHot(n *access.Node) {
	if n.Role == access.RoleMenuItem && n.State.Has(access.StateSelected) {
		w.accessHot = n.ID
	}
}

// writeNew adds to b the elements of n and all under it, which are new.
func (w *Window) writeNew(b *strings.Builder, n *access.Node, from geom.Point) {
	e := &accessEl{id: accessID(w, n.ID), seen: w.accessSeen}
	w.accessEls[n.ID] = e
	w.accessChanged = true
	w.noteHot(n)
	var a [numAttrs]string
	e.isRange = n.Role == access.RoleSlider && n.Range != nil && len(n.Children) == 0
	e.text = describe(n, e.isRange, &a)
	e.attrs = a
	e.remember(n)
	e.at = n.Bounds.Add(geom.Pt(-from.X, -from.Y))
	e.inSight = overlaps(w.accessSight, n.Bounds)
	tag := "div"
	if e.isRange {
		tag = `input type="range" tabindex="-1"`
	}
	b.WriteString("<" + tag + ` id="` + e.id + `" data-gunim-id="` + strconv.FormatUint(n.ID, 10) + `" style="` + placeCSS(e.at) + `"`)
	for i, v := range a {
		if v != "" {
			b.WriteString(" " + e.attrName(i) + `="` + html.EscapeString(v) + `"`)
		}
	}
	b.WriteString(">")
	if e.isRange {
		return
	}
	b.WriteString(html.EscapeString(e.text))
	for _, k := range n.Children {
		e.kids = append(e.kids, k.ID)
		w.writeNew(b, k, n.Bounds.Min)
	}
	b.WriteString("</div>")
}

// attrName is what attribute i is called on this element.
func (e *accessEl) attrName(i int) string {
	if e.isRange {
		if name, ok := rangeAttrs[i]; ok {
			return name
		}
	}
	return attrNames[i]
}

// accessID is the element id of node id in w. Each window numbers its own nodes.
func accessID(w *Window, id uint64) string {
	return "gunim-a" + strconv.FormatUint(w.accessNum, 10) + "-" + strconv.FormatUint(id, 10)
}

// describe fills a with what an element says for n, and returns the text it holds. It
// says each thing the way Chrome reads it for the role. Chrome drops the rest, whatever
// ARIA allows, and web_test checks this against Chrome's own tree.
func describe(n *access.Node, isRange bool, a *[numAttrs]string) string {
	s := n.State
	if int(n.Role) < len(roleNames) {
		a[attrRole] = roleNames[n.Role]
	}
	checked := s.Has(access.StateChecked)
	checkable := s.Has(access.StateCheckable) || checked
	press := hasPress(n)
	switch {
	case n.Role == access.RoleMenuItem && checkable:
		a[attrRole] = "menuitemcheckbox"
	case n.Role == access.RoleList && listPicks(n):
		a[attrRole] = "listbox"
	case n.Role == access.RoleListItem && press:
		// A tile: an item that is picked, which a plain list item can't say.
		a[attrRole] = "option"
	case n.Role == access.RoleLabel && press:
		a[attrRole] = "button"
	case a[attrRole] == "" && n.Name != "" && n.Role != access.RoleLabel:
		// A window, a scroll area or a group of its own name.
		a[attrRole] = "group"
	}
	if isRange {
		a[attrRole] = ""
	}
	// A label is its own text, and so is a leaf that holds its value as text.
	text := ""
	leaf := len(n.Children) == 0
	switch {
	case n.Role == access.RoleLabel && leaf && a[attrRole] == "":
		text = n.Name
	case n.Role == access.RoleTextField && leaf:
		text = capText(n.Value)
		a[attrLabel] = n.Name
	case a[attrRole] != "" || isRange:
		a[attrLabel] = n.Name
	}
	a[attrDescription] = n.Description
	if checkable {
		if a[attrRole] == "button" {
			a[attrPressed] = strconv.FormatBool(checked)
		} else {
			a[attrChecked] = strconv.FormatBool(checked)
		}
	}
	if s.Has(access.StateSelected) {
		switch a[attrRole] {
		case "tab", "row", "gridcell", "columnheader", "option":
			a[attrSelected] = "true"
		case "menuitem", "menuitemcheckbox":
			// The canvas names it: see noteHot.
		default:
			a[attrCurrent] = "true"
		}
	} else if a[attrRole] == "option" || a[attrRole] == "tab" {
		a[attrSelected] = "false"
	}
	if s.Has(access.StateExpandable) {
		a[attrExpanded] = strconv.FormatBool(s.Has(access.StateExpanded))
	}
	on := func(attr int, state access.State) {
		if s.Has(state) {
			a[attr] = "true"
		}
	}
	on(attrDisabled, access.StateDisabled)
	on(attrReadOnly, access.StateReadOnly)
	on(attrMultiline, access.StateMultiline)
	on(attrHasPopup, access.StateHasPopup)
	on(attrModal, access.StateModal)
	if n.Role == access.RoleHeading {
		a[attrLevel] = "2"
	}
	if r := n.Range; r != nil {
		a[attrValueMin] = strconv.FormatFloat(r.Min, 'f', -1, 64)
		a[attrValueMax] = strconv.FormatFloat(r.Max, 'f', -1, 64)
		a[attrValueNow] = strconv.FormatFloat(r.Value, 'f', -1, 64)
		if isRange {
			a[attrValueStep] = "any"
			if r.Step > 0 {
				a[attrValueStep] = strconv.FormatFloat(r.Step, 'f', -1, 64)
			}
		}
	}
	switch {
	case n.Role == access.RoleTextField:
	case n.Range != nil:
		a[attrValueText] = n.Value
	default:
		// Chrome reads a value text only on something with a range, so anywhere else, as
		// on a combo box, the value goes in the name.
		if n.Value != "" {
			if n.Name == "" || a[attrLabel] == "" {
				a[attrLabel] = n.Value
			} else {
				a[attrLabel] = n.Name + ": " + n.Value
			}
		}
	}
	return text
}

func hasPress(n *access.Node) bool {
	for _, act := range n.Actions {
		if act == access.ActionPress {
			return true
		}
	}
	return false
}

// listPicks says a list's items are picked by pressing them.
func listPicks(n *access.Node) bool {
	for _, k := range n.Children {
		if k.Role == access.RoleListItem && hasPress(k) {
			return true
		}
	}
	return false
}

// capText is s cut to accessTextMost bytes, whole characters only.
func capText(s string) string {
	if len(s) <= accessTextMost {
		return s
	}
	at := accessTextMost
	for at > 0 && !utf8.RuneStart(s[at]) {
		at--
	}
	return s[:at] + "…"
}

// remember keeps what a screen reader can ask of n's element.
func (e *accessEl) remember(n *access.Node) {
	e.press, e.focusable, e.role = hasPress(n), n.Focusable, n.Role
}

// show sets what the element says to what n says.
func (e *accessEl) show(n *access.Node, w *Window) {
	var a [numAttrs]string
	isRange := n.Role == access.RoleSlider && n.Range != nil && len(n.Children) == 0
	text := describe(n, isRange && e.isRange, &a)
	for i, v := range a {
		if v == e.attrs[i] {
			continue
		}
		w.accessChanged = true
		e.attrs[i] = v
		switch {
		case e.isRange && i == attrValueNow:
			e.element().Set("value", v)
		case e.attrName(i) == "":
		case v == "":
			e.element().Call("removeAttribute", e.attrName(i))
		default:
			e.element().Call("setAttribute", e.attrName(i), v)
		}
	}
	// A field's element keeps its text while it has focus: the mirror has the text, and
	// each key would have Chrome lay it out again.
	typing := n.Focused && n.Role == access.RoleTextField
	if text != e.text && !typing {
		w.accessChanged = true
		e.text = text
		e.element().Set("textContent", text)
	}
	e.remember(n)
}

// showFocusLocked tells the page where gunim's focus is. The canvas keeps the
// browser's own focus, and names the focused node's element, or the menu item a popup
// has lit up. A field the text mirror has the browser's focus for is the mirror: a
// screen reader reads the real text and caret from it, and a secret stays a password
// field. It writes to the page only what changed.
func (d *Driver) showFocusLocked(w *Window, t *access.Tree) {
	field := t.Focus != nil && t.Focus.Role == access.RoleTextField && d.text.Equal(js.Global().Get("document").Get("activeElement"))
	switch {
	case w.accessHot != 0:
		d.accessHotIn = w
		d.setFocusNameLocked(w, accessID(w, w.accessHot))
	case d.accessHotIn == w:
		d.accessHotIn = nil
		d.setFocusNameLocked(w, "")
	}
	if d.accessHotIn == nil {
		switch {
		case t.Focus != nil && !field:
			d.setFocusNameLocked(w, accessID(w, t.Focus.ID))
		case d.accessFocusIn == w || field:
			d.setFocusNameLocked(w, "")
		}
	}
	if field {
		d.accessFieldIn = w
		d.labelMirrorLocked(t.Focus.Name)
	} else if d.accessFieldIn == w {
		d.accessFieldIn = nil
		d.labelMirrorLocked("")
	}
}

// labelMirrorLocked names the text mirror for the field it stands for, or for none.
func (d *Driver) labelMirrorLocked(name string) {
	if name == d.mirrorLabel && (name == "" || d.mirrorNamed.Equal(d.text)) {
		return
	}
	if d.mirrorNamed.Truthy() {
		d.mirrorNamed.Call("removeAttribute", "aria-label")
	}
	d.mirrorLabel, d.mirrorNamed = name, js.Undefined()
	if name != "" {
		d.mirrorNamed = d.text
		d.text.Call("setAttribute", "aria-label", name)
	}
}

// focusText gives the text mirror the browser's focus. It says so to assistive
// technology first: a focused element can't be hidden from it.
func (d *Driver) focusText() {
	d.text.Call("removeAttribute", "aria-hidden")
	d.text.Call("focus", map[string]any{"preventScroll": true})
}

// mirrorBlurred hides a text mirror from assistive technology again.
func (d *Driver) mirrorBlurred(m js.Value) {
	m.Call("setAttribute", "aria-hidden", "true")
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.mirrorNamed.Truthy() && d.mirrorNamed.Equal(m) {
		d.labelMirrorLocked("")
	}
}

func (d *Driver) setFocusNameLocked(w *Window, id string) {
	if id == d.accessFocus {
		d.accessFocusIn = w
		return
	}
	d.accessFocus, d.accessFocusIn = id, w
	if id == "" {
		d.restoreCanvasAttr(canvasFocus)
	} else {
		d.canvas.Call("setAttribute", "aria-activedescendant", id)
	}
}

// accessTarget finds the window and the node of the element e happened on, or the
// nearest around it.
func (d *Driver) accessTarget(e js.Value) (*Window, *accessEl, uint64) {
	el := e.Get("target").Call("closest", "[data-gunim-id]")
	if el.IsNull() {
		return nil, nil, 0
	}
	id, err := strconv.ParseUint(el.Call("getAttribute", "data-gunim-id").String(), 10, 64)
	if err != nil {
		return nil, nil, 0
	}
	sid := el.Get("id").String()
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, w := range d.windows {
		if x := w.accessEls[id]; x != nil && x.id == sid {
			return w, x, id
		}
	}
	return nil, nil, 0
}

// accessClick takes a click a screen reader makes on an element, which is how it
// presses what the element stands for.
func (d *Driver) accessClick(e js.Value) {
	w, x, id := d.accessTarget(e)
	if w == nil {
		return
	}
	e.Call("preventDefault")
	d.mu.Lock()
	w.accessSoon, d.accessUsed = true, true
	w.accessGap = 0
	if x.role == access.RoleTextField {
		// The click is what lets a phone show its keyboard for the field.
		d.lastTouch = false
	}
	d.mu.Unlock()
	// Focus moves as for a real click, so the keys go there next, as Escape to a menu
	// it opens.
	r := access.Request{ID: id, Focus: x.focusable}
	if x.press {
		r.Action = access.ActionPress
	}
	if r.Focus || x.press {
		w.in.Push(r)
	}
}

// accessInput takes a new value a screen reader gave a slider. The browser moves the
// input at once, so what it showed is forgotten: the next publish says what the widget
// made of it.
func (d *Driver) accessInput(e js.Value) {
	w, x, id := d.accessTarget(e)
	if w == nil || !x.isRange {
		return
	}
	v := e.Get("target").Get("valueAsNumber").Float()
	d.mu.Lock()
	w.accessSoon, d.accessUsed = true, true
	w.accessGap = 0
	x.attrs[attrValueNow] = ""
	d.mu.Unlock()
	w.in.Push(access.Request{ID: id, Value: v, SetValue: true})
}

// accessFocused gives the canvas the browser's focus back, as keys go to it, when a
// screen reader moves focus onto a slider's input. gunim's focus moves there instead.
func (d *Driver) accessFocused(e js.Value) {
	w, x, id := d.accessTarget(e)
	d.canvas.Call("focus", map[string]any{"preventScroll": true})
	if w == nil {
		return
	}
	d.mu.Lock()
	w.accessSoon, d.accessUsed = true, true
	w.accessGap = 0
	d.mu.Unlock()
	if x.focusable {
		w.in.Push(access.Request{ID: id, Focus: true})
	}
}

// clearAccessFocusLocked takes back what the page says of w's focus, as w goes away
// or out of sight.
func (d *Driver) clearAccessFocusLocked(w *Window) {
	if d.accessHotIn == w {
		d.accessHotIn = nil
	}
	if d.accessFocusIn == w {
		d.setFocusNameLocked(w, "")
	}
	if d.accessFieldIn == w {
		d.accessFieldIn = nil
		d.labelMirrorLocked("")
	}
}

// forgetAccessLocked takes a closed window's tree off the page. It runs with mu held.
func (d *Driver) forgetAccessLocked(w *Window) {
	if w.accessOwed != nil {
		w.accessOwed.Stop()
		w.accessOwed = nil
	}
	if w.accessTop != nil {
		w.accessTop.el.Call("remove")
	}
	w.accessEls = nil
	d.clearAccessFocusLocked(w)
}

var (
	_ driver.AccessPublisher  = (*Window)(nil)
	_ driver.AccessFocusTaker = (*Window)(nil)
)

// overlaps says a and b share any point.
func overlaps(a, b geom.Rect) bool {
	return a.Min.X < b.Max.X && b.Min.X < a.Max.X && a.Min.Y < b.Max.Y && b.Min.Y < a.Max.Y
}
