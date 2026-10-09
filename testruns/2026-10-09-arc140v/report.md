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

## 3. Does it look the same: PASS

The mipmap fix changes the two paths that used `glGenerateMipmap`: wide
blurs and images drawn smaller than their size. The stress only proves
that nothing freezes, so a temporary test (removed since) drew the same
scenes offscreen with master's renderer and with the fix, on this GPU,
and compared them pixel by pixel.

| Scene | Master against the fix |
|---|---|
| Backdrop blur 4 and 12, over 1-pixel stripes, a checkerboard and blocks | identical |
| Backdrop blur 30 and 80 | at most 1 level in 255 |
| Layer blur 4 to 80, over the same | identical, or at most 1 level |
| A 512-pixel zone plate drawn at 290, 180, 120, 80, 50 and 33 pixels, at offsets of 0, 0.25 and 0.5 pixels | at most 22 levels, on 5% of the pixels, nearly all at 290 |

Against an exact area-average downscale made on the CPU, the zone plate at
290 pixels is closer with the fix (mean error 14.1 levels, master 19.1);
at the other sizes the two are within 0.1. In a 3× zoom both show the same
faint moiré in the finest rings, a little weaker with the fix. The
difference is the same at every sub-pixel offset, so an image moving does
not shimmer more.

The issue's manual checks (opening and closing windows, Settings, a
dropdown's popup, dragging a pane out, resizing) were not done by hand: the
stress did each of these thousands of times without a freeze, and the
fixes do not touch the code that opens, sizes or presents windows.

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
