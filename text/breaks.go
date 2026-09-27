package text

import "unicode"

// breakText returns the runes the line breaker reads for runes, which
// has as many runes: in a run with no spaces that holds a slash or a
// backslash, as a path or a web address does, lines break only after a
// slash or a backslash, and within a part only when that part alone is
// too wide.
func breakText(runes []rune) []rune {
	var out []rune
	for start := 0; start < len(runes); {
		end := start
		for end < len(runes) && !unicode.IsSpace(runes[end]) {
			end++
		}
		if isPath(runes[start:end]) {
			if out == nil {
				out = append([]rune(nil), runes...)
			}
			for i := start; i < end; i++ {
				out[i] = pathBreakRune(runes[i])
			}
		}
		start = end + 1
	}
	if out == nil {
		return runes
	}
	return out
}

// isPath reports whether a run of text holds a separator between other
// letters, as a path does.
func isPath(run []rune) bool {
	for i, r := range run {
		if isSeparator(r) && i > 0 && i < len(run)-1 {
			return true
		}
	}
	return false
}

// isSeparator reports whether r separates the parts of a path.
func isSeparator(r rune) bool { return r == '/' || r == '\\' }

// pathBreakRune is what the line breaker reads for r in a path: a slash
// for a separator, which lines break after, and a letter for other
// punctuation, which they do not break around.
func pathBreakRune(r rune) rune {
	switch {
	case isSeparator(r):
		return '/'
	case unicode.IsPunct(r) || unicode.IsSymbol(r):
		return 'a'
	}
	return r
}
