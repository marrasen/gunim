// Package markdown shows Markdown as gunim text: paragraphs, emphasis, code, quotes, lists with tasks, headings
// and links, which the reader can select and copy.
package markdown

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	gtext "github.com/yuin/goldmark/text"
)

// kind is what a block is.
type kind uint8

const (
	paragraph kind = iota
	heading
	code
	quote
	list
	rule
	table
)

// block is one block of a document.
type block struct {
	kind kind
	// level is a heading's level, from 1.
	level int
	// spans are a paragraph's or a heading's text.
	spans []span
	// text is a code block's text.
	text string
	// kids are a quote's blocks.
	kids []block
	// items are a list's items, numbered from start when ordered.
	items   []item
	ordered bool
	start   int
	// rows are a table's cells, the header first, and aligns how each column sets its text.
	rows   [][][]span
	aligns []align
}

// align is how a table's column sets its text.
type align uint8

const (
	alignStart align = iota
	alignCenter
	alignEnd
)

// item is one item of a list: its blocks, and whether it is a task and done.
type item struct {
	blocks []block
	task   task
}

type task uint8

const (
	noTask task = iota
	openTask
	doneTask
)

// span is a piece of text in one style, and a link's address.
type span struct {
	text  string
	style style
	url   string
}

// style is how a span is set.
type style uint8

const (
	bold style = 1 << iota
	italic
	mono
	strike
)

// markdown parses with the extensions a chat wants: strikethrough, task lists, bare addresses as links, and tables.
var markdown = goldmark.New(goldmark.WithExtensions(extension.Strikethrough, extension.TaskList, extension.Linkify,
	extension.Table))

// parse returns src's blocks. With breaks set, it reads src as a chat does: a single line break in a paragraph
// breaks the line, where CommonMark would join the lines, and a quote holds only the lines that start with >.
func parse(src string, breaks bool) []block {
	if breaks {
		src = endQuotes(src)
	}
	b := []byte(src)
	doc := markdown.Parser().Parse(gtext.NewReader(b))
	p := parser{src: b, breaks: breaks}
	return p.blocks(doc)
}

// endQuotes puts a blank line after each quote, before the first line after it that does not start with >, so
// that line is not part of the quote. Code blocks stay as they are.
func endQuotes(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	fence, quoted := "", false
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		switch {
		case fence != "":
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
		case strings.HasPrefix(trimmed, "```"), strings.HasPrefix(trimmed, "~~~"):
			fence = trimmed[:3]
		case quoted && trimmed != "" && !strings.HasPrefix(trimmed, ">"):
			out = append(out, "")
		}
		out = append(out, line)
		quoted = fence == "" && strings.HasPrefix(trimmed, ">")
	}
	return strings.Join(out, "\n")
}

type parser struct {
	src    []byte
	breaks bool
}

// blocks returns the blocks among n's children.
func (p parser) blocks(n ast.Node) []block {
	var out []block
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			if spans := p.inlines(c, 0, ""); len(spans) > 0 {
				out = append(out, block{kind: paragraph, spans: spans})
			}
		case *ast.Heading:
			out = append(out, block{kind: heading, level: c.Level, spans: p.inlines(c, 0, "")})
		case *ast.FencedCodeBlock:
			out = append(out, block{kind: code, text: p.lines(c)})
		case *ast.CodeBlock:
			out = append(out, block{kind: code, text: p.lines(c)})
		case *ast.HTMLBlock:
			out = append(out, block{kind: paragraph, spans: []span{{text: p.lines(c)}}})
		case *ast.Blockquote:
			out = append(out, block{kind: quote, kids: p.blocks(c)})
		case *ast.List:
			l := block{kind: list, ordered: c.IsOrdered(), start: c.Start}
			for it := c.FirstChild(); it != nil; it = it.NextSibling() {
				l.items = append(l.items, p.item(it))
			}
			out = append(out, l)
		case *ast.ThematicBreak:
			out = append(out, block{kind: rule})
		case *east.Table:
			out = append(out, p.table(c))
		}
	}
	return out
}

// table returns a table's rows of cells and its columns' alignments.
func (p parser) table(n *east.Table) block {
	t := block{kind: table}
	for _, a := range n.Alignments {
		switch a {
		case east.AlignCenter:
			t.aligns = append(t.aligns, alignCenter)
		case east.AlignRight:
			t.aligns = append(t.aligns, alignEnd)
		default:
			t.aligns = append(t.aligns, alignStart)
		}
	}
	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		var cells [][]span
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cells = append(cells, p.inlines(cell, 0, ""))
		}
		t.rows = append(t.rows, cells)
	}
	return t
}

// item returns a list item's blocks, taking a task's box off the front of its first paragraph.
func (p parser) item(n ast.Node) item {
	it := item{}
	if first := n.FirstChild(); first != nil {
		if box, ok := first.FirstChild().(*east.TaskCheckBox); ok {
			it.task = openTask
			if box.IsChecked {
				it.task = doneTask
			}
		}
	}
	it.blocks = p.blocks(n)
	if it.task != noTask && len(it.blocks) > 0 && len(it.blocks[0].spans) > 0 {
		s := &it.blocks[0].spans[0]
		s.text = strings.TrimLeft(s.text, " ")
	}
	return it
}

// lines returns a block's lines as one text, without the break after the last.
func (p parser) lines(n ast.Node) string {
	var b strings.Builder
	lines := n.Lines()
	for i := range lines.Len() {
		seg := lines.At(i)
		b.Write(seg.Value(p.src))
	}
	return strings.TrimRight(b.String(), "\n")
}

// inlines returns the spans of n's inline children, in style st and linking to url.
func (p parser) inlines(n ast.Node, st style, url string) []span {
	var out []span
	add := func(text string, st style, url string) {
		if text == "" {
			return
		}
		if k := len(out) - 1; k >= 0 && out[k].style == st && out[k].url == url {
			out[k].text += text
			return
		}
		out = append(out, span{text: text, style: st, url: url})
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			add(string(c.Segment.Value(p.src)), st, url)
			switch {
			case c.HardLineBreak(), c.SoftLineBreak() && p.breaks:
				add("\n", st, url)
			case c.SoftLineBreak():
				add(" ", st, url)
			}
		case *ast.String:
			add(string(c.Value), st, url)
		case *ast.CodeSpan:
			for _, s := range p.inlines(c, st|mono, url) {
				add(s.text, s.style, s.url)
			}
		case *ast.Emphasis:
			with := italic
			if c.Level >= 2 {
				with = bold
			}
			for _, s := range p.inlines(c, st|with, url) {
				add(s.text, s.style, s.url)
			}
		case *east.Strikethrough:
			for _, s := range p.inlines(c, st|strike, url) {
				add(s.text, s.style, s.url)
			}
		case *ast.Link:
			for _, s := range p.inlines(c, st, string(c.Destination)) {
				add(s.text, s.style, s.url)
			}
		case *ast.AutoLink:
			u := string(c.URL(p.src))
			label := string(c.Label(p.src))
			if c.AutoLinkType == ast.AutoLinkEmail && !strings.HasPrefix(u, "mailto:") {
				u = "mailto:" + u
			}
			add(label, st, u)
		case *ast.Image:
			// A picture shows as the words that describe it.
			for _, s := range p.inlines(c, st|italic, url) {
				add(s.text, s.style, s.url)
			}
		case *ast.RawHTML:
			segs := c.Segments
			for i := range segs.Len() {
				seg := segs.At(i)
				add(string(seg.Value(p.src)), st, url)
			}
		case *east.TaskCheckBox:
			// A task's box shows beside its item.
		default:
			for _, s := range p.inlines(c, st, url) {
				add(s.text, s.style, s.url)
			}
		}
	}
	return out
}

// Plain returns src's text without its Markdown: what a reader sees, with each block on a line of its own. It
// suits a preview, such as a quote of a message.
func Plain(src string) string {
	var b strings.Builder
	var walk func([]block)
	walk = func(bs []block) {
		for _, bl := range bs {
			switch bl.kind {
			case paragraph, heading:
				for _, s := range bl.spans {
					b.WriteString(s.text)
				}
				b.WriteByte('\n')
			case code:
				b.WriteString(bl.text)
				b.WriteByte('\n')
			case quote:
				walk(bl.kids)
			case list:
				for _, it := range bl.items {
					walk(it.blocks)
				}
			case table:
				for _, row := range bl.rows {
					for i, cell := range row {
						if i > 0 {
							b.WriteByte('\t')
						}
						for _, s := range cell {
							b.WriteString(s.text)
						}
					}
					b.WriteByte('\n')
				}
			case rule:
			}
		}
	}
	walk(parse(src, true))
	return strings.TrimRight(b.String(), "\n")
}
