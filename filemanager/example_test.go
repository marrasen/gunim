package filemanager_test

import (
	"context"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/filemanager"
)

// A program opens a window on the computer's own disk, with its servers
// in the sidebar under a heading of their own. A click on a server asks
// the program to visit it, which it does by turning the window that
// asked to the server's file system.
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
		visit := func(w *filemanager.Window, fs, path string) {
			if fs == "" {
				w.Show(filemanager.LocalFS(), path)
				return
			}
			// The program connects to the server fs names, and shows it:
			//
			//	w.Show(server, path)
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
