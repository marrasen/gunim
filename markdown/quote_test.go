package markdown

import (
	"reflect"
	"testing"
)

func TestAChatQuoteHoldsOnlyItsOwnLines(t *testing.T) {
	got := parse("> the export is slow\nIs that the CSV one?", true)
	want := []block{
		{kind: quote, kids: []block{{kind: paragraph, spans: []span{{text: "the export is slow"}}}}},
		{kind: paragraph, spans: []span{{text: "Is that the CSV one?"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parse gave\n%+v\nwant\n%+v", got, want)
	}
}

func TestEndQuotesLeavesCodeAlone(t *testing.T) {
	src := "```\n> not a quote\ntext\n```"
	if got := endQuotes(src); got != src {
		t.Fatalf("endQuotes changed a code block:\n%q", got)
	}
}
