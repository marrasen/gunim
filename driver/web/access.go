//go:build js && wasm

package web

import (
	"strconv"
	"syscall/js"
	"time"

	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

// A browser doesn't say whether a screen reader is there, so the page holds every
// window's tree whoever is reading, and that must cost those with none next to
// nothing. So a window hands over its tree once things have come to rest, which
// is accessQuiet after the last frame, where a pause costs nothing that shows. While
// something goes on moving, it hands it over every accessMost. A key press or a
// screen reader's own request doesn't wait.
const (
	accessQuiet = 250 * time.Millisecond
	accessMost  = time.Second
)

// The attributes an element can have, in the order accessEl keeps them.
const (
	attrRole = iota
	attrLabel
	attrDescription
	attrChecked
	attrSelected
	attrExpanded
	attrDisabled
	attrReadOnly
	attrMultiline
	attrHasPopup
	attrModal
	attrLevel
	attrValueMin
	attrValueMax
	attrValueNow
	attrValueText
	numAttrs
)

var attrNames = [numAttrs]string{
	"role", "aria-label", "aria-description", "aria-checked", "aria-selected", "aria-expanded", "aria-disabled",
	"aria-readonly", "aria-multiline", "aria-haspopup", "aria-modal", "aria-level", "aria-valuemin", "aria-valuemax",
	"aria-valuenow", "aria-valuetext",
}

// roleNames is each role's ARIA role. A role left out here is decided in
// [accessEl.show]: a group has one only with a name, as a nameless one is only
// layout, and a label is plain text.
var roleNames = map[access.Role]string{
	access.RoleButton: "button", access.RoleCheckbox: "checkbox", access.RoleSwitch: "switch",
	access.RoleSlider: "slider", access.RoleTextField: "textbox", access.RoleHeading: "heading",
	access.RoleImage: "img", access.RoleList: "list", access.RoleListItem: "listitem",
	access.RoleTabList: "tablist", access.RoleTab: "tab", access.RoleMenu: "menu", access.RoleMenuItem: "menuitem",
	access.RoleComboBox: "combobox", access.RoleDialog: "dialog", access.RoleTooltip: "tooltip",
	access.RoleLink: "link", access.RoleTable: "table", access.RoleRow: "row", access.RoleCell: "cell",
	access.RoleColumnHeader: "columnheader", access.RoleMenuBar: "menubar", access.RoleProgressBar: "progressbar",
	access.RoleScrollBar: "scrollbar",
}

// accessEl is a node's element, and what the element shows now, so a publish that
// changes nothing touches nothing on the page.
type accessEl struct {
	el    js.Value
	attrs [numAttrs]string
	text  string
	at    geom.Rect
	kids  []uint64
	// seen is the publish that last held the node.
	seen uint64
	// What a screen reader can ask of it.
	press, focusable bool
	role             access.Role
	rng              access.Range
}

// accessLayerLocked returns the element the windows' trees go in, made the first time. It
// lies over the canvas, unseen and out of the pointer's way: it is there for what
// reads the page's elements. It runs with mu held.
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
	d.canvas.Call("setAttribute", "role", "application")
	d.canvas.Call("setAttribute", "aria-owns", "gunim-access")
	d.on(root, "click", false, d.accessClick)
	d.on(root, "keydown", false, d.accessKey)
	d.placeAccessLocked()
	return root
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
		place(d.accessRoot, at)
	}
}

// place puts an element at r, in its parent's space.
func place(el js.Value, r geom.Rect) {
	s := el.Get("style")
	s.Set("left", cssPx(r.Min.X))
	s.Set("top", cssPx(r.Min.Y))
	s.Set("width", cssPx(r.Max.X-r.Min.X))
	s.Set("height", cssPx(r.Max.Y-r.Min.Y))
}

func cssPx(v float32) string { return strconv.FormatFloat(float64(v), 'f', -1, 32) + "px" }

// AccessWanted implements [driver.AccessPublisher]. A frame turned down leaves a
// redraw for when things have come to rest, so the page catches up.
func (w *Window) AccessWanted() bool {
	d := w.d
	d.mu.Lock()
	defer d.mu.Unlock()
	if w.hidden || w.closed || d.stopped {
		return false
	}
	now := time.Now()
	if w.accessSoon || now.Sub(w.accessLast) >= accessMost {
		return true
	}
	w.accessAsked = now
	if w.accessOwed == nil {
		w.accessOwed = time.AfterFunc(accessQuiet, w.accessRested)
	}
	return false
}

// accessRested runs once a turned-down frame's wait is over. With no frame since, it
// asks for one to hand over the tree in, and otherwise waits on.
func (w *Window) accessRested() {
	d := w.d
	d.mu.Lock()
	defer d.mu.Unlock()
	w.accessOwed = nil
	if w.closed {
		return
	}
	if wait := accessQuiet - time.Since(w.accessAsked); wait > 0 {
		w.accessOwed = time.AfterFunc(wait, w.accessRested)
		return
	}
	w.accessSoon = true
	w.in.Push(driver.Redraw{})
}

// PublishAccess implements [driver.AccessPublisher].
func (w *Window) PublishAccess(t *access.Tree) {
	d := w.d
	d.mu.Lock()
	defer d.mu.Unlock()
	if w.closed || d.stopped || t == nil || t.Root == nil {
		return
	}
	w.accessLast, w.accessSoon = time.Now(), false
	layer := d.accessLayerLocked()
	if w.accessEls == nil {
		w.accessEls = map[uint64]*accessEl{}
		w.accessTop = js.Global().Get("document").Call("createElement", "div")
		w.accessTop.Get("style").Set("cssText", "position:absolute")
		layer.Call("appendChild", w.accessTop)
	}
	d.placeAccessLocked()
	w.accessSeen++
	if at := w.rectLocked(); at != w.accessPlace {
		w.accessPlace = at
		place(w.accessTop, at)
	}
	if w.fills && t.Root.Name != d.accessTitle {
		d.accessTitle = t.Root.Name
		d.canvas.Call("setAttribute", "aria-label", t.Root.Name)
	}
	root := w.showAccess(t.Root, geom.Point{})
	if len(w.accessKids) != 1 || w.accessKids[0] != t.Root.ID {
		w.accessKids = append(w.accessKids[:0], t.Root.ID)
		w.accessTop.Call("appendChild", root.el)
	}
	for id, e := range w.accessEls {
		if e.seen != w.accessSeen {
			e.el.Call("remove")
			delete(w.accessEls, id)
		}
	}
	d.showFocusLocked(w, t)
}

// showAccess brings n's element and those under it up to date, and returns it. from
// is where its parent's element is on the window.
func (w *Window) showAccess(n *access.Node, from geom.Point) *accessEl {
	e := w.accessEls[n.ID]
	if e == nil {
		el := js.Global().Get("document").Call("createElement", "div")
		el.Get("style").Set("cssText", "position:absolute;margin:0;padding:0")
		el.Call("setAttribute", "data-gunim-id", strconv.FormatUint(n.ID, 10))
		// aria-activedescendant names an element by its id.
		el.Set("id", accessID(w, n.ID))
		e = &accessEl{el: el}
		w.accessEls[n.ID] = e
	}
	e.seen = w.accessSeen
	e.show(n)
	if at := n.Bounds.Add(geom.Pt(-from.X, -from.Y)); at != e.at {
		e.at = at
		place(e.el, at)
	}
	same := len(e.kids) == len(n.Children)
	for i, k := range n.Children {
		w.showAccess(k, n.Bounds.Min)
		same = same && e.kids[i] == k.ID
	}
	if !same {
		e.kids = e.kids[:0]
		for _, k := range n.Children {
			e.kids = append(e.kids, k.ID)
			e.el.Call("appendChild", w.accessEls[k.ID].el)
		}
	}
	return e
}

// accessID is the element id of node id in w. Each window numbers its own nodes.
func accessID(w *Window, id uint64) string {
	return "gunim-a" + strconv.FormatUint(w.accessNum, 10) + "-" + strconv.FormatUint(id, 10)
}

// show sets what the element says to what n says.
func (e *accessEl) show(n *access.Node) {
	var a [numAttrs]string
	s := n.State
	a[attrRole] = roleNames[n.Role]
	if n.Role == access.RoleMenuItem && s.Has(access.StateCheckable) {
		a[attrRole] = "menuitemcheckbox"
	}
	if a[attrRole] == "" && n.Name != "" && n.Role != access.RoleLabel {
		// A window, a scroll area or a group of its own name.
		a[attrRole] = "group"
	}
	// A label is its own text, and so is a leaf that holds its value as text.
	text := ""
	leaf := len(n.Children) == 0
	switch {
	case n.Role == access.RoleLabel && leaf:
		text = n.Name
	case n.Role == access.RoleTextField && leaf:
		text = n.Value
		a[attrLabel] = n.Name
	case a[attrRole] != "":
		a[attrLabel] = n.Name
	}
	a[attrDescription] = n.Description
	if s.Has(access.StateCheckable) {
		a[attrChecked] = strconv.FormatBool(s.Has(access.StateChecked))
	}
	if s.Has(access.StateSelected) {
		a[attrSelected] = "true"
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
	e.rng = access.Range{}
	if r := n.Range; r != nil {
		e.rng = *r
		a[attrValueMin] = strconv.FormatFloat(r.Min, 'f', -1, 64)
		a[attrValueMax] = strconv.FormatFloat(r.Max, 'f', -1, 64)
		a[attrValueNow] = strconv.FormatFloat(r.Value, 'f', -1, 64)
	}
	if n.Role != access.RoleTextField {
		a[attrValueText] = n.Value
	}
	for i, v := range a {
		if v == e.attrs[i] {
			continue
		}
		e.attrs[i] = v
		if v == "" {
			e.el.Call("removeAttribute", attrNames[i])
		} else {
			e.el.Call("setAttribute", attrNames[i], v)
		}
	}
	if text != e.text {
		e.text = text
		e.el.Set("textContent", text)
	}
	e.press = false
	for _, act := range n.Actions {
		e.press = e.press || act == access.ActionPress
	}
	e.focusable, e.role = n.Focusable, n.Role
}

// showFocusLocked tells the page where gunim's focus is. The canvas keeps the
// browser's own focus, and names the focused node's element, unless the node takes
// text: then the text mirror has the browser's focus, and is the node. A screen reader
// reads the real text and caret from it, and a secret stays a password field.
func (d *Driver) showFocusLocked(w *Window, t *access.Tree) {
	field := t.Focus != nil && t.Focus.Role == access.RoleTextField
	switch {
	case t.Focus != nil && !field:
		d.setFocusNameLocked(w, accessID(w, t.Focus.ID))
	case d.accessFocusIn == w || field:
		d.setFocusNameLocked(w, "")
	}
	for _, m := range []js.Value{d.plain, d.secret} {
		if field && m.Equal(d.text) {
			m.Call("removeAttribute", "aria-hidden")
			m.Call("setAttribute", "aria-label", t.Focus.Name)
		} else if field || d.accessFieldIn == w {
			m.Call("setAttribute", "aria-hidden", "true")
			m.Call("removeAttribute", "aria-label")
		}
	}
	if field {
		d.accessFieldIn = w
	} else if d.accessFieldIn == w {
		d.accessFieldIn = nil
	}
}

func (d *Driver) setFocusNameLocked(w *Window, id string) {
	if id == d.accessFocus {
		return
	}
	d.accessFocus, d.accessFocusIn = id, w
	if id == "" {
		d.canvas.Call("removeAttribute", "aria-activedescendant")
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
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, w := range d.windows {
		if x := w.accessEls[id]; x != nil && x.el.Equal(el) {
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
	w.accessSoon = true
	if x.role == access.RoleTextField {
		// The click is what lets a phone show its keyboard for the field.
		d.lastTouch = false
	}
	d.mu.Unlock()
	switch {
	case x.press:
		w.in.Push(access.Request{ID: id, Action: access.ActionPress})
	case x.focusable:
		w.in.Push(access.Request{ID: id, Focus: true})
	}
}

// accessKey takes the keys a browser sends to a slider's element when a screen reader
// turns it up or down.
func (d *Driver) accessKey(e js.Value) {
	w, x, id := d.accessTarget(e)
	if w == nil || x.role != access.RoleSlider {
		return
	}
	step := x.rng.Step
	if step <= 0 {
		step = (x.rng.Max - x.rng.Min) / 20
	}
	v := x.rng.Value
	switch e.Get("key").String() {
	case "ArrowUp", "ArrowRight":
		v += step
	case "ArrowDown", "ArrowLeft":
		v -= step
	default:
		return
	}
	e.Call("preventDefault")
	d.mu.Lock()
	w.accessSoon = true
	d.mu.Unlock()
	w.in.Push(access.Request{ID: id, Value: max(x.rng.Min, min(v, x.rng.Max)), SetValue: true})
}

// forgetAccessLocked takes a closed window's tree off the page. It runs with mu held.
func (d *Driver) forgetAccessLocked(w *Window) {
	if w.accessOwed != nil {
		w.accessOwed.Stop()
		w.accessOwed = nil
	}
	if w.accessTop.Truthy() {
		w.accessTop.Call("remove")
	}
	w.accessEls = nil
	if d.accessFocusIn == w {
		d.setFocusNameLocked(w, "")
	}
}

var _ driver.AccessPublisher = (*Window)(nil)
