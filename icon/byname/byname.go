// Package byname looks Lucide icons up by name. Importing it links every icon into the program.
package byname

import (
	"slices"

	"github.com/marrasen/gunim/icon"
)

// Lookup returns the icon Lucide calls name, such as "funnel", or by an older name, such as "filter".
func Lookup(name string) (*icon.Icon, bool) {
	ic, ok := icons[name]
	return ic, ok
}

// Names returns every name Lookup knows, sorted.
func Names() []string {
	out := make([]string, 0, len(icons))
	for name := range icons {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}
