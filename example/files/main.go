package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

func main() {
	dir := flag.String("dir", "", "the folder to open; the home folder when empty")
	prefsPath := flag.String("prefs", "", "the settings file; gunim-files/prefs.json in the user's configuration folder when empty")
	demo := flag.Bool("demo", false, "open a folder of sample files made in a temporary folder, with the settings kept there too")
	shot := flag.String("shot", "", "write the window to this PNG file after -after, and quit; implies -demo without -dir")
	after := flag.Duration("after", 2*time.Second, "how long -shot waits")
	pick := flag.String("select", "", "the name to select once the folder is read")
	keys := flag.String("do", "", "steps to run once the folder is read, split by commas, such as copy,into:Documents,paste")
	big := flag.Bool("big", false, "put a 2 GB file in the demo folder, to watch a long copy")
	runFor := flag.Duration("for", 0, "quit after this long; zero runs until the window closes")
	flag.Parse()
	o := options{dir: *dir, prefsPath: *prefsPath, pick: *pick, do: *keys}
	if err := start(o, *demo || *shot != "" && o.dir == "", *big, *shot, *after, *runFor); err != nil {
		log.Fatal(err)
	}
}

// start runs the window, on a demo folder made for the purpose with demo,
// which it removes once the window closes.
func start(o options, demo, big bool, shot string, after, runFor time.Duration) (err error) {
	if demo {
		root, merr := os.MkdirTemp("", "gunim-files-demo-")
		if merr != nil {
			return merr
		}
		defer func() { err = errors.Join(err, os.RemoveAll(root)) }()
		if o.dir, err = makeDemo(root); err != nil {
			return err
		}
		if big {
			if err := makeBig(filepath.Join(o.dir, "Big video.mp4"), 2<<30); err != nil {
				return err
			}
		}
		if o.prefsPath == "" {
			o.prefsPath = filepath.Join(root, "prefs.json")
		}
	}
	return run(o, shot, after, runFor)
}

func run(o options, shot string, after, runFor time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runFor)
		defer cancel()
	}
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{
			Title:      "Files",
			Size:       geom.Sz(1180, 740),
			Root:       &root{},
			Chromeless: true,
			Arrive:     true,
			ZoomKeys:   true,
			Icons:      icons(),
			AskToClose: CloseAsked{},
		})
		if err != nil {
			return fmt.Errorf("files: %w", err)
		}
		registerViews(w)
		c := w.Client()
		if shot != "" {
			go func() {
				select {
				case <-time.After(after):
				case <-ctx.Done():
					return
				}
				if err := writeShot(ctx, c, shot); err != nil {
					log.Print(err)
				}
				c.Close()
			}()
		}
		return serve(ctx, c, o)
	})
	if errors.Is(err, driver.ErrNoDriver) {
		log.Print("gunim has no driver for this operating system yet")
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// writeShot writes what the window shows to a PNG file.
func writeShot(ctx context.Context, c gunim.Client, path string) error {
	img, err := c.Shot(ctx)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(png.Encode(f, img), f.Close())
}
