# WASAPI recheck after PR #291, 2026-11-03, megamachine

The checks are from [#260's comment](https://github.com/marrasen/gunim/issues/260#issuecomment-6615415660), for the change in #291. They follow [recheck2](../2026-11-03-megamachine-recheck2/report.md).

## Machine

- MSI MPG Infinite X2 13FTD, Windows 11 Home 10.0.26300
- Go 1.27.1 windows/amd64
- Commit tested: `8331ec0` (origin/master, the merge of #291)
- Audio device: MOTU M6 (shows as "MOTU M Series")

`example/roundtrip`, through WASAPI, with `GUNIM_DEBUG_AUDIO=1`:
- Out 1-2 (the monitors) is the default output, and In 1-2 the default recording device.
- A mono mic is on In 1, 0.33 m from the monitors.
- One run, from a fresh start: [rate-1.log](rate-1.log), screenshot [rate-1.png](rate-1.png).
- Out 1-2 was set to 44100 Hz in `mmsys.cpl` at 22:24:25, and back to 48000 Hz at 22:24:46.
- The three measurements came about 80 s after the return. A script started them by posting clicks to the window.

The monitors were turned far down until after the return to 48 kHz. That is why the mic shows a peak of about 0.002 before then. The silence at 44.1 kHz was heard, and the log confirms it (below).

## Summary

| Expected | Result |
|---|---|
| At each reopen, one "opening it once more" line and one "opened once more" line | **The 44.1 kHz reopen: yes**, one of each. **The return to 48 kHz: neither**, because its clock first moved after 2.0 ms, under 100 ms. |
| The second start under 100 ms | **No.** The second start at 44.1 kHz took 370.8 ms: "past 100ms again: it plays as it is". |
| Back at 48 kHz, about 960 frames taken by the engine (was 1596–1602) | **Pass**, at 1023 (958–964 on the fresh open) |
| Back at 48 kHz, a device delay near 44 ms (was 57.9–58.3 ms) | **Pass**, at 41.0–41.9 ms |
| Offsets after the return near +12.3 ms (was −2.0 ms) | **Pass**, at +12.8, +12.7 ms and the same again (round trip 79.0–79.1 ms) |
| (not in the steps) Sound at 44.1 kHz | **Fail: silent for the whole 20 s at 44.1 kHz.** The output lines show 1126 frames in the buffer and **0 taken by the engine**. Recheck2's 44.1 kHz reopen played, with 1022 taken. |

## The log lines asked for

### Fresh open, 48 kHz

```
22:24:09 speaker: WASAPI: the device's clock first moved 1.5674ms after it started, to frame 7; by then 1728 frames were written, 320 of them silence, and its buffer held 704, so the engine had taken 1024, 1017 past the frame the clock told
22:24:09 speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 2090 frames written past the position it told: 1126 in its buffer, 964 taken by the engine; 320 frames of silence written since it opened; device delay 44.020833ms
22:24:14 speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 2084 frames written past the position it told: 1126 in its buffer, 958 taken by the engine; 320 frames of silence written since it opened; device delay 44.020833ms
```

There was no "once more" line, as expected on a fresh open.

### The 44.1 kHz reopen

```
22:24:25 speaker: WASAPI: wasapi: the device went away; opening the default device again
22:24:25 speaker: WASAPI: Out 1-2 (MOTU M Series) plays at 44100 Hz and records 2 channels, where the last played at 48000 Hz and recorded 2; opening the output again
22:24:25 speaker: opened again, at 44100 Hz, working 41.133786ms ahead
22:24:26 speaker: WASAPI: the device's clock first moved 381.2349ms after it started, past 100ms: the endpoint was still warming up, and its clock may miss frames for the whole open; opening it once more
22:24:26 speaker: WASAPI plays Out 1-2 (MOTU M Series) again
22:24:26 speaker: WASAPI: opened once more, the device's clock first moved 370.7876ms after it started, past 100ms again: it plays as it is
22:24:30 speaker: WASAPI: the device's clock first moved 370.7876ms after it started, to frame 688; by then 1376 frames were written, 0 of them silence, and its buffer held 688, so the engine had taken 688, 0 past the frame the clock told
22:24:30 speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 512 frames written past the position it told: 512 in its buffer, 0 taken by the engine; 336 frames of silence written since it opened; device delay 25.804988ms
22:24:35 speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 1126 frames written past the position it told: 1126 in its buffer, 0 taken by the engine; 336 frames of silence written since it opened; device delay 25.827664ms
```

The 22:24:40 and 22:24:45 output lines read the same: 1126 in the buffer and 0 taken by the engine, with a device delay of 25.4–25.8 ms. The window showed a device delay of 25.1–26.0 ms and a latency of 40.7–41.6 ms, and the "ran dry" count went from 0 to 1 at this reopen. The log has no "clock stood still" line.

**Nothing was heard at 44.1 kHz**, from the reopen until the return to 48 kHz.

For comparison, recheck2's 44.1 kHz reopen started once, after 380 ms, and then played. Its output line:

```
WASAPI output: ... with 2148 frames written past the position it told: 1126 in its buffer, 1022 taken by the engine; 0 frames of silence written since it opened; device delay 45.941043ms
```

### Return to 48 kHz

```
22:24:46 speaker: WASAPI: wasapi: the device went away; opening the default device again
22:24:46 speaker: WASAPI: Out 1-2 (MOTU M Series) plays at 48000 Hz and records 2 channels, where the last played at 44100 Hz and recorded 2; opening the output again
22:24:46 speaker: opened again, at 48000 Hz, working 38.125ms ahead
22:24:51 speaker: WASAPI: the device's clock first moved 1.9949ms after it started, to frame 2; by then 1728 frames were written, 320 of them silence, and its buffer held 704, so the engine had taken 1024, 1022 past the frame the clock told
22:24:51 speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 2150 frames written past the position it told: 1126 in its buffer, 1024 taken by the engine; 320 frames of silence written since it opened; device delay 41.916666ms
22:24:56 speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 2149 frames written past the position it told: 1126 in its buffer, 1023 taken by the engine; 320 frames of silence written since it opened; device delay 41.9375ms
```

This time the clock moved 2.0 ms after the start, against 378 ms in recheck2. So there was no "opening it once more", and none was needed. The engine took 1023–1024 frames, close to a fresh open, and the device delay is 41.9 ms. Later output lines read the same: 1023 taken, 41.0–41.9 ms. One reading at 22:25:16 had 706 taken.

## Measurements after the return

| Click | Offset | Round trip | Level |
|---|---|---|---|
| 1 | +12.8 ms | 79.1 ms | 0.19 |
| 2 | +12.7 ms | 79.0 ms | 0.19 |
| 3 | the same as one of these | | |

The log shows a result once when it is new, and then again every 5 s, so it holds only the two distinct results. The offset is back near a fresh open's +12.3 to +12.9 ms (recheck2, check 1). After a rate change in recheck2 it was −2.0 ms.

Stamps moved back 52–62 µs at 48 kHz after the return, with the soonest packet read 10.68 ms after its stamp. That is the same pattern as recheck2's cable starts.

## Run 2: the silence repeats

This is a second fresh start with the same steps: [rate-2.log](rate-2.log), [rate-2.png](rate-2.png). This time the monitors stayed at a normal level throughout. Out 1-2 was set to 44100 Hz at 22:28:31 and back to 48000 Hz at 22:29:02. The three clicks came about 40 s after the return.

**At 44.1 kHz it was silent again, and the mic shows it.** The window's microphone peak, the loudest sample in the last second:

| | Peak |
|---|---|
| 48 kHz, fresh | 0.131–0.132 |
| 44.1 kHz | **0.002–0.012** |
| 48 kHz, after the return | 0.130–0.133 |

While it was silent, the output meter for Out 1-2 in `mmsys.cpl` was moving. So the endpoint was taking the stream's samples, but nothing reached the monitors.

The reopen logged the same way as in run 1:

```
22:28:31 speaker: WASAPI: the device's clock first moved 380.5711ms after it started, past 100ms: the endpoint was still warming up, and its clock may miss frames for the whole open; opening it once more
22:28:31 speaker: WASAPI plays Out 1-2 (MOTU M Series) again
22:28:32 speaker: WASAPI: opened once more, the device's clock first moved 370.9657ms after it started, past 100ms again: it plays as it is
22:28:36 speaker: WASAPI: the device's clock first moved 370.9657ms after it started, to frame 688; by then 1376 frames were written, 0 of them silence, and its buffer held 688, so the engine had taken 688, 0 past the frame the clock told
22:28:36 speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 1126 frames written past the position it told: 1126 in its buffer, 0 taken by the engine; 0 frames of silence written since it opened; device delay 26.031746ms
22:28:41 speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 1126 frames written past the position it told: 1126 in its buffer, 0 taken by the engine; 0 frames of silence written since it opened; device delay 26.00907ms
```

All six output lines at 44.1 kHz, from 22:28:36 to 22:29:01, read 1126 in the buffer and 0 taken by the engine, with a device delay of 25.1–26.1 ms. Nothing logged a stall or a dropout.

The return to 48 kHz again started at once, without a second open:

```
22:29:07 speaker: WASAPI: the device's clock first moved 3.499ms after it started, to frame 1; by then 1728 frames were written, 320 of them silence, and its buffer held 704, so the engine had taken 1024, 1023 past the frame the clock told
22:29:07 speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 1897 frames written past the position it told: 1126 in its buffer, 771 taken by the engine; 320 frames of silence written since it opened; device delay 42.5ms
22:29:12 speaker: WASAPI output: the clock read 100ns after its stamp, the soonest in 5s, with 2149 frames written past the position it told: 1126 in its buffer, 1023 taken by the engine; 320 frames of silence written since it opened; device delay 42.5ms
```

After the return:
- The device delay read 41.7–42.5 ms, and the engine took 765–1024 frames.
- At 22:29:11, 9 s after the return, the output `ran dry 2 times in 250ms`, and the lead grew to 53.3 ms. The fresh start in run 1 had no dropout.
- Of the three clicks, one measured **+12.2 ms, round trip 89.3 ms, level 0.19**. The other two returned `capture: the click was not heard in the recording`.

## Open

- **The silent 44.1 kHz open, in 2 of 2 runs.** Both times the endpoint started late, was opened once more, and started late again (370.8 and 371.0 ms). After that the engine took no frames and nothing played, while the `mmsys.cpl` meter moved. The device delay read about 26 ms, and nothing logged a stall. In recheck2, the one open at 44.1 kHz, also late (380 ms), played normally with 1022 frames taken. So the second open, made right after the first, seems to be what leaves the endpoint silent.
- In both runs the return to 48 kHz started at once. So neither run shows what a second start does after a late *48 kHz* start.
- Two of run 2's three measurements did not hear the click. That run also had a dropout 9 s after the return.
