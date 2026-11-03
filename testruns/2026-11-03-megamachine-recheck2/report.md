# WASAPI recheck after PR #290, 2026-11-03, megamachine

The checks from [#260's comment](https://github.com/marrasen/gunim/issues/260#issuecomment-6614105937) on the changes in #290. They follow [the first recheck](../2026-11-03-megamachine-recheck/report.md).

## Machine

- MSI MPG Infinite X2 13FTD, Windows 11 Home 10.0.26300
- Go 1.27.1 windows/amd64
- Commit tested: `6a3ac72` (origin/master, the merge of #290)
- Audio device: MOTU M6 (shows as "MOTU M Series")

All runs use `example/roundtrip` with `GUNIM_DEBUG_AUDIO=1`, through WASAPI, with In 1-2 as the default recording device. Roundtrip measures input channel 0, which is In 1. A script started each run and, 10 s after the window opened, posted three left clicks to the window (`WM_LBUTTONDOWN`/`UP`) 5 s apart. It then took a screenshot and closed the window. The mouse was not moved. In check 2 the format was changed by hand, and the clicks came 10 s after that.

The log shows a result once when it is new, and then every 5 s. Two clicks with the same result show as one line.

No run logged a "ran dry" line.

## Summary

| Check | Expected | Result |
|---|---|---|
| 1. Three fresh starts, mic | "stamps moved back" about 10.6 ms each time; offsets within about 1 ms, near +12.8 ms | **Pass.** 10.667 ms on every start; offsets +12.3 to +12.9 ms across all three |
| 2. Rate change, mic | "stamps moved back 0s" after the reopen; offset near −8.6 ms if the delay jump recurs | **Mostly as expected.** 0 s at 44.1 kHz and 110.6 µs back at 48 kHz. The delay jump recurred, but at +13.9 ms rather than +24 ms, and the offset was −2.0 ms, which is where that jump puts it |
| 3. Two fresh starts, cable | About +11.5 ms both times; "stamps moved back" about 10.6 ms | **Offset passes, at +11.6 ms on both starts.** But "stamps moved back" was only 36–71 µs, because the soonest packet was read one period after its stamp on these starts |

## 1. Three fresh starts, Out 1-2 with the mic: pass

The mic is on In 1, about 0.33 m from the monitors on Out 1-2.

| Start | Soonest read after stamp | Stamps moved back | Device delay | Offsets | Round trip | Log |
|---|---|---|---|---|---|---|
| 1 | 10.8–11.6 µs | 10.667 ms | 44.0 ms | +12.3, +12.8, +12.3 ms | 81.6–82.1 ms | [1-mic-1.log](1-mic-1.log), [png](1-mic-1.png) |
| 2 | 10.6–11.7 µs | 10.667 ms | 44.0 ms | +12.9, +12.3, +12.4 ms | 81.7–82.2 ms | [1-mic-2.log](1-mic-2.log), [png](1-mic-2.png) |
| 3 | 5.8–8.8 µs | 10.667 ms | 44.8 ms | +12.4, +12.3, +12.3 ms | 82.2–82.4 ms | [1-mic-3.log](1-mic-3.log), [png](1-mic-3.png) |

All nine offsets fall within 0.6 ms, between +12.3 and +12.9 ms. The same kind of open gave +23.5 ms before #290.

## 2. Rate change round trip on Out 1-2 with the mic

Log: [2-rate-1.log](2-rate-1.log), screenshot: [2-rate-1.png](2-rate-1.png). The run started fresh at 48000 Hz. Out 1-2 was set to 44100 Hz at 21:06:52 and back to 48000 Hz at 21:07:05. The clicks came about 10 s after the return.

From the fresh open:

```
speaker: WASAPI: the device's clock first moved 7.6709ms after it started, to frame 2; by then 2150 frames were written, 320 of them silence, and its buffer held 1126, so the engine had taken 1024, 1022 past the frame the clock told
speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 2084 frames written past the position it told: 1126 in its buffer, 958 taken by the engine; 320 frames of silence written since it opened; device delay 44.020833ms
speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 2090 frames written past the position it told: 1126 in its buffer, 964 taken by the engine; 320 frames of silence written since it opened; device delay 44.041666ms
```

From the return to 48 kHz:

```
speaker: WASAPI: the device's clock first moved 378.047ms after it started, to frame 1; by then 2816 frames were written, 0 of them silence, and its buffer held 1088, so the engine had taken 1728, 1727 past the frame the clock told
speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 2728 frames written past the position it told: 1126 in its buffer, 1602 taken by the engine; 0 frames of silence written since it opened; device delay 57.916666ms
speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 2722 frames written past the position it told: 1126 in its buffer, 1596 taken by the engine; 0 frames of silence written since it opened; device delay 57.9375ms
```

For completeness, the reopen at 44.1 kHz:

```
speaker: WASAPI: the device's clock first moved 380.2774ms after it started, to frame 688; by then 1376 frames were written, 0 of them silence, and its buffer held 688, so the engine had taken 688, 0 past the frame the clock told
speaker: WASAPI output: ... with 2148 frames written past the position it told: 1126 in its buffer, 1022 taken by the engine; 0 frames of silence written since it opened; device delay 45.941043ms
```

| | Fresh 48 kHz | 44.1 kHz | Back at 48 kHz |
|---|---|---|---|
| Clock first moved | 7.7 ms, to frame 2 | 380 ms, to frame 688 | 378 ms, to frame 1 |
| Engine past the frame told, at first move | 1022 | 0 | **1727** |
| Taken by the engine, steady | 958–964 | 1022–1023 | **1596–1602** |
| Device delay | 44.0 ms | 45.5–45.9 ms | **57.9–58.3 ms** |
| Soonest read after stamp | 10–11 µs | 21.36 ms | 10.68 ms |
| Stamps moved back | 10.667 ms | **0 s** | **110.6 µs** |
| Working ahead | 38.1 ms | 41.1 ms | 38.1 ms |
| Offset / round trip | (+12.3 ms in check 1) | — | **−2.0 ms / 81.2 ms** (3 clicks, all the same) |

- **The delay jump recurred, at +13.9 ms this time** (+24 ms in the first recheck). It is all frames the engine took: 1596–1602 against 958–964, a rise of about 640 frames (13.3 ms). At the clock's first move the engine was already 1727 frames past the frame told, against 1022 on the fresh open: 705 frames, or 14.7 ms, more. The buffer stayed 1126 frames.
- **The offset moved with the delay:** +12.3 ms on a fresh open, minus 13.9 ms, gives −1.6 ms; it measured −2.0 ms. The expected −8.6 ms was for a 24 ms jump.
- **Stamps moved back** 0 s at 44.1 kHz, as expected. Back at 48 kHz they moved back 110.6 µs (about 5 frames), not exactly 0.
- For a moment, right after the return, the window showed a device delay of 23.5 ms and a latency of 38.1 ms. From 5 s later it read 57.9–58.3 ms.
- The lead went 38.1 → 41.1 → 38.1 ms, with no dropouts.

## 3. Two fresh starts, cable from Line Out 3 to In 1, Out 3-4 as default

The mic is unplugged, and In 1's gain is turned down: peak 0.474, level 0.97, no longer 1.00.

| Start | Clock first moved | Soonest read after stamp | Stamps moved back | Device delay | Offsets | Round trip | Log |
|---|---|---|---|---|---|---|---|
| 1 | 18.2 ms, to frame 1; engine 1023 past | 10.680–10.681 ms | 36–71 µs | 40.8 ms | +11.6 ms (3 clicks, the same) | 76.3–77.7 ms | [3-cable-1.log](3-cable-1.log), [png](3-cable-1.png) |
| 2 | 15.9 ms, to frame 2; engine 1022 past | 10.680–10.681 ms | 39–63 µs | 40.8 ms | +11.6 ms (3 clicks, the same) | 76.3–77.7 ms | [3-cable-2.log](3-cable-2.log), [png](3-cable-2.png) |

- **The offset is +11.6 ms on both starts**, against an expected ~11.5 ms. It was +22.2 ms before #290.
- **"Stamps moved back" was not about 10.6 ms.** On both cable starts, the soonest packet was read one period (10.68 ms) after its stamp. In check 1 it was read within microseconds. The stamps moved back only 36–71 µs, but the offset still came out where expected. The difference between check 1 and check 3 is the output: Out 1-2 with a mic in check 1, Out 3-4 with a cable in check 3. Both read In 1-2 with a 512-frame period.
- The mic's +12.3 ms against the cable's +11.6 ms leaves 0.7 ms, close to the ~1 ms that sound takes over 0.33 m.

## What is left

- After a reopen, the device delay reads longer by however many more frames the engine took before the clock first moved: +13.9 ms here and +24 ms before. The offset follows it, so a recording made after a rate change is off by that much. The two logged lines above, from the fresh open and from the return, show where the extra frames are.
- What is left of the offset: +11.6 ms over a cable, and +12.3 ms with the mic, on fresh opens. It is steady across starts.
