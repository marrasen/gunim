package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Kind is what a folder entry is.
type Kind uint8

// The kinds of entry.
const (
	KindFile Kind = iota
	KindFolder
	KindLink
)

// entry is one item of a folder, as the app half keeps it.
type entry struct {
	Name string
	Kind Kind
	// Dir is set for a folder, and for a link to one.
	Dir    bool
	Size   int64
	Mod    time.Time
	Hidden bool
	// Broken is set for a link whose target is missing, and for an item
	// that cannot be read.
	Broken bool
	Type   string
	// Err says why the item, or the target of the link it is, cannot be
	// read.
	Err string
	// lower is the name in lower case, for sorting and filtering.
	lower string
}

// listDir reads the entries of dir. An entry it cannot read lists with
// its name and why, and an entry deleted while it lists is left out, so
// the errors it returns are about dir itself.
func listDir(ctx context.Context, dir string) (es []entry, err error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			es, err = nil, fmt.Errorf("reading %s: %w", dir, cerr)
		}
	}()
	var out []entry
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch, rerr := f.ReadDir(1024)
		for _, d := range batch {
			if e, ok := readEntry(dir, d); ok {
				out = append(out, e)
			}
		}
		if errors.Is(rerr, io.EOF) {
			return out, nil
		}
		if rerr != nil {
			return nil, fmt.Errorf("reading %s: %w", dir, rerr)
		}
		if len(batch) == 0 {
			return out, nil
		}
	}
}

// readEntry turns a directory entry into an entry. ok is false for an
// entry deleted since dir was read.
func readEntry(dir string, d fs.DirEntry) (e entry, ok bool) {
	info, err := d.Info()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return entry{}, false
	case err != nil:
		name := d.Name()
		return entry{Name: name, Kind: KindFile, Hidden: strings.HasPrefix(name, "."), Broken: true,
			Type: "Cannot be read", Err: err.Error(), lower: strings.ToLower(name)}, true
	}
	return makeEntry(dir, info), true
}

// makeEntry turns what Lstat says about an item of dir into an entry.
func makeEntry(dir string, info fs.FileInfo) entry {
	name := info.Name()
	e := entry{Name: name, Size: info.Size(), Mod: info.ModTime(), lower: strings.ToLower(name)}
	e.Hidden = strings.HasPrefix(name, ".") || hiddenAttr(info)
	switch {
	case info.IsDir():
		e.Kind, e.Dir, e.Size = KindFolder, true, 0
		e.Type = "Folder"
	case isLink(info):
		e.Kind = KindLink
		target, err := os.Stat(filepath.Join(dir, name))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			e.Broken = true
			e.Type = "Broken link"
		case err != nil:
			// A link to itself, or into a folder that cannot be read.
			e.Broken = true
			e.Type = "Link that cannot be followed"
			e.Err = err.Error()
		case target.IsDir():
			e.Dir, e.Size = true, 0
			e.Type = "Link to folder"
		default:
			e.Size = target.Size()
			e.Type = "Link to " + typeLabel(name)
		}
	default:
		e.Type = typeLabel(name)
	}
	return e
}

// isLink reports whether info is a symbolic link, or a junction on Windows.
func isLink(info fs.FileInfo) bool {
	return info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0
}

// statEntry reads one entry by its path.
func statEntry(path string) (entry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return entry{}, err
	}
	return makeEntry(filepath.Dir(path), info), nil
}

// typeLabel names the type of a file from its extension.
func typeLabel(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	if ext == "" || ext == strings.ToLower(strings.TrimPrefix(name, ".")) {
		return "File"
	}
	if t, ok := typeNames[ext]; ok {
		return t
	}
	return strings.ToUpper(ext) + " file"
}

// typeNames are the labels of the extensions that have a better name than
// their letters.
var typeNames = map[string]string{
	"txt": "Text", "md": "Markdown", "log": "Log", "csv": "CSV table", "tsv": "TSV table",
	"json": "JSON", "xml": "XML", "yaml": "YAML", "yml": "YAML", "toml": "TOML", "ini": "Settings",
	"go": "Go source", "c": "C source", "h": "C header", "cpp": "C++ source", "rs": "Rust source",
	"py": "Python script", "js": "JavaScript", "ts": "TypeScript", "html": "Web page", "htm": "Web page",
	"css": "Stylesheet", "sh": "Shell script", "bat": "Batch file", "ps1": "PowerShell script",
	"png": "PNG image", "jpg": "JPEG image", "jpeg": "JPEG image", "gif": "GIF image", "bmp": "Bitmap image",
	"webp": "WebP image", "tif": "TIFF image", "tiff": "TIFF image", "svg": "SVG image", "ico": "Icon",
	"pdf": "PDF document", "doc": "Word document", "docx": "Word document", "xls": "Excel sheet",
	"xlsx": "Excel sheet", "ppt": "Presentation", "pptx": "Presentation", "odt": "Document",
	"zip": "Zip archive", "7z": "7-Zip archive", "rar": "RAR archive", "tar": "Tar archive",
	"gz": "Gzip archive", "xz": "XZ archive", "bz2": "Bzip2 archive", "zst": "Zstandard archive",
	"exe": "Program", "msi": "Installer", "dll": "Library", "so": "Library", "lnk": "Shortcut",
	"mp3": "MP3 audio", "wav": "WAV audio", "flac": "FLAC audio", "ogg": "Ogg audio",
	"mp4": "MP4 video", "mkv": "Matroska video", "mov": "QuickTime video", "avi": "AVI video",
	"iso": "Disc image", "ttf": "Font", "otf": "Font",
}
