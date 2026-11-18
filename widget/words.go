package widget

import (
	"strings"
	"unicode"
)

// A word is a run of runes that are neither spaces nor stops: the runes
// in [WordStops], passed to these as stops.

// inWord reports whether r is part of a word.
func inWord(r rune, stops string) bool {
	return !unicode.IsSpace(r) && !strings.ContainsRune(stops, r)
}

// wordStart returns where the word before i starts, passing over the
// spaces and stops before it: where Ctrl+Left takes the caret from i.
func wordStart(rs []rune, i int, stops string) int {
	i = max(0, min(i, len(rs)))
	for i > 0 && !inWord(rs[i-1], stops) {
		i--
	}
	for i > 0 && inWord(rs[i-1], stops) {
		i--
	}
	return i
}

// wordEnd returns where the word after i ends, passing over the spaces
// and stops before it: where Ctrl+Right takes the caret from i.
func wordEnd(rs []rune, i int, stops string) int {
	i = max(0, min(i, len(rs)))
	for i < len(rs) && !inWord(rs[i], stops) {
		i++
	}
	for i < len(rs) && inWord(rs[i], stops) {
		i++
	}
	return i
}

// wordAt returns the word at rune i: the one i is in, or the one it ends,
// for a click just after a word. On a stop with no word ending there it
// returns the run of stops, so a double click on "//" selects both. On a
// space with no word ending there it returns i alone, so a double click
// or a long press on the space after a text's last word selects nothing
// past it.
func wordAt(rs []rune, i int, stops string) (start, end int) {
	i = max(0, min(i, len(rs)))
	j := i
	if j == len(rs) || !inWord(rs[j], stops) {
		switch {
		case j > 0 && inWord(rs[j-1], stops):
			j--
		case j < len(rs) && !unicode.IsSpace(rs[j]):
			// A stop: take the run of stops round it.
			start, end = j, j+1
			for start > 0 && isStop(rs[start-1], stops) {
				start--
			}
			for end < len(rs) && isStop(rs[end], stops) {
				end++
			}
			return start, end
		default:
			return i, i
		}
	}
	start, end = j, j
	for start > 0 && inWord(rs[start-1], stops) {
		start--
	}
	for end < len(rs) && inWord(rs[end], stops) {
		end++
	}
	return start, end
}

// isStop reports whether r is one of stops and not a space.
func isStop(r rune, stops string) bool {
	return !unicode.IsSpace(r) && strings.ContainsRune(stops, r)
}
