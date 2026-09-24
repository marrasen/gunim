# gunim

An animation-first GUI framework for Go. Pure Go, GPU rendered, driven by
the display's refresh rate.

## Status

The API is a sketch, and windows draw. `example/twowindows` opens two
windows, each animating a rounded rectangle on its own render thread,
built with `CGO_ENABLED=0`:

```sh
CGO_ENABLED=0 go run ./example/twowindows -for 5s
```

It runs on Windows 11 with 60 Hz and 120 Hz monitors side by side, each
window at its own monitor's rate, and on Linux under X11. macOS builds
and is still untested. Text shapes and wraps in pure Go, including
right-to-left and mixed scripts.

## The split

A gunim program is two halves that speak only in values.

The **window** owns the widget tree, the springs, the focus ring, and
how a dialog arrives and leaves. The **application** owns the work.
Between them run commands one way and intents the other, and every one
of them is a plain value.

```go
// Window half: the wiring is data.
b := widget.NewButton("Delete everything")
b.On = DeleteRequested{Target: "everything"}

// Application half: reached only through Client, which returns at once.
c.Mount(gunim.Root, "confirm", "confirm", ConfirmState{Title: "Delete everything?"})
```

That keeps application code off the goroutine that draws frames, by
construction: the application holds a `Client`, and a `Client` takes
values, so values are all it can hand the window.

In one process those values cross as they are, uncopied, so a state
holding an image gives the window the image itself.
The price is the rule channels already teach: sending a value hands it
over, so leave it unchanged afterwards.

Both halves are Go, so one type declaration serves them both, and
`CheckWire` proves in a test that each one would also survive a socket.
Put a socket transport where the queue is, and the application moves
to another machine with both halves unchanged.

Local interaction stays local. A dialog closes itself on the frame the
button is released, and tells the application afterwards.

## Topics and patches

Views watch topics. One publish reaches every view showing that data.

```go
c.Mount(gunim.Root, "jobs",    "joblist",  nil, "jobs")   // watches "jobs"
c.Mount(gunim.Root, "sidebar", "jobcount", nil, "jobs")   // watches it too

c.Publish("jobs", JobList{Jobs: jobs})                    // both update
c.Patch("jobs", JobProgress{ID: id, Progress: 0.4})       // one spring retargets
```

`Publish` and `Patch` draw the same line aprot draws between a refresh
trigger and `PatchSubscription`, and here that line is an animation
distinction: **a publish reconciles structure, a patch retargets a
value.** An aprot refresh lands as a `Publish`, and a pushed patch lands
as a `Patch`.

`Update(id, state)` is `Publish` to the topic named after a view's own
ID, so there is one delivery path.

## Push as hard as you like

`Client.Send` returns at once and never blocks on the window. The
application pushes at whatever rate it produces state, and the window
draws once per display refresh.

Two things make that true. A window keeps one frame in flight and
draws the next only once the display has taken it, so the display sets
the rate. And a `Publish` that a later
one supersedes is replaced in the queue before it ever reaches a view,
so 101 publishes between two frames cost one reconciliation:

```go
for i := range 100 { c.Publish("jobs", state[i]) }
c.Publish("jobs", latest)
// one frame, one view update, and latest is what it renders
```

`Patch` is left alone, because two patches on one topic may aim at
different rows. Coalescing also stops at `Mount`, `Unmount` and `Focus`,
so state queued ahead of one still reaches the views that were already
there.

`Window.Stats()` reports `Frames`, `Commands` and `Coalesced`, so the
behaviour is something you can check rather than trust.

## Keyed lists

Published state is worth animating only if the list can tell what
changed. `Sync` does that from keys.

```go
widget.Sync(l, u, s.Jobs,
    func(j Job) widget.Key { return widget.Key(j.ID) },
    newJobRow,
    (*jobRow).Set)
```

A row that arrived grows into place. A row that went collapses while its
neighbours close the gap. Everything else springs to where it belongs
now. A row that comes back mid-exit is revived with the velocity it had.

## The three ideas

**The engine owns the widget tree and runs each node through a
lifecycle.** A node is `Entering`, then `Present`, then `Exiting`. Remove
a dialog and it moves to `Exiting`, animates itself out, and tells the
engine when to unlink it. Put it back mid-dismiss and it reverses,
keeping its velocity. Over the wire that is `Mount` with the ID of a
view that is still leaving.

**Animation runs on wall-clock time.** Every frame carries a timestamp
and a delta, so motion looks the same at 60 Hz, at 144 Hz, and across a
dropped frame.

**A window draws while something moves, and sleeps the rest of the
time.** Each node reports whether its values are still in flight. Once
they all report settled, the window blocks until the next keystroke or
click.

## What a widget looks like

```go
func (d *Dialog) Transition(p gunim.Presence, f gunim.Frame) bool {
    switch p {
    case gunim.Entering:
        d.in.Animate(1, Bounce.Get(f.Theme))
    case gunim.Exiting:
        d.in.Animate(0, Settle.Get(f.Theme))
    }
    return !d.in.Active()
}
```

That is the whole exit-animation contract. The engine calls `Transition`
every frame while the node is entering or leaving. `Animate` ignores a
target it is already heading for, so the repeated call is free.

## Themes

A theme sets how widgets look and move: colours, but also paddings,
radii, text sizes and the springs that animations run on. A widget
declares each value as a token with a default, and reads it every frame:

```go
var ButtonPadding = theme.Length("button.padding", 16)

size := run.Advance + 2*ButtonPadding.Get(f.Theme)
```

A theme gives tokens values, and a window keeps one animated value per
token. Switching themes retargets them all, so going from dark to light
moves colours, sizes and motion together:

```go
w.RegisterTheme(widget.Light())
c.SetTheme("light")
```

Widgets animate state and look up style: a button animates how hovered
it is, from 0 to 1, and blends the theme's idle and hover colours by it
each frame. A theme switch in the middle of a hover then just works.
`example/dialog` switches between a dark and a light theme when you
press T.

## Encoding

Encoding belongs to a socket transport. It calls `MarshalCommand` and
`MarshalEnvelope`, which use `encoding/json/v2` with one set of options
in `codec.go`:

```go
var wireOptions = json.JoinOptions(
	json.OmitZeroStructFields(true),
	json.Deterministic(true),
)
```

`OmitZeroStructFields` is why this repo has zero struct tags. A zero
field is left out and comes back zero, with no annotation on it. v2
matches member names case-sensitively, so the Go field name is the wire
name. Each carried value goes with the name `RegisterType` gave it, so
the far end knows what to decode it into:

```json
{"Command":"mount","Parent":"root","ID":"jobs","View":"joblist","Watch":["jobs"],"Value":{"Kind":"job.list","Data":{"Jobs":[{"ID":"1","Title":"Reindex archive","Status":"pending"}]}}}
```

## Packages

| Package | What it is |
| --- | --- |
| `gunim` | `Node`, the presence lifecycle, the window and its frame loop |
| `gunim/anim` | `Animated[T]`, springs, tweens, easings |
| `gunim/paint` | The per-frame draw list: rounded rects, shadows, text, layers |
| `gunim/geom` | float32 points, sizes, rectangles |
| `gunim/input` | Pointer, keyboard and focus events, keys, buttons, modifiers |
| `gunim/theme` | Tokens, themes, and animated theme switching |
| `gunim/text` | Fonts and fallback, shaping, paragraph layout, glyph rasterizing |
| `gunim/driver` | The seam with the operating system, and an offscreen window |
| `gunim/driver/desktop` | The driver for Linux, Windows and macOS, on GLFW and OpenGL |
| `gunim/widget` | `Row`, `Column`, `Scroll`, `Label`, `TextField`, `Card`, `Button`, `Dialog`, a keyed `List`, and their theme tokens |

Commands, intents, topics and the `Client` live in `wire.go` and
`view.go`. `driver.Offscreen` plus `Window.Frame` run a window with no
display, which is how the tests step an interface a frame at a time.

A node implements `Node`, which is `Layout` and `Paint`. Three further
interfaces are opt-in: `Handler` for input, `Animator` for animated
values, `Transitioner` for enter and exit.

## The platform layer

gunim takes it from Ebitengine 2.10, which ships a complete
reimplementation of GLFW in pure Go: X11 with GLX and EGL on Linux and
the BSDs, Win32 with WGL on Windows, Cocoa with NSGL on macOS, all
through purego. Its window constructor is the real GLFW one,
`CreateWindow(w, h, title, monitor, share)`, so several windows, a chosen
monitor and shared GL objects are all available down there. Ebitengine
confines itself to a single window in `internal/ui`, one layer up, and
that is the layer gunim replaces.

`internal/glfw` and `internal/gl` are copies of Ebitengine's, with two
changes listed in `internal/README.md`. The main one tracks the current
GL context per thread, so each window can render on a thread of its
own.

`driver/desktop` pumps GLFW events on the main thread, and gives each
window a render thread that owns its GL context. The render thread
replays the frame's `paint` ops, swaps buffers, and reports the frame
shown once the swap returns. That report is what paces the window, so
two windows on two monitors keep two refresh rates. On Windows, GLFW's
swap waits for the compositor, which follows the fastest monitor, so
each render thread waits for its own monitor's vertical blank instead,
the way Chromium does. Where the swap does
not wait for the display, as under a remote desktop, the render thread
sleeps out the rest of the refresh itself.

Every shape is one quad and one signed distance field, so rounded
rectangles, strokes, gradients and shadows stay crisp at any scale. A
layer draws into an offscreen texture and is composited back with its
opacity and rounded clip. A layer's `Blur` and `Backdrop` are Gaussian
blurs, run at half or a quarter of the resolution when they are wide.
Text is shaped and wrapped by go-text/typesetting, a
pure-Go port of HarfBuzz, and drawn from a glyph atlas that keeps four
quarter-pixel shifts of each glyph, so text sits sharp at any
fractional position. While a transform scales it, as when a dialog
grows into place, the glyphs keep their resting size and scale with
the quads, and they sharpen again when the motion settles.

On Linux that port speaks X11, so a Wayland desktop runs gunim through
XWayland. That suits free-floating popups: X11 lets a client place a
window at an absolute screen position, and Wayland keeps a popup
anchored to its parent.

## Next

The first milestone is two windows on two monitors at different refresh
rates, each drawing an animated rounded rectangle, with
`CGO_ENABLED=0`, on Linux and Windows. Windows passes, on a 60 Hz and a
120 Hz monitor (issue #1). What remains:

- Run it on Linux with two monitors at different rates, on a real GPU.

Then widgets and layout, on top of the theme:

- Text fields: input methods (the driver reports no composition yet),
  multi-line editing, and caret movement that follows visual order in
  mixed-direction text.
- Theme switches that look good halfway: blend colours in Oklab rather
  than sRGB, and let a theme stagger its tokens, so text keeps its
  contrast while light and dark cross.
- Themes for a subtree, so a sidebar or a dialog can wear its own.
