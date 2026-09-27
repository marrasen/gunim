package main

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/paint"
)

// Fact is one line of what the preview pane says about an item.
type Fact struct {
	Label, Value string
}

// Preview is what the preview pane shows: one item, several, or the
// folder itself. Seq changes with each new subject.
type Preview struct {
	Seq   int
	Title string
	Type  string
	// Path is the item's path, for the Reveal and Copy path links; empty
	// for several items.
	Path  string
	Tint  Tint
	Facts []Fact
	// Image is a picture's thumbnail, and Text the start of a text file,
	// Cut set when there is more.
	Image *paint.Image
	Text  string
	Cut   bool
	// Counting is set while a folder's contents are still being counted.
	Counting bool
	// Err says why the preview is incomplete.
	Err string
}

// Counted is a patch: how much a folder holds, so far or in all.
type Counted struct {
	Seq      int
	Items    string
	Size     string
	Counting bool
	Err      string
}

// RevealPath asks to show a path in the system's file manager.
type RevealPath struct {
	Path string
}

func init() {
	gunim.RegisterType[Preview]("files.preview")
	gunim.RegisterType[Counted]("files.counted")
	gunim.RegisterType[RevealPath]("files.reveal")
}
