// Package emoji lists the emoji, in the groups a picker shows them in, with their names for searching.
//
// The data comes from Unicode's emoji-test.txt, through gen.go. It holds each emoji once, without its skin tones.
package emoji

//go:generate go run gen.go emoji-test.txt

import (
	"slices"
	"strings"
	"sync"
)

// Emoji is one emoji: its text, its short name, and the version of Emoji it arrived in, such as "15.0".
type Emoji struct {
	Text, Name, Since string
}

// Group is a group of emoji, such as "Smileys & Emotion".
type Group struct {
	Name  string
	Emoji []Emoji
}

// Groups returns the emoji in their groups, in Unicode's order. The slices are shared: do not change them.
func Groups() []Group { return groups }

// Search returns the emoji whose names hold every word of query, each at the start of one of the name's words,
// best first: a name that is the query, then one with the query's first word as a whole word of its own, as "red
// heart" for "heart", then one that starts with it, then the rest; the first two shortest name first, as closest to
// what was typed, and each otherwise in Unicode's order. An empty query finds none.
func Search(query string) []Emoji {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	q := strings.Join(words, " ")
	var tiers [4][]Emoji
	for _, g := range groups {
		for _, e := range g.Emoji {
			name := strings.ToLower(e.Name)
			if !matches(name, words) {
				continue
			}
			tier := 3
			switch {
			case name == q:
				tier = 0
			case hasWord(name, words[0]):
				tier = 1
			case strings.HasPrefix(name, words[0]):
				tier = 2
			}
			tiers[tier] = append(tiers[tier], e)
		}
	}
	for _, t := range tiers[1:3] {
		slices.SortStableFunc(t, func(a, b Emoji) int { return len([]rune(a.Name)) - len([]rune(b.Name)) })
	}
	return append(append(append(tiers[0], tiers[1]...), tiers[2]...), tiers[3]...)
}

// hasWord reports whether w is a whole word of name.
func hasWord(name, w string) bool {
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '-' || r == ':' }) {
		if part == w {
			return true
		}
	}
	return false
}

// matches reports whether name holds every word, each at the start of one of its words.
func matches(name string, words []string) bool {
	for _, w := range words {
		found := false
		for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '-' || r == ':' }) {
			if strings.HasPrefix(part, w) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

var (
	byText     map[string]Emoji
	byTextOnce sync.Once
)

// Lookup returns the emoji whose text is s, with its name, and false for text that is not one of them.
func Lookup(s string) (Emoji, bool) {
	byTextOnce.Do(func() {
		byText = map[string]Emoji{}
		for _, g := range groups {
			for _, e := range g.Emoji {
				byText[e.Text] = e
			}
		}
	})
	e, ok := byText[s]
	return e, ok
}
