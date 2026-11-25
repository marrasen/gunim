// Command filesout hands files to other applications, with none on the disk: it opens a text it makes up as it is read in
// another application, or shares it. It also saves it, to a file or in a folder the user chooses. It writes slowly, so
// that a run on a phone shows what comes of a file that is left half way: the next run takes it away.
//
//	go run ./tools/gunimapk -run -name "Files out" ./example/filesout
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// doIntent is a button clicked, by its label.
type doIntent struct{ what string }

// The buttons.
const (
	open   = "Open in another app"
	share  = "Share"
	stop   = "Stop serving"
	save   = "Save"
	folder = "Save in a folder"
)

// size is how long the text is, in lines of lineLen bytes.
const (
	lineLen = 16
	size    = 4 << 20
)

// text is a text of numbered lines, made up as it is read.
type text struct {
	// read counts the bytes read of it.
	read atomic.Int64
}

// ReadAt implements [io.ReaderAt].
func (t *text) ReadAt(p []byte, off int64) (int, error) {
	n := 0
	for n < len(p) && off+int64(n) < size {
		at := off + int64(n)
		line := fmt.Sprintf("%0*d\n", lineLen-1, at/lineLen)
		n += copy(p[n:], line[at%lineLen:])
	}
	t.read.Add(int64(n))
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	err := gunim.Main(ctx, func(a *gunim.App) error {
		left := takeAwayLeft(a)
		w, err := a.NewWindow(gunim.WindowOptions{Title: "gunim files out", Size: geom.Sz(420, 640), Root: widget.NewSurface()})
		if err != nil {
			return err
		}
		gunim.RegisterView(w, "out", func(string) *out { return newOut() }, func(v *out, s string, _ *gunim.UI) { v.text.Text = s })
		c := w.Client()
		if err := c.Mount(gunim.Root, "out", "out", left); err != nil {
			return err
		}
		var mu sync.Mutex
		lines := left
		tell := func(format string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			s := fmt.Sprintf(format, args...)
			log.Print(s)
			lines = s + "\n" + lines
			_ = c.Update("out", lines)
		}
		data := &text{}
		serve := func() *driver.Served {
			return &driver.Served{Name: "numbers.txt", Size: size, Data: data, OnUse: func(open int) {
				tell("Open %d times, %d bytes read.", open, data.read.Load())
			}}
		}
		served := serve()
		for e := range c.Intents() {
			in, ok := e.Intent.(doIntent)
			if !ok {
				continue
			}
			switch in.what {
			case open:
				tell("Opened: %v", c.OpenServed(served))
			case share:
				tell("Shared: %v", c.Share(driver.Share{Served: []*driver.Served{served}}))
			case stop:
				served.Stop()
				served = serve()
				tell("Stopped.")
			case save:
				go func() {
					f, err := c.SaveStream(ctx, driver.SaveOptions{Name: "numbers.txt"})
					if err != nil || f == nil {
						tell("Not saved: %v", err)
						return
					}
					tell("Saves %s, made %v.", f.Name(), f.Made())
					err = write(f, data)
					tell("Saved %s: %v", f.Name(), err)
				}()
			case folder:
				go func() { tell("Saved in the folder: %v", saveIn(ctx, c, data, tell)) }()
			}
		}
		return c.Err()
	})
	if errors.Is(err, driver.ErrNoDriver) {
		log.Print("gunim has no driver for this operating system yet")
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// saveIn saves three files in a folder the user chooses.
func saveIn(ctx context.Context, c gunim.Client, data io.ReaderAt, tell func(string, ...any)) error {
	fo, err := c.SaveFolder(ctx, driver.ChooseOptions{})
	if err != nil || fo == nil {
		return err
	}
	defer func() { _ = fo.Close() }()
	for _, name := range []string{"numbers.txt", "deep/er/numbers.txt", "deep/more.jpg"} {
		f, err := fo.Create(name)
		if err != nil {
			tell("%s: %v", name, err)
			continue
		}
		tell("Saves %s as %s.", name, f.Name())
		if err := write(f, data); err != nil {
			return err
		}
		tell("Saved %s.", f.Name())
	}
	return nil
}

// write writes the text to f, over some seconds, and takes f away where it can't. The place is kept meanwhile, for the
// next run.
func write(f driver.SavedFile, data io.ReaderAt) error {
	if err := keepPlace(f.Place()); err != nil {
		return errors.Join(err, f.Remove())
	}
	b := make([]byte, 64<<10)
	for off := int64(0); off < size; off += int64(len(b)) {
		n, err := data.ReadAt(b, off)
		if err == nil || errors.Is(err, io.EOF) {
			_, err = f.Write(b[:n])
		}
		if err != nil {
			return errors.Join(err, f.Remove())
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := f.Close(); err != nil {
		return errors.Join(err, f.Remove())
	}
	return keepPlace("")
}

// placeFile is where the place of the file being written is kept.
func placeFile() (string, error) {
	d, err := os.UserConfigDir()
	return filepath.Join(d, "gunim-filesout", "place"), err
}

// keepPlace keeps place, or forgets it for "".
func keepPlace(place string) error {
	p, err := placeFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(place), 0o600)
}

// takeAwayLeft takes away what an earlier run left half written, and says so.
func takeAwayLeft(a *gunim.App) string {
	p, err := placeFile()
	if err != nil {
		return ""
	}
	place, err := os.ReadFile(p)
	if err != nil || len(place) == 0 {
		return ""
	}
	err = a.RemoveSaved(string(place))
	s := fmt.Sprintf("A file was left half way. Taken away: %v", err)
	log.Print(s)
	_ = keepPlace("")
	return s
}

// out is buttons over what came of them.
type out struct {
	buttons []gunim.Node
	text    *widget.Label
}

func newOut() *out {
	v := &out{text: widget.NewLabel("")}
	for _, what := range []string{open, share, stop, save, folder} {
		b := widget.NewButton(what)
		b.OnClick = widget.Sends(doIntent{what})
		v.buttons = append(v.buttons, b)
	}
	return v
}

// Children implements [gunim.Composite].
func (v *out) Children() []gunim.Node { return append(append([]gunim.Node{}, v.buttons...), v.text) }

// Layout implements [gunim.Node]: the buttons on top and the text filling the rest. The surface under the view keeps it
// clear of the system's bars.
func (v *out) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	box := c.Max
	const margin, buttonH = 12, 48
	w := box.W - 2*margin
	y := float32(margin)
	for i := range v.buttons {
		kids.At(i).Layout(gunim.Tight(geom.Sz(w, buttonH)))
		kids.At(i).Place(geom.Pt(margin, y))
		y += buttonH + margin
	}
	last := kids.At(len(v.buttons))
	last.Layout(gunim.Tight(geom.Sz(w, max(box.H-y-margin, 0))))
	last.Place(geom.Pt(margin, y))
	return box
}

// Paint implements [gunim.Node].
func (v *out) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for kid := range kids.All {
		kid.Paint(p)
	}
}
