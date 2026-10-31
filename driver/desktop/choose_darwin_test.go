package desktop

import (
	"slices"
	"testing"

	"github.com/marrasen/gunim/driver"
)

func TestThePanelShowsEveryFiltersExtensions(t *testing.T) {
	for _, c := range []struct {
		filters []driver.FileFilter
		want    []string
	}{
		{nil, nil},
		{[]driver.FileFilter{{Name: "Songs", Patterns: []string{"*.hej"}}, {Name: "MIDI", Patterns: []string{"*.mid", "*.midi"}}}, []string{"hej", "mid", "midi"}},
		// A filter of every file leaves the panel showing every file.
		{[]driver.FileFilter{{Name: "Songs", Patterns: []string{"*.hej"}}, {Name: "Everything", Patterns: []string{"*"}}}, nil},
		{[]driver.FileFilter{{Name: "Odd", Patterns: []string{"take?.wav"}}}, nil},
	} {
		if got := extensions(c.filters); !slices.Equal(got, c.want) {
			t.Errorf("extensions(%v) = %v, want %v", c.filters, got, c.want)
		}
	}
}
