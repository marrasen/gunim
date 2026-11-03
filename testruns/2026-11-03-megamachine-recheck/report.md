# WASAPI recheck after PR #284, 2026-11-03, megamachine

A recheck for issue #260 of PR #284 (merged): the lead after a reopen, and the debug log of WASAPI's stamps.

## Machine

Same machine as [the first run](../2026-11-03-megamachine/report.md):

- MSI MPG Infinite X2 13FTD, Windows 11 Home 10.0.26300
- Go 1.27.1 windows/amd64
- Commit tested: `a27e60f` (origin/master, after #284, #286 and #287)
- Audio device: MOTU M6 (shows as "MOTU M Series")

Both runs use `example/roundtrip` with `GUNIM_DEBUG_AUDIO=1`, through WASAPI. In 1-2 is the default recording device in both. Roundtrip measures input channel 0, which is In 1.

| Run | Output | Input on In 1 | Log | Screenshot |
|---|---|---|---|---|
| A | Out 1-2, the monitors | Mono mic, 0.33 m from the monitors | [A-mic.log](A-mic.log) | [A-mic.png](A-mic.png) |
| B | Out 3-4, nothing else plugged into it | Cable from Line Out 3 to In 1, with no mic. In 1's gain was high and the input clipped (level 1.00) | [B-cable.log](B-cable.log) | [B-cable.png](B-cable.png) |

The PR asks for a cable from Out 1 to In 1. Run B uses Out 3 instead, with Out 3-4 as the default output, so the click goes into the cable and not into the monitors.

Each run had three opens:
1. at 48000 Hz,
2. after the output's default format was set to 44100 Hz in `mmsys.cpl`,
3. after it was set back to 48000 Hz.

Measurements were taken on opens 1 and 3.

## 1. The lead through a rate change: it no longer grows

| | A: open 1 | A: 44.1k | A: open 3 | B: open 1 | B: 44.1k | B: open 3 |
|---|---|---|---|---|---|---|
| Working ahead | 38.1, then 55.1 ms | 51.1 ms | 48.1 ms | 38.1 ms | 41.1 ms | **38.1 ms** |
| Ran dry | 2, at 19:14:56 | 0 | 0 | 0 | 0 | 0 |
| Device delay | 41.3 ms | 49.8 ms | **65.4 ms** | 42.8–44.8 ms | 46–48 ms | 48.0 ms |
| Latency (window) | 66 ms | 75.4 ms | **90 ms** | 57.5–59.4 ms | 61.5–63.6 ms | 62.7 ms |
| Clock first moved | 3.0 ms | 384 ms | 380 ms | 8.2 ms | 381 ms | 378 ms |

- **Run B: pass.** No dropouts, and the lead went 38.1 → 41.1 → 38.1 ms. The reopen added nothing.
- **Run A: pass for the lead.** The output ran dry twice, 2 s after start-up and long after the clock first moved (3 ms). The lead then grew, and logged 55.1 ms. Through the rate change it came back to 48.1 ms, which is 38.1 ms plus the 10 ms margin, as the PR expects after one dry spell. The reopens grew nothing.
- **The latency did not come back in run A.** The device delay as the clock reads it went from 41.3 to 65.4 ms after the reopen at 48000 Hz, +24 ms, so the window's latency went from 66 to 90 ms. The first run had the same jump (44 → 64.6 ms, [first report](../2026-11-03-megamachine/report.md), check 3). In run B, on Out 3-4, the device delay moved only +3.2 ms. The lead is fixed, but the device delay reading after a reopen still varies by up to about 24 ms.

## 2. The recording offset: present with a cable, and it changes between opens

### Offsets

| Run | Open | Offset | Round trip | Results |
|---|---|---|---|---|
| A (mic) | 1 | +22.9 to +23.5 ms | 98.5–99 ms | 5 |
| A (mic) | 3 | −8.5 to −8.6 ms | 92.1–92.2 ms | 3 |
| B (cable) | 1 | +22.2 to +22.3 ms | 91.8–92.4 ms | 4 distinct (5 taken; the log shows identical results once) |
| B (cable) | 3 | +7.6 ms | 80.9–81 ms | 3 |
| First run, mic | — | +12.8 ms | 77.5–79.9 ms | 9 |

**The cable does not remove the offset.** With no air and no mic it is still +22.2 ms. The mic on the same kind of open read +23.5 ms; the difference is about the 1.0 ms that sound takes to travel 0.33 m. So the offset comes from the clocks and stamps, not from the acoustics.

**Within one open the offset is steady to about 0.6 ms**, but it changes from one open to the next: +12.8, +23.5, −8.6, +22.2 and +7.6 ms.

### The stamp log

| | Open 1 (48 kHz) | 44.1 kHz | Open 3 (48 kHz) |
|---|---|---|---|
| A: soonest capture packet read after its stamp | 16–61 µs | 21.37 ms (470–471 frames) | 10.678 ms |
| B: soonest capture packet read after its stamp | 6.9–9.3 µs | 21.36 ms (470–471 frames) | 10.678 ms |
| A: output, frames written past the position told | 2149 | 2245–2251 | 3107–3116 |
| B: output, frames written past the position told | 2148–2149 | 2100–2149 | 2275–2282 |
| Output clock read after its stamp | 100–300 ns | 100–200 ns | 100 ns |

The input's period is 512 frames (10.667 ms) at 48 kHz and 470 frames (10.658 ms) at 44.1 kHz, as the log reports it.

What the log shows:

- **On open 1, in both runs, the soonest packet is read 7–61 µs after its stamp.** A full 512-frame packet can't be read 10 µs after its first frame was recorded. So on this device the stamp marks the packet's **end**, or at least it does on the first open. By the PR's rule, the fix is to subtract the packet's length from the stamp.
- **After a reopen, the soonest packet is read exactly one or two periods after its stamp:** 21.36 ms (two periods) at 44.1 kHz, then 10.678 ms (one period) back at 48 kHz, in both runs. The lag within an open varies by less than 50 µs. This is what the reader running one or two packets behind would look like. By the PR's rule it would also be read as the stamp marking the first frame. Either way, the same device gives a different lag on a later open.
- **The offset changes with that lag and with the device delay.** From open 1 to open 3:

| Run | Offset moved | Device delay moved | Soonest read lag moved | Delay + lag |
|---|---|---|---|---|
| A | −32.1 ms | +24.1 ms | +10.7 ms | 34.8 ms |
| B | −14.6 ms | +3.2 ms | +10.7 ms | 13.9 ms |

  In both runs the offset dropped by about the device delay's rise plus one input period, within 0.7 ms in B and 2.7 ms in A. So both ends seem to move between opens: the output's delay reading, and the input's stamp-to-packet relation, by one period.

- On open 1, the lag is near zero, and the offset is still +22.2 ms over the cable. The DAC and ADC take only about 1–2 ms of that. So about 20 ms is unexplained even on the open where the input's stamp looks like the packet's end.

### Step 4: offset after a rate change

The PR's step 4 says that if the offset moves by about as much as the device delay moved, then the output's delay reading is what changed. It moved by more than that in both runs: by the device delay's change plus about one input period (table above). So the output's delay reading explains part of the change, and the input's one-period shift explains the rest.

## Not done

- Open 1's offset over the cable (+22.2 ms) has not been split further. Measuring several fresh starts, without rate changes, would show whether open 1 always gives near-zero lag and about +22 ms.
- Run B's input clipped (level 1.00). The click was still found every time, and the timing came out steady.
