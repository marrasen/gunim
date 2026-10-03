package filemanager_test

import (
	"context"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/filemanager"
)

// A program opens a window on the computer's own disk, with its servers
// in the sidebar under a heading of their own. A click on a server asks
// the program to visit it, which it does by turning the window that
// asked to the server's file system, or, with Ctrl held, by opening a
// window of its own on it.
func ExampleHub() {
	ctx := context.Background()
	_ = gunim.Main(ctx, func(ga *gunim.App) error {
		hub := filemanager.NewHub(ctx, ga)
		places := func() ([]filemanager.Place, error) {
			ps, err := filemanager.LocalPlaces()
			for i := range ps {
				ps[i].Group = "This computer"
			}
			ps = append(ps, filemanager.Place{Name: "web1", Path: "/home/deploy", Kind: "drive", Group: "Servers",
				Note: "Connected", FS: "sftp://web1"})
			return ps, err
		}
		var visit func(w *filemanager.Window, fs, path string, newWindow bool)
		visit = func(w *filemanager.Window, fs, path string, newWindow bool) {
			if fs != "" {
				// The program connects to the server fs names, and shows it:
				//
				//	w.Show(server, path)
				//
				// or, for a new window, opens one with FS: server.
				return
			}
			if newWindow {
				_, _ = hub.Open(filemanager.Options{Name: "Kakel", Dir: path, Places: places, Visit: visit})
				return
			}
			w.Show(filemanager.LocalFS(), path)
		}
		w, err := hub.Open(filemanager.Options{Name: "Kakel", Places: places, Visit: visit})
		if err != nil {
			return err
		}
		go func() {
			<-w.Done()
			// The window has closed.
		}()
		return hub.Wait()
	})
}
