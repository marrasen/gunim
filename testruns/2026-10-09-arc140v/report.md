# Issue #30: proof runs for the two fixes

This run follows [2026-10-07-arc140v](../2026-10-07-arc140v/report.md) on the
same machine. That run found the two freezes, their causes and their fixes,
but could not run the proofs. This one rebases the fixes onto current master
and runs them.

## Machine

- Windows 11 Pro 10.0.26300
- Intel Core Ultra 7 268V, 8 cores
- Intel Arc 140V GPU (16 GB), driver 32.0.101.8860
- Monitors as in the earlier run
- Go 1.27.1 windows/amd64
- gunim e215d33 (master) with the two fix commits after this one on top,
  kakel 76dfd52 (main), built with a `go.work`

## The fixes

1. **render: make mipmaps with a pass of our own, not glGenerateMipmap.**
   Intel's driver takes two of its locks in one order in `glGenerateMipmap`,
   and in the other order in making a context, `glTexImage2D` and
   `SetPixelFormat`. Two render threads deadlock, and the main thread stops
   in the driver's window hook.
2. **desktop: leave thread and process priorities as Windows sets them.**
   Raised render, UI and main threads starved the Go runtime's own threads
   under full load. The whole process then stood still for 4 to 11 s.

Both cherry-picked cleanly onto master. The only change was a test helper
in `blur_test.go`, renamed `paintStripes` because master has since added a
`stripes` type in the same package.

## 1. Master still freezes: FAIL, as expected

The full stress (`KAKEL_ALONE=1`, `KAKEL_STRESS=5m`, no `KAKEL_STRESS_ONLY`)
on unchanged master wrote a hang file after **16 s**
([hang file](1-master-hang.txt)). It shows the first freeze's pair again:

- goroutine 191: `wglCreateContextAttribsARB` ← `createContextWGL` ←
  `CreateDeferredContext` ← `startGL`, a new window's context;
- another render thread: `GenerateMipmap` ← `Renderer.blur` (`blur.go:81`)
  ← `closeLayer`.

## 2. Full stress with both fixes, twice for 30 minutes: PASS

The full stress, `KAKEL_STRESS=30m`, no `KAKEL_STRESS_ONLY`. A script checked
every 5 s for a hang file for kakel's pid, and for the process reporting
"Not Responding" (`Process.Responding`).

| Run | Started | Length | Hang file | Not Responding | Done by the stress |
|---|---|---|---|---|---|
| [1](2-fix-run1.log) | 17:11 | 30 min | none | never | 7732 window and pane actions, 1604 What's New and About windows |
| [2](2-fix-run2.log) | 17:41 | 30 min | none | never | 7884 window and pane actions, 1669 What's New and About windows |

Both runs ended when the stress ended, with every worker saying "no freeze".

## 3. Manual checks

Not done by the agent: opening and closing kakel windows, the Settings
window, a dropdown's popup, dragging a pane out into a window of its own,
resizing, and no flicker or black as windows open. Each needs a person at
the screen.

## Tests and lint

- `go test ./...` in kakel: passes.
- `go test ./...` in gunim, with the fixes: fails in the same places as
  master does:
  - `TestCellsDrawPixelForPixel` in `driver/internal/render`;
  - `TestSignWithANewKey` in `tools/gunimsign`;
  - `filemanager`: a different test fails in each run, on master and with
    the fixes alike (`TestDeleteKeyTrashesAndUndoRestores`,
    `TestACopyReplacedByARenameIsNoticed`,
    `TestDontAskAgainWithUploadUploadsLaterEditsByThemselves`), and the
    package sometimes passes. These look flaky, not broken.
  - The fixes' own tests in `driver/internal/render` pass.
- `golangci-lint run ./...`, as is and with `GOOS=linux`: 8 findings
  each, not 0, all in files the fixes do not touch (`syntax/golang.go`,
  `text/colr.go`, `text/colr_test.go`, `input/input.go`, `theme/theme.go`,
  `syntax/golang_test.go`). Master has these findings too. The fixes add none.

## Other findings

- `go vet` on master reports three "possible misuse of unsafe.Pointer" in
  `driver/desktop/uia_windows.go` (lines 746, 778, 806). Not looked at.
