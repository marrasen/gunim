# oto WASAPI: frames played, speakers and Bluetooth earbuds (#438)

For the upstream PR ebitengine/oto#311.

## Summary

Checks 1, 2, 3, 5 pass, and check 4 fails. The driver reports a steady delay, with no errors, and it follows pause,
suspend and device switches. But Windows reports the same delay for the Bluetooth earbuds as for the speakers, so
`UnplayedSize` doesn't count the earbuds' real latency of about 230 ms.

| Check | Device | `delay` while playing | TAP median, UnplayedSize / BufferedSize | Result |
|---|---|---|---|---|
| 1. Report | Speakers | 60–70 ms | | pass |
| 2. Tap | Speakers | | +32 / +100 ms | pass |
| 3. Report | WF-1000XM4 | 60–70 ms | | pass on every point but one: the earbuds' `delay` is no larger than the speakers' |
| 4. Tap | WF-1000XM4 | | +263 / +330 ms | **fail**: UnplayedSize medians 231 ms apart (limit about 40) |
| 5. Switch | both | back above 0 within 1.25 s | | pass |

## Machine

- **Machine:** MarcusXPS25 laptop, Windows 11 Pro 10.0.26300.
- **Speakers:** "Speakers (2- SoundWire Speakers)", the built-in laptop speakers.
- **Earbuds:** "Headphones (WF-1000XM4)", Sony WF-1000XM4 over Bluetooth.
  - **Multipoint:** the earbuds are usually also connected to a phone. Bluetooth was off on the phone for checks
    3–5, but the earbuds' multipoint setting may still have kept them off LDAC.
- **Go:** go1.26.0 windows/amd64.
- **oto:** marrasen/oto `eb878df` (windows-played-test). `go build ./...` and `go test ./...` pass.

## Order of the runs

The earbuds were already the default output when the run started, so the first report and tap runs were on the
earbuds. They are kept as checks 3 and 4: `3-bt-report-first.log` and `4-bt-tap.log`. Checks 1 and 2 were then run
on the speakers, and check 3 was run once more on the earbuds (`3-bt-report.log`). All sound in each run came from
the intended device, as the tester confirmed.

## 1. Speakers, report: pass

[1-speakers-report.log](1-speakers-report.log)

- **Playing:** `delay` was between 60 and 70 ms.
- **Pause:** it was 0 by the first line, within 0.1 s.
- **Suspended:** it was 0 on every line.
- **Resumed:** it was 61 ms on the first line, 0.1 s after the resume.
- **Cycles:** all five showed `delay` above 0, from 60 to 63 ms.
- **Errors:** `err=<nil>` on every line.

## 2. Speakers, tap: pass

[2-speakers-tap.log](2-speakers-tap.log)

`TAP median offset past the click: with UnplayedSize +32 ms, with BufferedSize +100 ms, 20 presses`

The tester pressed in time with the once-a-second beat rather than reacting to each click, which is why the medians
are below a reaction time. It was done the same way in check 4, so it cancels out of the comparison.

## 3. Earbuds, report: pass, but the delay isn't larger

[3-bt-report.log](3-bt-report.log), [3-bt-report-first.log](3-bt-report-first.log)

Pause, suspend, resume and cycles behaved as in check 1, with `err=<nil>` throughout. But `delay` while playing was
60–70 ms in both runs, the same as the speakers. The issue expected it to be clearly larger, likely 100 to 300 ms.
Windows doesn't count the earbuds' latency in this figure here.

## 4. Earbuds, tap: fail

[4-bt-tap.log](4-bt-tap.log)

`TAP median offset past the click: with UnplayedSize +263 ms, with BufferedSize +330 ms, 20 presses`

| | Speakers | WF-1000XM4 | Difference |
|---|---|---|---|
| UnplayedSize median | +32 ms | +263 ms | 231 ms |
| BufferedSize median | +100 ms | +330 ms | 230 ms |
| `delay` reported | 60–70 ms | 60–70 ms | about 0 |

To pass, the UnplayedSize medians had to be within about 40 ms of each other. They are 231 ms apart, so the earbuds
play about 230 ms later than the speakers, and `UnplayedSize` doesn't show any of it.

## 5. Switching: pass

[5-switch.log](5-switch.log), [5-switch-first.log](5-switch-first.log)

The first run, `5-switch-first.log`, had no record of when each step was done, so it was repeated. For the second
run, `switch` mode in `internal/wintest` was changed locally:
- **Marks:** a goroutine logs `MARK n` with the time on each Enter press.
- **Sampling:** it logs every 0.25 s instead of every 1 s.

The change isn't part of the oto commit tested.

The earbuds started as the default output.

| Step (mark) | Gap with `delay` at 0 | `delay` after |
|---|---|---|
| 1. Earbuds into the case (13.7 s) | 13.6–14.4 s | 49–58 ms, speakers |
| 2. Earbuds out (25.4 s) | 25.9–26.4 s | 105–117 ms, earbuds |
| 3. Default switched in Sound settings (38.4 s) | 37.4–38.4 s | 60–69 ms, speakers |
| 4. Default switched back (49.4 s) | 48.96 s, one line | 61–73 ms, earbuds |

- **Errors:** no `ERROR`, and `err=<nil>` on every line, in both runs.
- **Recovery:** after each switch the clicks went on from the new device, and `delay` was above 0 again within
  1.25 s.
- **Sound:** the tester heard only a short silence at each switch, with no stutter, crackle or repeated click.

**Observation:** after the earbuds reconnected in step 2, Windows reported about 110 ms for them. Before step 1 and
after step 4, it reported 62–73 ms. Even 110 ms is about 120 ms short of what check 4 measured. Not investigated.
