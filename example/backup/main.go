// Command backup counts how often it has started, twice: in a file that Android backs up, and in one that it never
// does. Moved to another phone, the first count goes on, and the second starts over.
//
//	go run ./tools/gunimapk -run -name Backup ./example/backup
//
// Built with -backup=false, neither file is backed up, and both counts start over on another phone. With -keep-data,
// Android offers to keep both as the program is uninstalled:
//
//	go run ./tools/gunimapk -run -name Backup -backup=false -keep-data ./example/backup
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/widget"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, werr := a.NewWindow(gunim.WindowOptions{Title: "gunim backup", Size: geom.Sz(360, 240), Root: widget.NewSurface()})
		if werr != nil {
			return werr
		}
		text := started("backed up", keptDir) + "\n" + started("never backed up", noBackupDir)
		log.Print(text)
		gunim.RegisterView(w, "text", widget.NewLabel,
			func(l *widget.Label, s string, _ *gunim.UI) { l.Text = s })
		c := w.Client()
		if merr := c.Mount(gunim.Root, "text", "text", text); merr != nil {
			return merr
		}
		for range c.Intents() {
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

// started counts a start in the folder dir names, and says how many there have been.
func started(name string, dir func() (string, error)) string {
	n, err := count(dir)
	if err != nil {
		return fmt.Sprintf("%s: %v", name, err)
	}
	return fmt.Sprintf("%s: started %d times", name, n)
}

// count adds one to the number in a file in the folder dir names, and returns it.
func count(dir func() (string, error)) (int, error) {
	d, err := dir()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return 0, err
	}
	file := filepath.Join(d, "starts")
	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	// A file that holds no number starts the count over.
	n, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	n++
	return n, os.WriteFile(file, []byte(strconv.Itoa(n)), 0o600)
}

// keptDir is the folder for what the program keeps, which Android backs up.
func keptDir() (string, error) {
	d, err := os.UserConfigDir()
	return filepath.Join(d, "gunim-backup"), err
}
