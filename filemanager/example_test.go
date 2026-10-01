package filemanager_test

import (
	"context"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/filemanager"
)

// A program opens a window on the computer's own disk, with its servers
// in the sidebar under a heading of their own. A click on a server asks
// the program to visit it, which it does by opening a window on the
// server's file system.
func ExampleHub() {
	ctx := context.Background()
	_ = gunim.Main(ctx, func(ga *gunim.App) error {
		hub := filemanager.NewHub(ga)
		places := func() ([]filemanager.Place, error) {
			ps, err := filemanager.LocalPlaces()
			for i := range ps {
				ps[i].Group = "This computer"
			}
			ps = append(ps, filemanager.Place{Name: "web1", Path: "/home/deploy", Kind: "drive", Group: "Servers",
				Note: "Connected", FS: "sftp://web1"})
			return ps, err
		}
		visit := func(fs, path string) {
			// The program connects to the server fs names, and opens a
			// window on it, which visits other places as this one does:
			//
			//	hub.Open(ctx, filemanager.Options{FS: server, Dir: path, Places: places, Visit: ...})
		}
		if _, err := hub.Open(ctx, filemanager.Options{Places: places, Visit: visit}); err != nil {
			return err
		}
		return hub.Wait()
	})
}
