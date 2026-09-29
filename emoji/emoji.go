// Package emoji lists the emoji, in the groups a picker shows them in, with their names for searching.
//
// The data comes from Unicode's emoji-test.txt, through gen.go. It holds each emoji once, without its skin tones.
package emoji

//go:generate go run gen.go emoji-test.txt

import (
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

// Search returns the emoji whose names hold every word of query, in Unicode's order, those whose name starts
// with the query's first word first. An empty query finds none.
func Search(query string) []Emoji {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return nil
	}
	var first, rest []Emoji
	for _, g := range groups {
		for _, e := range g.Emoji {
			if !matches(e.Name, words) {
				continue
			}
			if strings.HasPrefix(e.Name, words[0]) {
				first = append(first, e)
			} else {
				rest = append(rest, e)
			}
		}
	}
	return append(first, rest...)
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
