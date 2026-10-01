//go:build linux || windows || darwin

package render

import (
	"os"
	"runtime"
	"testing"

	"github.com/marrasen/gunim/internal/gl"
	"github.com/marrasen/gunim/internal/glfw"
)

// The tests draw with a hidden window's context, made once on the main
// thread, where GLFW has to make it. testES says the context is OpenGL
// ES, and testShared is what every test's renderer shares.
var (
	testWindow *glfw.Window
	testES     bool
	testShared Shared
)

func init() { runtime.LockOSThread() }

func TestMain(m *testing.M) {
	w, err := openTestWindow()
	if err != nil {
		// Without a display the tests that draw skip.
		os.Exit(m.Run())
	}
	testWindow = w
	code := m.Run()
	_ = w.Destroy()
	_ = glfw.Terminate()
	os.Exit(code)
}

// openTestWindow opens a hidden window with the context a desktop
// window gets: OpenGL 3.2 core, or OpenGL ES 3.0 where that is all the
// GL library offers.
func openTestWindow() (*glfw.Window, error) {
	if err := glfw.Init(); err != nil {
		return nil, err
	}
	probe, err := gl.NewDefaultContext()
	if err != nil {
		return nil, err
	}
	testES = probe.IsES()
	hints := [][2]int{{int(glfw.ContextVersionMajor), 3}, {int(glfw.Visible), glfw.False}}
	if testES {
		hints = append(hints,
			[2]int{int(glfw.ClientAPI), glfw.OpenGLESAPI},
			[2]int{int(glfw.ContextVersionMinor), 0},
			[2]int{int(glfw.ContextCreationAPI), glfw.EGLContextAPI})
	} else {
		hints = append(hints,
			[2]int{int(glfw.ClientAPI), glfw.OpenGLAPI},
			[2]int{int(glfw.ContextVersionMinor), 2},
			[2]int{int(glfw.OpenGLProfile), glfw.OpenGLCoreProfile},
			[2]int{int(glfw.OpenGLForwardCompat), glfw.True})
	}
	for _, h := range hints {
		if err := glfw.WindowHint(glfw.Hint(h[0]), h[1]); err != nil {
			return nil, err
		}
	}
	return glfw.CreateWindow(int(benchSize.W), int(benchSize.H), "gunim render test", nil, nil)
}
