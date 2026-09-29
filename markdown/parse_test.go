package markdown

import (
	"reflect"
	"testing"
)

func TestParseReadsTheBlocksAChatUses(t *testing.T) {
	src := "# Plan\n\nShip **v5.3** with `thumbs.go`, *soon*.\nSecond line ~~maybe~~.\n\n" +
		"> quoted\n\n- [ ] changelog\n- [x] tag\n\n1. one\n2. two\n\n```\ngo test ./...\n```\n\n---\n\nsee https://example.com/x"
	got := parse(src, true)
	want := []block{
		{kind: heading, level: 1, spans: []span{{text: "Plan"}}},
		{kind: paragraph, spans: []span{
			{text: "Ship "}, {text: "v5.3", style: bold}, {text: " with "}, {text: "thumbs.go", style: mono},
			{text: ", "}, {text: "soon", style: italic}, {text: ".\nSecond line "}, {text: "maybe", style: strike},
			{text: "."}}},
		{kind: quote, kids: []block{{kind: paragraph, spans: []span{{text: "quoted"}}}}},
		{kind: list, start: 0, items: []item{
			{task: openTask, blocks: []block{{kind: paragraph, spans: []span{{text: "changelog"}}}}},
			{task: doneTask, blocks: []block{{kind: paragraph, spans: []span{{text: "tag"}}}}},
		}},
		{kind: list, ordered: true, start: 1, items: []item{
			{blocks: []block{{kind: paragraph, spans: []span{{text: "one"}}}}},
			{blocks: []block{{kind: paragraph, spans: []span{{text: "two"}}}}},
		}},
		{kind: code, text: "go test ./..."},
		{kind: rule},
		{kind: paragraph, spans: []span{{text: "see "},
			{text: "https://example.com/x", url: "https://example.com/x"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parse gave\n%+v\nwant\n%+v", got, want)
	}
}

func TestSoftBreaksJoinWithoutBreaks(t *testing.T) {
	got := parse("one\ntwo", false)
	if len(got) != 1 || len(got[0].spans) != 1 || got[0].spans[0].text != "one two" {
		t.Fatalf("parse gave %+v, want one line", got)
	}
}

func TestPlainDropsTheMarkup(t *testing.T) {
	if got, want := Plain("**Ship** it:\n- [ ] `tag`\n> ok"), "Ship it:\ntag\nok"; got != want {
		t.Fatalf("Plain = %q, want %q", got, want)
	}
}
