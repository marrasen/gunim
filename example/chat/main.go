// Command chat is a chat client with a pretend server behind it, for trying gunim's widgets in a chat.
//
// A rail of projects runs down the left, then the project's conversations, then the open conversation: a timeline
// that opens at its latest message and stays there as messages arrive, and a message box under it. Enter sends and
// Shift+Enter starts a line. Colleagues type, send, reply, edit and withdraw. The button in the header drops the
// connection: messages sent while offline wait, and go when it is back. Some sends fail, and a click on the red note
// beside one sends it again. Under the pointer a message shows a toolbar to react and reply, and for your own to
// edit or withdraw it. A button in the header opens the conversation in a window of its own.
//
//	CGO_ENABLED=0 go run ./example/chat
//	CGO_ENABLED=0 go run ./example/chat -history 50000
//	CGO_ENABLED=0 go run ./example/chat -shot chat.png
//	CGO_ENABLED=0 go run ./example/chat -do 'reply;type:Yes;send' -after 4s -shot chat.png
package main

import (
	"context"
	"errors"
	"flag"
	"image/png"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/widget"
)

func main() {
	history := flag.Int("history", 300, "how many messages the first conversation starts with")
	fail := flag.Float64("fail", 0.1, "the share of sends the pretend server turns down")
	seed := flag.Uint64("seed", 1, "seeds the pretend history and colleagues")
	runFor := flag.Duration("for", 0, "quit after this long; zero runs until the window closes")
	shot := flag.String("shot", "", "write the window to this PNG file after -after, and quit")
	after := flag.Duration("after", 2*time.Second, "how long -shot waits")
	zoom := flag.Float64("zoom", 1, "zoom the window, as Ctrl with + and - does")
	do := flag.String("do", "", "a script of steps separated by semicolons to run at the start; see runScript")
	flag.Parse()
	var script []string
	if *do != "" {
		script = strings.Split(*do, ";")
	}
	if err := run(*history, *fail, *seed, script, *runFor, *shot, *after, float32(*zoom)); err != nil {
		log.Fatal(err)
	}
}

func run(history int, fail float64, seed uint64, script []string, runFor time.Duration, shot string, after time.Duration,
	zoom float32,
) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runFor)
		defer cancel()
	}
	return gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{
			Title: "Chat",
			Size:  geom.Sz(1180, 760),
			Root:  widget.NewSurface(),
		})
		if err != nil {
			return err
		}
		registerViews(w)
		c := w.Client()
		if zoom != 1 {
			if err := c.SetZoom(zoom); err != nil {
				return err
			}
		}
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
		// Finding which emoji the fonts draw takes a moment, so it starts now, before the picker needs it.
		go widget.LoadEmoji()
		app := newApp(ctx, c, seed, history, fail)
		app.openWindow = func(title string) (gunim.Client, error) {
			pw, err := a.NewWindow(gunim.WindowOptions{Title: title, Size: geom.Sz(560, 720), Root: widget.NewSurface()})
			if err != nil {
				return gunim.Client{}, err
			}
			registerViews(pw)
			return pw.Client(), nil
		}
		app.runScript(script)
		return app.serve()
	})
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
