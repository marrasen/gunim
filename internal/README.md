# Code from Ebitengine

These packages are copied from [Ebitengine](https://github.com/hajimehoshi/ebiten)
v2.10.2, under the Apache License 2.0 in `LICENSE-ebitengine`:

| Here | Upstream |
| --- | --- |
| `glfw` | `internal/glfw`, a pure-Go port of GLFW |
| `gl` | `internal/graphicsdriver/opengl/gl`, OpenGL bindings through purego |
| `cocoa`, `exeicon`, `microsoftgdk`, `winver`, `windowsystem` | the `internal` packages of the same names, which `glfw` and `gl` need |

Go keeps another module from importing `internal` packages, so gunim
carries a copy. Tests, the js and console variants, `gen.go`, and the
cgo-only Xlib check are left out. Import paths are rewritten to gunim's.

gunim's changes are marked `gunim change` in the code:

- `glfw` tracks the current GL context per OS thread, as C GLFW does
  (`current_context.go`, `thread_*.go`). Ebitengine kept one global,
  which suited its one render thread. gunim renders each window on a
  thread of its own. macOS and the BSDs still share one slot.
- `gl` loads desktop OpenGL before OpenGL ES on Linux and the BSDs
  (`procaddr_linbsd.go`), so the context is created through GLX without
  asking `glxinfo` whether GLX can make an ES context.
- `glfw` gives the Win32 port the input-method API of the X11 one
  (`win32_ime_windows.go`): compositions through IMM32 while the
  application takes text, typed text through the text input callback,
  `ResetInputContext`, and, Windows only, `SetInputMethodEnabled` and
  `SetInputMethodCaret`. `windowProc` routes the `WM_IME_*` composition
  messages there, and the platform window state holds its fields.
- `glfw` implements the Win32 clipboard (`win32_clipboard_windows.go`),
  as C GLFW does. The Ebitengine port left it as a stub that panicked.

To take a newer Ebitengine, copy the same files again and reapply those
changes.
