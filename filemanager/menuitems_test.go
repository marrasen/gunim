package filemanager

import "github.com/marrasen/gunim/widget"

// labelsOf returns each item's label, in order.
func labelsOf(items []widget.MenuItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Label
	}
	return out
}

// disabledOf returns whether each item is disabled, in order.
func disabledOf(items []widget.MenuItem) []bool {
	out := make([]bool, len(items))
	for i, it := range items {
		out[i] = it.Disabled
	}
	return out
}

// breaksOf returns the items a line goes above.
func breaksOf(items []widget.MenuItem) []int {
	var out []int
	for i, it := range items {
		if it.Break {
			out = append(out, i)
		}
	}
	return out
}
