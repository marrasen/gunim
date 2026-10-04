package vst3

import (
	"math"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// newCallback returns fn as a function a plugin can call.
func newCallback(fn any) uintptr { return syscall.NewCallback(fn) }

// A callback of Windows' takes its arguments in the integer registers
// only, while x64 passes a double third in XMM2. floatThunk returns a
// function that copies XMM2 into R8, the third integer register, and
// jumps to fn: the callback reads the double's bits there.
func floatThunk(fn uintptr) uintptr {
	code := []byte{
		0x66, 0x49, 0x0F, 0x7E, 0xD0, // movq r8, xmm2
		0x48, 0xB8, 0, 0, 0, 0, 0, 0, 0, 0, // mov rax, fn
		0xFF, 0xE0, // jmp rax
	}
	*(*uint64)(unsafe.Pointer(&code[7])) = uint64(fn)
	mem, err := windows.VirtualAlloc(0, uintptr(len(code)), windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		panic("vst3: " + err.Error())
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(mem)), len(code)), code) //nolint:govet // memory of the system's
	var old uint32
	if err := windows.VirtualProtect(mem, uintptr(len(code)), windows.PAGE_EXECUTE_READ, &old); err != nil {
		panic("vst3: " + err.Error())
	}
	return mem
}

// newFloatCallback returns fn, which takes a double third, as a function
// a plugin can call.
func newFloatCallback(fn func(a, b uintptr, v float64) uintptr) uintptr {
	return floatThunk(syscall.NewCallback(func(a, b, v uintptr) uintptr {
		return fn(a, b, math.Float64frombits(uint64(v)))
	}))
}

// newFloatCallback4 is newFloatCallback for a function of four
// arguments.
func newFloatCallback4(fn func(a, b uintptr, v float64, d uintptr) uintptr) uintptr {
	return floatThunk(syscall.NewCallback(func(a, b, v, d uintptr) uintptr {
		return fn(a, b, math.Float64frombits(uint64(v)), d)
	}))
}
