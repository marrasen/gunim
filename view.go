package gunim

import (
	"encoding/json/jsontext"
	"fmt"
)

// A view builds, updates and patches a subtree from application state.
// The generic front doors are [RegisterView] and [RegisterPatch]; this
// is what survives after the types are erased.
type view struct {
	name    string
	build   func(jsontext.Value) (Node, error)
	update  func(Node, jsontext.Value, *UI) error
	patches map[string]func(Node, jsontext.Value, *UI) error
}

// RegisterView names a view the application can mount, and wires it to
// the state type it renders.
//
// build runs once, when the application mounts the view. update runs
// straight after it with the same state, and again for every [Update]
// or [Publish] that reaches the view. update is where a change of state
// becomes animation: retarget a spring, sync a list, mark a node for
// removal. Both run on the window's UI goroutine.
//
//	gunim.RegisterView(w, "jobs",
//	    func(s JobList) *widget.List { return widget.NewList() },
//	    func(l *widget.List, s JobList, u *gunim.UI) {
//	        widget.Sync(l, u, s.Jobs, jobKey, newJobRow, (*jobRow).Set)
//	    })
//
// Views live in the window's process, so the application sends state
// and stays clear of the render loop.
func RegisterView[S any, N Node](w *Window, name string, build func(S) N, update func(N, S, *UI)) {
	v := w.view(name)
	v.build = func(raw jsontext.Value) (Node, error) {
		s, err := decodeState[S](name, raw)
		if err != nil {
			return nil, err
		}
		return build(s), nil
	}
	v.update = func(n Node, raw jsontext.Value, u *UI) error {
		typed, err := nodeAs[N](name, n)
		if err != nil {
			return err
		}
		s, err := decodeState[S](name, raw)
		if err != nil {
			return err
		}
		if update != nil {
			update(typed, s, u)
		}
		return nil
	}
}

// RegisterPatch wires a patch type to a view.
//
// A patch says that a value changed while the shape stayed put, so
// apply usually retargets a spring and lets it carry the node from
// wherever it is now. Register the patch type with [RegisterType] as
// well, so both ends of the connection agree on its name.
//
//	gunim.RegisterPatch(w, "jobs", func(l *widget.List, p Progress, u *gunim.UI) {
//	    l.Row(p.ID).Progress.Animate(p.Done, anim.Snappy)
//	})
func RegisterPatch[P any, N Node](w *Window, viewName string, apply func(N, P, *UI)) {
	var zero P
	kind, ok := TypeName(zero)
	if !ok {
		panic(fmt.Sprintf("gunim: patch %T needs RegisterType before RegisterPatch", zero))
	}
	v := w.view(viewName)
	v.patches[kind] = func(n Node, raw jsontext.Value, u *UI) error {
		typed, err := nodeAs[N](viewName, n)
		if err != nil {
			return err
		}
		var p P
		if err := decode(raw, &p); err != nil {
			return fmt.Errorf("gunim: view %q patch %s: %w", viewName, kind, err)
		}
		apply(typed, p, u)
		return nil
	}
}

func decodeState[S any](name string, raw jsontext.Value) (S, error) {
	var s S
	if err := decode(raw, &s); err != nil {
		return s, fmt.Errorf("gunim: view %q state: %w", name, err)
	}
	return s, nil
}

func nodeAs[N Node](name string, n Node) (N, error) {
	typed, ok := n.(N)
	if !ok {
		var zero N
		return zero, fmt.Errorf("gunim: view %q wants a %T and found a %T", name, zero, n)
	}
	return typed, nil
}

// view returns the named view's registration, creating it on first use
// so that [RegisterView] and [RegisterPatch] may arrive in either
// order.
func (w *Window) view(name string) *view {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.views == nil {
		w.views = make(map[string]*view)
	}
	v, ok := w.views[name]
	if !ok {
		v = &view{name: name, patches: map[string]func(Node, jsontext.Value, *UI) error{}}
		w.views[name] = v
	}
	return v
}

// lookup finds a registered view that is ready to build.
func (w *Window) lookup(name string) (*view, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	v, ok := w.views[name]
	return v, ok && v.build != nil
}

// CommandFailed travels back to the application when a command fails to
// take effect, so a lost mount shows up as a message rather than as a
// window that quietly stays empty.
type CommandFailed struct {
	Command string
	ID      ID
	Key     string
	Reason  string
}

func init() { RegisterType[CommandFailed]("gunim.command-failed") }

// apply carries out one command on the UI goroutine.
func (u *UI) apply(c Command) {
	var err error
	switch c := c.(type) {
	case Mount:
		err = u.mount(c)
	case Update:
		err = u.publish(string(c.ID), c.State)
	case Publish:
		err = u.publish(c.Key, c.State)
	case Patch:
		err = u.patch(c)
	case Unmount:
		err = u.unmount(c)
	case Focus:
		err = u.focusID(c.ID)
	default:
		err = fmt.Errorf("unknown command %T", c)
	}
	if err != nil {
		id, key := commandTarget(c)
		u.report(CommandFailed{Command: c.Name(), ID: id, Key: key, Reason: err.Error()})
	}
}

func commandTarget(c Command) (ID, string) {
	switch c := c.(type) {
	case Mount:
		return c.ID, ""
	case Update:
		return c.ID, ""
	case Publish:
		return "", c.Key
	case Patch:
		return "", c.Key
	case Unmount:
		return c.ID, ""
	case Focus:
		return c.ID, ""
	default:
		return "", ""
	}
}

func (u *UI) mount(c Mount) error {
	parent, ok := u.ids[c.Parent]
	if !ok {
		return fmt.Errorf("parent %q is missing", c.Parent)
	}
	v, ok := u.w.lookup(c.View)
	if !ok {
		return fmt.Errorf("view %q needs RegisterView", c.View)
	}
	if s, taken := u.ids[c.ID]; taken {
		if !s.leaving() {
			return fmt.Errorf("id %q is already mounted", c.ID)
		}
		if s.view == v {
			return u.revive(s, parent, c)
		}
		// A different view wants the ID. The old one finishes its exit
		// without it.
		u.release(s)
	}
	n, err := v.build(c.State)
	if err != nil {
		return err
	}

	u.Insert(parent.node, n)
	s := u.index[n]
	s.id = c.ID
	s.view = v
	u.ids[c.ID] = s
	u.watch(s, c)

	// Build makes the shell; update fills it. Running both here means a
	// view has one place that turns state into nodes.
	return v.update(n, c.State, u)
}

// revive brings back a view that is still animating out, so a dialog
// dismissed and reopened swings back from wherever its exit had got to.
// It takes the new parent, watch list and state, as a fresh mount would.
func (u *UI) revive(s, parent *state, c Mount) error {
	if parent.within(s) {
		return fmt.Errorf("parent %q is inside %q", c.Parent, c.ID)
	}
	u.Insert(parent.node, s.node)
	u.unsubscribe(s)
	u.watch(s, c)
	return s.view.update(s.node, c.State, u)
}

// release takes a leaving view's ID and topics away, so a new view can
// have the ID while the old one finishes its exit.
func (u *UI) release(s *state) {
	delete(u.ids, s.id)
	u.unsubscribe(s)
	s.id = ""
}

// watch subscribes s to the topics c names. Every view watches a topic
// named after its own ID, so Update and Publish share one delivery
// path.
func (u *UI) watch(s *state, c Mount) {
	u.subscribe(s, string(c.ID))
	for _, key := range c.Watch {
		u.subscribe(s, key)
	}
}

func (u *UI) subscribe(s *state, key string) {
	u.topics[key] = append(u.topics[key], s)
	s.topics = append(s.topics, key)
}

func (u *UI) unsubscribe(s *state) {
	for _, key := range s.topics {
		subs := u.topics[key]
		for i, sub := range subs {
			if sub == s {
				u.topics[key] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		if len(u.topics[key]) == 0 {
			delete(u.topics, key)
		}
	}
	s.topics = nil
}

// publish hands state to every view watching key.
func (u *UI) publish(key string, state jsontext.Value) error {
	subs := u.topics[key]
	if len(subs) == 0 {
		return fmt.Errorf("nothing is watching %q", key)
	}
	var firstErr error
	for _, s := range subs {
		if err := s.view.update(s.node, state, u); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	u.invalid = true
	return firstErr
}

// patch hands a typed partial change to every view watching the key
// that has a handler for it.
func (u *UI) patch(c Patch) error {
	subs := u.topics[c.Key]
	if len(subs) == 0 {
		return fmt.Errorf("nothing is watching %q", c.Key)
	}
	var firstErr error
	applied := 0
	for _, s := range subs {
		apply, ok := s.view.patches[c.Kind]
		if !ok {
			continue
		}
		applied++
		if err := apply(s.node, c.Data, u); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if applied == 0 && firstErr == nil {
		return fmt.Errorf("no view watching %q handles patch %q", c.Key, c.Kind)
	}
	u.invalid = true
	return firstErr
}

func (u *UI) unmount(c Unmount) error {
	s, ok := u.ids[c.ID]
	if !ok {
		return fmt.Errorf("id %q is missing", c.ID)
	}
	u.Remove(s.node)
	return nil
}

func (u *UI) focusID(id ID) error {
	if id == "" {
		u.Focus(nil)
		return nil
	}
	s, ok := u.ids[id]
	if !ok {
		return fmt.Errorf("id %q is missing", id)
	}
	if s.leaving() {
		return fmt.Errorf("id %q is leaving", id)
	}
	u.Focus(s.node)
	return nil
}
