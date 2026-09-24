# gunim

An animation-first GUI framework for Go. Pure Go, GPU rendered, driven by
the display's refresh rate.

## Status

The API is a sketch. It compiles, `go vet` is clean, and the tests pass.
`driver.Open` returns `ErrNoDriver`, so drawing comes next.

## The split

A gunim program is two halves that speak only in values.

The **window** owns the widget tree, the springs, the focus ring, and
how a dialog arrives and leaves. The **application** owns the work.
Between them run commands one way and intents the other, and every one
of them survives `encoding/json`.

```go
// Window half: the wiring is data.
b := widget.NewButton("Delete everything")
b.On = DeleteRequested{Target: "everything"}

// Application half: reached only through Client, which returns at once.
c.Mount(gunim.Root, "confirm", "confirm", ConfirmState{Title: "Delete everything?"})
```

That keeps application code off the goroutine that draws frames, by
construction: a function pointer has no wire format, so the only thing
that fits through is data.

Both halves are Go, so one type declaration serves them both. Swap the
in-process channel for a socket and the application moves to another
machine with both halves unchanged.

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

Two things make that true. Every path to a frame waits on the driver's
vsync tick, so the display sets the rate. And a `Publish` that a later
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
func (d *Dialog) Transition(p gunim.Presence) bool {
    switch p {
    case gunim.Entering:
        d.in.Animate(1, anim.Bouncy)
    case gunim.Exiting:
        d.in.Animate(0, anim.Gentle)
    }
    return !d.in.Active()
}
```

That is the whole exit-animation contract. The engine calls `Transition`
every frame while the node is entering or leaving. `Animate` ignores a
target it is already heading for, so the repeated call is free.

## Encoding

Everything crossing the boundary goes through `encoding/json/v2`, with
one set of options in `codec.go`:

```go
var wireOptions = json.JoinOptions(
	json.OmitZeroStructFields(true),
	json.Deterministic(true),
)
```

`OmitZeroStructFields` is why this repo has zero struct tags. A zero
field is left out and comes back zero, with no annotation on it. v2
matches member names case-sensitively, so the Go field name is the wire
name:

```json
{"Parent":"root","ID":"jobs","View":"joblist","Watch":["jobs"]}
```

## Packages

| Package | What it is |
| --- | --- |
| `gunim` | `Node`, the presence lifecycle, the window and its frame loop |
| `gunim/anim` | `Animated[T]`, springs, tweens, easings |
| `gunim/paint` | The per-frame draw list: rounded rects, shadows, text, layers |
| `gunim/geom` | float32 points, sizes, rectangles |
| `gunim/driver` | The seam with the operating system |
| `gunim/widget` | Worked examples: `Button`, `Dialog` |

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

A gunim driver is `internal/glfw` forked and exported, plus the GL calls
to replay a `paint` op list.

On Linux that port speaks X11, so a Wayland desktop runs gunim through
XWayland. That suits free-floating popups: X11 lets a client place a
window at an absolute screen position, and Wayland keeps a popup
anchored to its parent.

## Next

One milestone covers most of the risk: two windows, on two monitors at
different refresh rates, each drawing an animated rounded rectangle,
`CGO_ENABLED=0`, on Linux and Windows. Widgets, layout and text follow.
