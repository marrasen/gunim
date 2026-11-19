package text

import (
	"strconv"
	"testing"
)

// benchText is a label's worth of prose, set again and again as a widget
// sets its text on every frame.
const benchText = "Keep the copies on the disk for 30 days, then remove the oldest first when the disk is nearly full."

func BenchmarkLayoutSameText(b *testing.B) {
	f := Default()
	for b.Loop() {
		f.Layout(benchText, Style{Size: 15}, 240)
	}
}

func BenchmarkShapeSameText(b *testing.B) {
	f := Default()
	for b.Loop() {
		f.Shape(benchText, 15)
	}
}

func BenchmarkLayoutSpansSameText(b *testing.B) {
	spans := []Span{
		{Text: "Keep the copies for ", Face: Default(), Size: 15},
		{Text: "30 days", Face: GoSans(true, false), Size: 15},
		{Text: ", then remove the oldest first when the disk is nearly full.", Face: Default(), Size: 15},
	}
	for b.Loop() {
		LayoutSpans(spans, Style{}, 240)
	}
}

// BenchmarkLayoutSameTextParallel sets the same text from every core at
// once, as several windows do.
func BenchmarkLayoutSameTextParallel(b *testing.B) {
	f := Default()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			f.Layout(benchText, Style{Size: 15}, 240)
		}
	})
}

// BenchmarkLayoutNewText sets a text never set before on every call: the
// cost of a layout the cache cannot answer.
func BenchmarkLayoutNewText(b *testing.B) {
	f := Default()
	i := 0
	for b.Loop() {
		i++
		f.Layout(benchText+strconv.Itoa(i), Style{Size: 15}, 240)
	}
}
