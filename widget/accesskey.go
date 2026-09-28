package widget

import (
	"image/color"
	"strings"
	"unicode"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// accessKey splits s, which may mark its access key with a & before it, into the text shown, the key in lower case,
// and the key's rune index in the text shown. With no mark the key is the first letter or digit; && shows a &.
func accessKey(s string) (shown string, key rune, at int) {
	at = -1
	if strings.Contains(s, "&") {
		var b strings.Builder
		rs := []rune(s)
		n := 0
		for i := 0; i < len(rs); i++ {
			r := rs[i]
			if r == '&' {
				if i+1 >= len(rs) || rs[i+1] != '&' {
					if i+1 < len(rs) && at < 0 {
						key, at = unicode.ToLower(rs[i+1]), n
					}
					continue
				}
				i++
			}
			b.WriteRune(r)
			n++
		}
		shown = b.String()
		if at >= 0 {
			return shown, key, at
		}
	} else {
		shown = s
	}
	for i, r := range []rune(shown) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return shown, unicode.ToLower(r), i
		}
	}
	return shown, 0, -1
}

// keyRune returns the character k's key types, in lower case, for matching an access key, or 0.
func keyRune(k input.KeyPress) rune {
	switch {
	case k.Char != 0:
		return unicode.ToLower(k.Char)
	case k.Key >= input.KeyA && k.Key <= input.KeyZ:
		return 'a' + rune(k.Key-input.KeyA)
	case k.Key >= input.Key0 && k.Key <= input.Key9:
		return '0' + rune(k.Key-input.Key0)
	}
	return 0
}

// altKey reports whether k is either Alt key.
func altKey(k input.Key) bool { return k == input.KeyLeftAlt || k == input.KeyRightAlt }

// altOnly reports whether mods hold Alt, and Shift or nothing else, as an access key is pressed with.
func altOnly(mods input.Mods) bool { return mods&^input.ModShift == input.ModAlt }

// underline draws a line under rune at of run, drawn at pos, in ink.
func underline(p *paint.Painter, run text.Run, at int, pos geom.Point, ink color.NRGBA) {
	if at < 0 {
		return
	}
	x0, x1 := run.CaretX(at), run.CaretX(at+1)
	p.RRect(geom.Rc(pos.X+min(x0, x1), pos.Y+run.Ascent+1.5, abs32(x1-x0), 1), 0, paint.Solid(ink))
}
