package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run joins the two halves over a channel in this process.
//
// The seam is [gunim.Client]. Putting an aprot connection there instead
// would move serve to another machine, and both halves would carry on
// as they are.
func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{
			Title: "gunim",
			Size:  geom.Sz(900, 620),
			Root:  &shell{},
		})
		if err != nil {
			return fmt.Errorf("dialog example: %w", err)
		}
		registerViews(w)
		return serve(ctx, w.Client())
	})
	if errors.Is(err, driver.ErrNoDriver) {
		log.Print("gunim has no driver for this operating system yet")
		return nil
	}
	return err
}
