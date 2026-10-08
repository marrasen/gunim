// Command themeedit shows the theme editor beside a few widgets and a
// grid of cells whose cursor moves along a line, as a terminal's does.
// Every edit shows at once on the widgets and the grid. The Chosen tab
// picks out the cursor, which glides or jumps, the accent colour and
// the gap between things; All values lists every token.
//
// With -file, the editor starts from the values in that file, saves
// each edit to it, and Import and Export read and write it.
//
//	CGO_ENABLED=0 go run ./example/themeedit
//	CGO_ENABLED=0 go run ./example/themeedit -file mytheme.json
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/themeedit"
	"github.com/marrasen/gunim/widget"
)

// The vocabulary the two halves share.
type (
	// Page is the state the page renders: the values saved, as JSON.
	Page struct{ Saved []byte }
	// Saved travels as an edit ends, with the values changed as JSON.
	Saved struct{ Data []byte }
	// ImportAsked travels when the user presses Import.
	ImportAsked struct{}
	// Imported is the patch that hands the editor the file's values.
	Imported struct{ Data []byte }
)

func init() {
	gunim.RegisterType[Page]("themeedit.page")
	gunim.RegisterType[Saved]("themeedit.saved")
	gunim.RegisterType[ImportAsked]("themeedit.import")
	gunim.RegisterType[Imported]("themeedit.imported")
}

func main() {
	runFor := flag.Duration("for", 0, "quit after this long; zero runs until the window closes")
	file := flag.String("file", "", "a file to read the theme's values from and save them to")
	flag.Parse()
	if err := run(*runFor, *file); err != nil {
		log.Fatal(err)
	}
}

func run(runFor time.Duration, file string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runFor)
		defer cancel()
	}
	return gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{
			Title: "gunim theme editor",
			Size:  geom.Sz(1100, 640),
			Root:  widget.NewSurface(),
		})
		if err != nil {
			return err
		}
		w.RegisterTheme(widget.Dark())
		w.RegisterTheme(widget.Light())
		gunim.RegisterView(w, "page", buildPage, nil)
		gunim.RegisterPatch(w, "page", func(p *page, im Imported, u *gunim.UI) {
			if err := p.editor.Import(im.Data, u); err != nil {
				log.Print(err)
			}
		})
		return serve(ctx, w.Client(), file)
	})
}

// page is the editor beside the widgets it themes.
type page struct {
	*widget.Pad
	editor *themeedit.Editor
	typist *typist
}

// sections are what the Chosen tab picks out.
var sections = []themeedit.Section{
	{Title: "Terminal", Fields: []themeedit.Field{{
		Key:    widget.Caret.Key(),
		Label:  "Cursor",
		Detail: "How the cursor moves to the next place on its line.",
		Presets: []themeedit.Preset{
			{Label: "Glides", Value: widget.Caret.Default()},
			{Label: "Jumps", Value: themeedit.Instant},
		},
	}}},
	{Title: "Look", Fields: []themeedit.Field{
		{Key: widget.Accent.Key(), Label: "Accent", Detail: "The colour of focus, selection and the chosen tab."},
		{Key: widget.Gap.Key(), Label: "Gap", Detail: "The room between things side by side.", Min: 0, Max: 32},
		{Key: widget.Quick.Key(), Label: "Feedback", Detail: "How hover, press and focus move."},
	}},
}

func buildPage(s Page) *page {
	// The editor starts from the dark theme, with what was saved over it.
	over, err := theme.UnmarshalValues(theme.Make("mine"), s.Saved)
	if err != nil && len(s.Saved) > 0 {
		log.Print(err)
	}
	ed := themeedit.New(themeedit.Options{
		Base:      widget.Dark(),
		Overrides: over,
		Sections:  sections,
		OnChange: func(th theme.Theme, u *gunim.UI) gunim.Intent {
			u.UseTheme(th)
			return nil
		},
		OnCommit: func(over theme.Theme, u *gunim.UI) gunim.Intent {
			data, err := theme.MarshalValues(over)
			if err != nil {
				return nil
			}
			return Saved{Data: data}
		},
		OnExport: func(data []byte, u *gunim.UI) gunim.Intent { return Saved{Data: data} },
		OnImport: widget.Sends(ImportAsked{}),
	})

	// The base theme, which the edits lie over.
	base := widget.NewSegmented("Dark", "Light")
	base.Tooltip = "The theme the edits lie over"
	base.OnChange = func(i int, u *gunim.UI) gunim.Intent {
		ed.SetBase([]theme.Theme{widget.Dark(), widget.Light()}[i], u)
		u.UseTheme(ed.Theme())
		return nil
	}

	button := widget.NewButton("A button")
	primary := widget.NewButton("Primary")
	primary.Kind = widget.ButtonPrimary
	sw := widget.NewSwitch("A switch")
	sw.SetChecked(true, nil)
	slider := widget.NewSlider(0, 100)
	slider.Label = "A slider"
	slider.SetValue(40, nil)
	drop := widget.NewDropdown(widget.Labels("Apple", "Banana", "Cherry"))
	drop.Label = "Fruit"
	field := widget.NewTextField()
	field.Placeholder = "Type here"
	buttons := widget.Row(button, primary)

	ty := newTypist()
	demo := widget.Column(widget.Row(widget.NewLabel("Base"), base), buttons, sw, slider, drop, field, ty)
	demo.Cross = widget.CrossStretch
	side := widget.NewCard(demo)

	row := widget.Row(ed, side).Grow(ed, 1)
	row.Cross = widget.CrossStretch
	return &page{Pad: widget.NewPad(row), editor: ed, typist: ty}
}

// typist is a grid of cells with a cursor that moves along a line as if
// someone typed, a cell every quarter second, and wraps to the next line
// at the end.
type typist struct {
	grid    *widget.CellGrid
	col     int
	row     int
	elapsed time.Duration
}

// typistCols and typistRows are the grid's size, in cells.
const (
	typistCols = 28
	typistRows = 4
)

func newTypist() *typist {
	g := widget.NewCellGrid()
	g.Resize(typistCols, typistRows)
	for y, line := range []string{"$ make the cursor jump", "$ open All values", "$ search motion", "$ "} {
		cells := make([]widget.Cell, 0, len(line))
		for _, r := range line {
			cells = append(cells, widget.Cell{Rune: r})
		}
		g.SetRow(y, cells)
	}
	t := &typist{grid: g, col: 2}
	t.place()
	return t
}

// place puts the grid's cursor where the typist is.
func (t *typist) place() {
	t.grid.SetCursor(widget.Cursor{Col: t.col, Row: t.row, Visible: true})
}

// Children implements [gunim.Composite].
func (t *typist) Children() []gunim.Node { return []gunim.Node{t.grid} }

// Step implements [gunim.Animator]: every quarter second the cursor
// moves on a cell.
func (t *typist) Step(dt time.Duration) bool {
	t.elapsed += dt
	if t.elapsed < 250*time.Millisecond {
		return false
	}
	t.elapsed = 0
	t.col++
	if t.col >= typistCols {
		t.col, t.row = 2, (t.row+1)%typistRows
	}
	t.place()
	return true
}

// WakeIn implements [gunim.Waker].
func (t *typist) WakeIn() time.Duration { return max(time.Millisecond, 250*time.Millisecond-t.elapsed) }

// Layout implements [gunim.Node].
func (t *typist) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	s := kids.At(0).Layout(gunim.Constraints{Max: geom.Sz(c.Max.W, 0)})
	kids.At(0).Place(geom.Point{})
	return c.Constrain(s)
}

// Paint implements [gunim.Node].
func (t *typist) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

func serve(ctx context.Context, c gunim.Client, file string) error {
	var saved []byte
	if file != "" {
		data, err := os.ReadFile(file)
		switch {
		case err == nil:
			saved = data
		case !errors.Is(err, fs.ErrNotExist):
			return err
		}
	}
	if err := c.Mount(gunim.Root, "page", "page", Page{Saved: saved}); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-c.Intents():
			if !ok {
				return c.Err()
			}
			switch v := ev.Intent.(type) {
			case Saved:
				if file == "" {
					continue
				}
				if err := os.WriteFile(file, v.Data, 0o600); err != nil {
					log.Print(err)
				}
			case ImportAsked:
				if file == "" {
					log.Print("Import reads the file -file names; none was given")
					continue
				}
				data, err := os.ReadFile(file)
				if err != nil {
					log.Print(err)
					continue
				}
				if err := c.Patch("page", Imported{Data: data}); err != nil {
					return fmt.Errorf("importing: %w", err)
				}
			}
		}
	}
}
