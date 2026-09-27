package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// userFolders returns where Windows keeps the user's desktop, documents,
// downloads and pictures.
func userFolders(string) ([]userPlace, error) {
	known := []struct {
		name, kind string
		id         *windows.KNOWNFOLDERID
	}{
		{"Desktop", "desktop", windows.FOLDERID_Desktop},
		{"Documents", "documents", windows.FOLDERID_Documents},
		{"Downloads", "downloads", windows.FOLDERID_Downloads},
		{"Pictures", "pictures", windows.FOLDERID_Pictures},
	}
	out := make([]userPlace, 0, len(known))
	for _, k := range known {
		path, err := windows.KnownFolderPath(k.id, 0)
		if err != nil {
			return nil, fmt.Errorf("finding your %s folder: %w", k.name, err)
		}
		out = append(out, userPlace{name: k.name, kind: k.kind, path: path})
	}
	return out, nil
}

// volumes returns a place for each drive letter.
func volumes() ([]Place, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("listing the drives: %w", err)
	}
	var out []Place
	for i := range 26 {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p := Place{Path: root, Kind: "drive", Name: root[:2]}
		r, err := windows.UTF16PtrFromString(root)
		if err != nil {
			return nil, err
		}
		label := make([]uint16, windows.MAX_PATH+1)
		if err := windows.GetVolumeInformation(r, &label[0], uint32(len(label)), nil, nil, nil, nil, 0); err != nil {
			p.Err = err.Error()
		} else if l := windows.UTF16ToString(label); l != "" {
			p.Name = l + " (" + root[:2] + ")"
		}
		out = append(out, p)
	}
	return out, nil
}

// volumeSpace returns the room on the volume that holds path.
func volumeSpace(path string) (space, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return space{}, err
	}
	var free, total, all uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &all); err != nil {
		return space{}, fmt.Errorf("reading the free space of %s: %w", path, err)
	}
	return space{free: free, total: total}, nil
}
