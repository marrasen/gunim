# Test run 2026-10-03, rtx3070 (Windows)

The checks from issue #20 on the `android-driver` branch.

- **Commit tested:** `05a592b` Light nothing under a finger, and give a chat message a menu
- **OS:** Windows 11 Home, 10.0.26300
- **GPU:** NVIDIA GeForce RTX 3070, driver 32.0.15.8157
- **Monitor:** one, 5120×1440 at 120 Hz, scale 100% (96 dpi)
- **Go:** go1.27.1 windows/amd64; golangci-lint 2.14.0
- **Input methods:** Swedish and US keyboards, and Microsoft Japanese IME, already installed for the user

All input was driven with `SendInput` (mouse, keys, and Unicode characters), and each screenshot was taken from the screen.

| Check | Result |
|---|---|
| 1. Tests and lint | Tests **pass**. Lint **fails**: 154 issues, 39 of them new on the branch; two fixed in the PR |
| 2. Typing and editing | **Pass** |
| 3. Input method composition | **Pass**, with a note on undo |
| 4. The edit menu | **Pass** |
| 5. The moved renderer | **Pass**; the two-refresh-rate part couldn't be run (one monitor) |

Fixes: the branch `fix/2026-10-03-rtx3070`, a pull request against `android-driver`.

## 1. Tests and lint

`go test ./...` passes ([1-test.log](1-test.log)). `TestWindowOpensAtItsPlacement` didn't fail.

`golangci-lint run ./...` reports **154 issues**, the same as Windows and with `GOOS=linux` ([1-lint-windows.log](1-lint-windows.log), [1-lint-linux.log](1-lint-linux.log)). `master` reports 116 with the same golangci-lint here. So this linter version finds issues the repo's usual one doesn't; the branch's own share is the difference.

The issues new on the branch, matched by file, message and linter rather than line: **39** ([1-lint-new-on-branch.txt](1-lint-new-on-branch.txt)).

- **Fixed in the PR** (both from the renderer's move):
  - `driver/desktop/desktop.go` and `window.go`: the new `driver/internal/render` and `inbox` imports were in the standard library's group (goimports).
  - `driver/internal/render/render.go`: the now exported `New` had no doc comment (revive).

  With these, `golangci-lint run ./driver/...` reports 0 issues as is and with `GOOS=linux`.
- **Left as they are:**
  - 20 in the new `tools/gunimapk`: unchecked `Close` errors, `exec.Command` without a context, shadowed `err`.
  - 16 in tests, examples, `input.go`, `ui.go`, `text/text.go` and `widget/handles.go`. Two notes:
    - `widget/textedit_test.go`'s `teh` misspellings look deliberate, a word for autocorrect to fix. They need a `//nolint:misspell` rather than a correction.
    - `text/text.go` uses `shaping.Glyph.XAdvance`, which is deprecated in favour of `Advance`.

## 2. Typing and editing

`example/widgets`, Notes box and Filter field.

- **Typing three lines** shows them as typed.
- **Drag, Ctrl+C, Ctrl+V, Ctrl+X, Ctrl+Z, Ctrl+Y** each do what they say ([2-edits.png](2-edits.png), top to bottom):
  - a drag selects "quick brow";
  - Ctrl+C, then Ctrl+V at the end of line 3, pastes it there;
  - a drag over "over the la" and Ctrl+X cuts it, and the clipboard holds it;
  - Ctrl+Z brings it back, selected;
  - Ctrl+Y cuts it again.
- **Double click** ([2-doubleclick.png](2-doubleclick.png)):
  - On a word, it selects the word.
  - After a space that follows the text's last word, it selects nothing and leaves the caret there ([2-dclick-after-trailing-space.png](2-dclick-after-trailing-space.png)).
  - Past the end of a line that ends in a word, it selects that word, as `wordAt` says it should ("the one it ends").
- **Emoji with Win+.**: the panel opens at the caret, and Enter inserts its first emoji. Then a space, "word x y" ([2-emoji-line.png](2-emoji-line.png), zoomed in [2-emoji-space.png](2-emoji-space.png)).
  - Measured in pixels between glyphs: 4, 5 and 3 px between letters, 7 px before the emoji and 8 px after it.
  - The 3–4 px extra on *both* sides is the emoji's own side bearing. The space after it is an ordinary space, not an emoji's width, which would be 15 px or more.
- **Filter field**: typing, double-clicking a word, Ctrl+X and Ctrl+Z work the same way ([2-filter.png](2-filter.png)).

## 3. Input method composition

Microsoft Japanese IME, in the Notes box.

- **Composing shows in place**, underlined, with the IME's candidate hint below it ([3a-composing.png](3a-composing.png)).
- **Commit and cancel** ([3-commit-cancel.png](3-commit-cancel.png)):
  - Space converts to 日本, shown highlighted, and Enter commits it. Exactly 日本 is left.
  - さくら, composed and cancelled with Escape, leaves the text as it was. A second Escape changes nothing.
- **Composing over a selection** ([3-over-selection.png](3-over-selection.png)): double-click "word", compose やま over it, convert and commit 山.
  - The composition replaces the selection while composing, and 山 is left after the commit.
  - **Note:** one Ctrl+Z undoes the commit, but leaves the selection deleted (`a b 🫠  x y`). A second Ctrl+Z brings "word" back. That meets the check, since Ctrl+Z undoes the commit. But replacing a word by composing over it takes two undos to reverse, where typing over a selection takes one.

## 4. The edit menu

- **Right click inside a selection** opens Cut, Copy, Paste and Select all at the pointer, and keeps the selection ([4-menu-on-selection.png](4-menu-on-selection.png)).
- **Copy**: "word" is on the clipboard, and pasted into Notepad it reads "word" ([4-notepad-paste.png](4-notepad-paste.png)).
- **Right click elsewhere** drops the selection; Cut and Copy are dimmed ([4b-menu-elsewhere.png](4b-menu-elsewhere.png)).
- **Paste** puts it at the pointer: 日本word ([4-paste-at-pointer.png](4-paste-at-pointer.png)).
- **Select all** selects everything, and the menu opens again with Cut and Copy now enabled ([4-selectall.png](4-selectall.png)). That is `editSelectAll` as written.
- **Cut** leaves the box empty, with all 86 characters, five lines, on the clipboard ([4-cut.png](4-cut.png)).
- **A click outside** an open menu closes it and leaves the text as it was ([4-dismiss.png](4-dismiss.png)).

The menu is a window of its own. While it is open, .NET's `Process.MainWindowHandle` can return the menu rather than the main window. A test script that finds the window that way aims its clicks wrongly, as mine first did.

## 5. The moved renderer

- **`twowindows -for 10s`**: one monitor only, so different refresh rates couldn't be checked.
  - Both windows hold 120 fps on the 120 Hz monitor from the second second ([5-twowindows.log](5-twowindows.log), [5-twowindows.png](5-twowindows.png)).
  - The first second shows 97 and 92 fps while the windows open.
- **`calculator`**:
  - 7 × 6 = animates the sum from the display into the tape, nothing missing or upside down ([5-calc-mid.png](5-calc-mid.png) mid-animation, [5-calc-sum.png](5-calc-sum.png) after).
  - Graph, then `sin(x)` with the keypad, draws the curve ([5-graph-sin.png](5-graph-sin.png)).
  - The button in the plot's top right corner grows it to fill the window ([5-graph-grow-mid.png](5-graph-grow-mid.png) mid-animation, [5-graph-grown.png](5-graph-grown.png)), and brings it back ([5-graph-back.png](5-graph-back.png)).
- **`GUNIM_DEBUG_FRAMES=1`** prints its lines ([5-calculator-frames.log](5-calculator-frames.log)).
  - 120 Hz, 0–4 late frames a second. Draw time is 0.1–0.5 ms, at most 1.5 ms, apart from one 14 ms frame.
  - The 1140×820 surface is the window's shadow, 80 px around the 980×660 window, while it animates.

## Rerun on fb1e7a1

The two checks from #22, on `fb1e7a1` (after `f3ed1f8`, `cc8206f` and `fb1e7a1`), on the same machine. `example/widgets`, Notes box.

| Check | Result |
|---|---|
| `go test ./...` | **Pass**: 24 packages pass, none fail ([rerun-test.log](rerun-test.log)) |
| 1. Typing and undo | **Pass** |
| 2. Composition, Microsoft Japanese IME | **Pass**: one Ctrl+Z now undoes a commit over a selection |

### 1. Typing and undo

- **Typing three lines, then the edits**, as in the first run ([rerun-1-edits.png](rerun-1-edits.png), top to bottom):
  - a drag selects "quick brow";
  - Ctrl+C, then Ctrl+V at the end of line 3, pastes it there;
  - Ctrl+X cuts "over the la", and the clipboard holds it;
  - Ctrl+Z brings it back, selected;
  - Ctrl+Y cuts it again.
- **A double letter undoes with its neighbour** ([rerun-1-double-letter.png](rerun-1-double-letter.png), zoomed 3×, top to bottom). Type `helo world`, click between `he` and `lo`, and type `l`, then `p`.
  - The text reads `helplo world`.
  - **One Ctrl+Z** takes away `lp` together and leaves `helo world`.
  - A second Ctrl+Z takes away ` world`, the typing before the click, and Ctrl+Y puts it back.
- **Emoji**: Win+., then Enter, then a space and `word x y` ([rerun-1-emoji-line.png](rerun-1-emoji-line.png), zoomed [rerun-1-emoji-space.png](rerun-1-emoji-space.png)). The measured gaps are the same as in the first run: 4, 5 and 3 px between letters, 7 px before the emoji, 8 px after it. The space after the emoji is an ordinary width; the extra is the emoji's own side bearing, on both sides.

### 2. Composition

- **Compose, then commit or cancel** ([rerun-2-commit-cancel.png](rerun-2-commit-cancel.png), top to bottom):
  - にほん shows in place, underlined.
  - Space converts it to 日本, and Enter commits it, leaving exactly 日本.
  - さくら, composed and cancelled with Escape, leaves the text as it was.
- **Over a selection** ([rerun-2-over-selection.png](rerun-2-over-selection.png), top to bottom):
  - Double-click `word` to select it; やま, composed over it, shows in its place.
  - **Escape** cancels it and leaves `word` there, still selected.
  - Double-click `word` again, compose やま, convert to 山, and commit. `a b 🫠 山 x y` is left.
  - **One Ctrl+Z** brings `word` back, selected. In the first run that took two.
