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

// fieldsOf returns the embedded fields of struct type t that are structs or pointers to structs.
func fieldsOf(t reflect.Type) []int {
	if v, ok := embedFields.Load(t); ok {
		if idx, ok := v.([]int); ok {
			return idx
		}
	}
	var idx []int
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.Anonymous {
			continue
		}
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct {
			idx = append(idx, i)
		}
	}
	embedFields.Store(t, idx)
	return idx
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
