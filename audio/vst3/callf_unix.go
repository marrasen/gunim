//go:build !windows

package vst3

import (
	"sync"

	"github.com/ebitengine/purego"
)

// funcs of a double second argument, made once for each function.
var floatFuncs sync.Map

// callFloat calls method m of obj, of an integer argument and a double:
// as setParamNormalized.
func callFloat(obj uintptr, m int, a uintptr, v float64) uintptr {
	fn := method(obj, m)
	f, ok := floatFuncs.Load(fn)
	if !ok {
		var g func(obj, a uintptr, v float64) uintptr
		purego.RegisterFunc(&g, fn)
		f, _ = floatFuncs.LoadOrStore(fn, g)
	}
	g, _ := f.(func(obj, a uintptr, v float64) uintptr)
	return g(obj, a, v)
}
