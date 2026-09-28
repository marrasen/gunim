package gunim

import (
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"unsafe"
)

// embedFields holds, for each struct type, the indexes of its embedded fields that are or may hold a node.
var embedFields sync.Map // reflect.Type -> []int

// embedded returns the nodes n embeds, directly or through the structs it embeds, such as the *widget.Dialog of a
// view that embeds one. Methods promoted from them see those nodes, not n.
func embedded(n Node) []Node {
	v := reflect.ValueOf(n)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return nil
	}
	var out []Node
	collectEmbedded(v.Elem(), &out, 0)
	return out
}

// collectEmbedded adds the nodes struct value s embeds to out.
func collectEmbedded(s reflect.Value, out *[]Node, depth int) {
	if depth > 8 {
		return
	}
	for _, i := range fieldsOf(s.Type()) {
		f := s.Field(i)
		var p reflect.Value // a pointer to the embedded struct
		if f.Kind() == reflect.Pointer {
			if f.IsNil() {
				continue
			}
			p = reflect.NewAt(f.Type().Elem(), unsafe.Pointer(f.Pointer()))
		} else {
			p = reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr()))
		}
		if p.Type().Elem().Size() == 0 {
			continue
		}
		if n, ok := p.Interface().(Node); ok {
			*out = append(*out, n)
		}
		collectEmbedded(p.Elem(), out, depth+1)
	}
}

// nodeType is the Node interface, for asking a type whether it is one.
var nodeType = reflect.TypeFor[Node]()

// fieldsOf returns the embedded fields of struct type t, structs or pointers to structs, that are nodes or embed
// one at some depth. It is worked out from the types alone, once for each, so a node that embeds none, as most do
// through an anim.Group, is passed over at once.
func fieldsOf(t reflect.Type) []int {
	if v, ok := embedFields.Load(t); ok {
		if idx, ok := v.([]int); ok {
			return idx
		}
	}
	idx, _ := fieldsAt(t, map[reflect.Type]bool{})
	return idx
}

// fieldsAt is fieldsOf, with visiting the types being worked out above
// t. It reports whether t leads back to one of them, as a type embedding
// itself through a pointer does. Such a field may hold a node, and is
// kept, which is right at any depth, so every answer is kept for its
// type.
func fieldsAt(t reflect.Type, visiting map[reflect.Type]bool) (idx []int, loops bool) {
	if v, ok := embedFields.Load(t); ok {
		if kept, ok := v.([]int); ok {
			return kept, false
		}
	}
	if visiting[t] {
		return nil, true
	}
	visiting[t] = true
	defer delete(visiting, t)
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.Anonymous {
			continue
		}
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() != reflect.Struct {
			continue
		}
		if reflect.PointerTo(ft).Implements(nodeType) {
			idx = append(idx, i)
			continue
		}
		inner, back := fieldsAt(ft, visiting)
		loops = loops || back
		if len(inner) > 0 || back {
			idx = append(idx, i)
		}
	}
	embedFields.Store(t, idx)
	return idx, loops
}

// A Stray is a call about a node that was not in the tree: Send or
// Focus with a node that had left it, or had yet to enter. The call
// does nothing for the node, which is right for a node that left while
// something about it was on its way, and a bug for code that took the
// node to be there. An offscreen window keeps its strays for a test to
// look at; see [Window.Strays].
type Stray struct {
	// Call is the call: "Send" or "Focus".
	Call string
	// Node is the node's type, such as "*widget.Toast".
	Node string
	// At is where the call came from, as file:line, the first caller
	// outside the engine.
	At string
}

func (s Stray) String() string {
	return fmt.Sprintf("%s of a %s that is not in the tree, from %s", s.Call, s.Node, s.At)
}

// stray keeps a call about a node that is not in the tree, in a window
// that keeps them.
func (u *UI) stray(call string, n Node) {
	if !u.w.keepStrays {
		return
	}
	u.w.strays = append(u.w.strays, Stray{Call: call, Node: fmt.Sprintf("%T", n), At: callerOutside()})
}

// callerOutside is the file and line of the first caller outside the
// engine's own package.
func callerOutside() string {
	pcs := make([]uintptr, 16)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(3, pcs)])
	for {
		f, more := frames.Next()
		if !strings.HasPrefix(f.Function, "github.com/marrasen/gunim.") || strings.HasSuffix(f.File, "_test.go") {
			return fmt.Sprintf("%s:%d", filepath.Base(f.File), f.Line)
		}
		if !more {
			return "?"
		}
	}
}
