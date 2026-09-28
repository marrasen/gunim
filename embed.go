package gunim

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
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

// panicOnStrays is set by PanicOnStrays.
var panicOnStrays atomic.Bool

// PanicOnStrays makes Remove, Send and Focus with a node that is not in the tree panic, naming the call and the
// node's type, so the mistake shows where it happens. Without it they do nothing. A test suite turns it on in its
// TestMain; an application's own tests choose for themselves.
func PanicOnStrays(on bool) { panicOnStrays.Store(on) }

// stray reports a call about a node that is not in the tree, which panics with PanicOnStrays on.
func (u *UI) stray(call string, n Node) {
	if panicOnStrays.Load() {
		panic(fmt.Sprintf("gunim: %s of a %T that is not in the tree", call, n))
	}
}
