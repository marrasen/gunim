# Issue #30: every window stops answering under kakel's stress test

## Machine

- Windows 11 Pro 10.0.26300
- Intel Core Ultra 7 268V, 8 cores, 8 logical processors
- Intel Arc 140V GPU (16 GB), driver 32.0.101.8860
- Monitors: the laptop's LQ134Z1 at 120 Hz (2880×1800, about 140%),
  and two Dell S2722QC at 60 Hz (3840×2160, about 125%)
- Go 1.27.1 windows/amd64
- gunim e41e6ff (master), kakel da0e1df (main), built with a `go.work`
- WinDbg 1.2610 (`cdbX64`), public symbols from the Microsoft symbol server

## Summary

The stress brings back two freezes, not one. Both stop every window at once.

1. **A lock-order deadlock inside Intel's GL driver.** `glGenerateMipmap`
   takes two of the driver's critical sections in one order, and every
   other call seen (making a context, `glTexImage2D`, `SetPixelFormat`)
   in the other. Two render threads deadlock, and the main thread stops
   in a window hook the driver installs, which takes one of the two.
   **Fix:** gunim no longer calls `glGenerateMipmap`.
2. **A priority inversion between gunim's raised threads and the Go
   runtime.** The main, render and UI threads run at
   `THREAD_PRIORITY_HIGHEST` in an above-normal process. Under full load
   they fill every core. Go's own threads, which hold the
   scheduler's Ps, starve, and the whole runtime stands still for 4 to
   11 s at a time, until Windows' anti-starvation boost lets them run.
   **Fix:** gunim no longer raises the process or any thread.

The issue's hypothesis, a render thread needing the main thread through a
message, was not borne out: no render thread in the deadlock waits on the
main thread, and the main thread holds nothing the render threads need.

## 1. The driver's lock order

### Reproducing

The full stress (`KAKEL_ALONE=1`, `KAKEL_STRESS=30m`) wrote a hang file
after 90 s, 24 s and 28 s on master. Native stacks came from `cdbX64 -pv`
(non-invasive) and a full dump, `!locks` naming each locked critical
section and its owner, and each waiting thread's `rcx` at
`RtlEnterCriticalSection` naming the one it waits for.

### The cycle

Freeze 2, [stacks](1-freeze2-native.txt):

| Thread | Doing | Holds | Waits for |
|---|---|---|---|
| render `9e98` | `wglCreateContextAttribsARB`, a new window's context (`startGL`) | A `…8b70` | B `…8bd0` |
| render `4204` | `glGenerateMipmap` in `Renderer.blur` | B `…8bd0` | A `…8b70` |
| main `9ca0` | `CreateWindowExW` → `user32!DispatchHookA` → `igxe2lpgicd64` | | A |
| 9 more render threads | `BindFramebuffer`, `DeleteFramebuffer`, `wglDXOpenDeviceNV` | | A |

Freeze 3, [stacks](1-freeze3-native.txt): the same cycle. `wglCreateContextAttribsARB`
holds A and waits for B, and `glGenerateMipmap` in `blur` holds B and waits for A.
The main thread waits for A in the driver's hook, this time through
`fnHkINLPCWPRETSTRUCTA` (a `WH_CALLWNDPROCRET` hook), and a third render
thread, in `SetPixelFormat` within the same context creation, waits for A too.

In words: making a context takes the driver's lock A and then B, while
`glGenerateMipmap` takes B and then A. The driver hooks the main thread's
window procedures (`WH_CALLWNDPROC` and `WH_CALLWNDPROCRET`), and its hook takes A,
so the main thread stops with the render threads, inside `CreateWindowExW`
here, or `PeekMessageW` in the issue's report, whichever message comes first.
The issue's report fits: it too has `wglCreateContextAttribsARB` and
`GenerateMipmap` on render threads, and the main thread in `PeekMessageW`.

### Approaches

**A Go lock round a context's making and deleting, against frames being
drawn** (a `sync.RWMutex`, frames shared, lifecycle alone). The stress
froze again after 15 s. [Stacks](2-texture-vs-mipmap-native.txt): two
render threads both drawing, `glTexImage2D` in `Renderer.fitSize` holding A
and waiting for B, `glGenerateMipmap` in `blur` holding B and waiting for A.
Any two draws can deadlock, so no lock round the lifecycle helps. Reverted.

**No `glGenerateMipmap` (the fix).** It holds B in all three cycles, and is
the one call seen taking the locks in reverse. gunim called it in two places:

- `blur`, to average a layer down before a wide blur runs at a fraction of
  the window's resolution. The blur shader now has a box mode: a pass
  averages each k×k block with (k/2)² bilinear samples, each falling where
  four pixels meet. It draws into the vertical pass's target, which that
  pass overwrites anyway, so no target is added, and it covers only the
  region the blur needs.
- `texture`, an image's mipmaps on first upload. Each level is now drawn
  from the one above with the same box pass, the texture's base and top
  levels narrowed to the level above, so a pass never reads the level it
  draws into.

Both run on the GPU, at about the cost of `glGenerateMipmap`. With this,
the stress still froze, but in the dump taken then no critical section
was locked: that was the second freeze. The proof runs below still have
to show the first is gone.

## 2. The priority inversion

### What it looked like

After the mipmap fix, the stress still froze within a minute, with a hang file, and
Windows marked the windows "Not Responding". But there were no locked critical
sections, and the main thread sat idle in `WaitMessage` whenever cdb looked,
with tasks queued and the wake-up (`PostMessageW`) succeeding.

Investigation code (removed since) logged, once a second from an ordinary
goroutine, the event loop's iterations and the tasks run. The
[log](3-runtime-stall.log):

- The once-a-second goroutine itself went silent for 5, 6 and 11 s at a time:
  no Go code ran anywhere in the process, not just on the main thread.
- The garbage collector, which ran every 0.2 s, did not run in those gaps.
  Its stop-the-world phases took under 1 ms, with one of 97 ms. Not the cause.
- A separate PowerShell process, sampling kakel every 2 s, went silent
  for the same 11 s (14:29:06 to 14:29:17), and its 2 s sleeps often took
  6 s. kakel was using 7.5 to 7.9 of the 8 cores.

### The cause

- **The priority levels.** Since #1, gunim put the process in the above-normal
  class (base 10) and raised to `THREAD_PRIORITY_HIGHEST` (12) the main thread,
  every window's render thread and every window's UI goroutine's thread. That
  is dozens of threads with kakel's windows open, while the Go runtime's other
  threads stay at 10 and other programs at 8.
- **What happens under load.** The raised threads alone fill the 8 cores, and
  Windows hardly runs anything below 12. That includes the threads holding the
  Go scheduler's Ps, the GC workers and the goroutines on unraised threads. A
  raised thread that needs a P held by a starved one waits too. The Go
  scheduler hands work between threads without regard to their OS priority, so
  raising some of a Go program's threads cannot put them first; it can only
  stall them behind the ones it starves.
- **Why it ends.** Only Windows' balance-set manager breaks it: it boosts
  threads ready for about 4 s, hence stalls of 4 to 11 s.

### The test

The same build with thread raising turned off (the process class left
above normal): the full stress ran **15 minutes with no hang file**
([log](4-no-raise.log)). Every run with threads raised, nine in all,
wrote a hang file, after 14 s to 3 minutes, most within 90 s.

### The fix

gunim no longer changes priorities: no process class, no thread raise,
and no `driver.Raiser`. The UI goroutine no longer keeps its thread for
life, which it did only so its thread could be raised, and the render
thread lets its thread go again as it exits, as before #1's change.

This undoes #1's two priority changes (`362177e`, `c38c5e5`). They kept
frames at rate while Defender and WMI scanning, or busy loops at normal
priority, loaded every CPU; that may come back as dropped frames under
such load, but cannot freeze anything. Raising the whole process alone
cannot invert either, but it lets a busy gunim program starve every other
program on the machine, as the stress did here.

## Proof runs

**Not done yet.** The two 30-minute full stress runs, and the manual checks
(kakel windows, Settings, a dropdown's popup, dragging a pane out, resizing,
no flicker or black as windows open), still have to be run with both fixes.
The machine was needed back. They need an Intel GPU to prove the first fix;
any machine proves the second.

## Tests and lint

- `go test ./...` in kakel passes.
- `go test ./...` in gunim: three tests fail, all also on unchanged master:
  - `TestCellsDrawPixelForPixel`: pixels 798 and 799 of row 0 at scale 1
    are `[24 36 48 191]` against `[32 48 64 255]`;
  - `TestEachWindowTakesItsMonitorsRate`: the 120 Hz window draws 83 to 86
    frames a second, on master and with the fixes alike;
  - `TestSignWithANewKey` in `tools/gunimsign`.
- `golangci-lint run ./...`, as is and with `GOOS=linux`: 156 findings on
  master, not 0, and the same findings with the fixes.
- New tests: a wide blur of fine detail is even, a wide blur spreads an
  edge both ways, an image drawn at an eighth of its size averages its
  pixels, and an image uploaded in the middle of a clipped layer leaves the
  rest of the layer drawing, clipped. The last fails if the mipmap passes
  leave the framebuffer bound.

## Other findings

- **The hang file can mislead.** It is written once the main thread has not
  answered for 10 s, and its main goroutine is wherever the main thread was
  when the world came back, here often `WaitMessage`, idle, though the
  whole runtime had stood still.
- **cdb refused to attach once** with `Win32 error 0n5`, during the first
  freeze. It attached every time after.
- **The Linux crash in `glXCreateWindow`** the issue mentions was not
  looked at. Making a context while others draw in the same share group is
  where Intel's driver deadlocks too.
