package driver

import (
	"errors"
	"io"
)

// A SavedFile is a file the user chose a place for, as it is written. It
// ends with Close, for a file that is whole, or with Remove. A write waits
// for the disk, or for the application that keeps the file, and Remove
// ends it.
type SavedFile interface {
	io.Writer
	// Name is the file's name where it is kept, which may differ from the
	// one asked for. A file that another takes the name of as it is
	// written may change it in Close.
	Name() string
	// Place says where the file is to the system, for [Remover.RemoveSaved]
	// in a later run, where this one ends before Close or Remove. It holds
	// the file's name and folder. It is of no use once Close has returned.
	// It is "" for a file that was there, and is written over where it is:
	// what is left of that is not the program's to take away.
	Place() string
	// Made reports whether the user's choice made the file. It is false
	// where they chose one that was there, to write over.
	Made() bool
	// Close says that the file is whole: it waits until all of it is
	// kept, and returns an error where it is not. It returns
	// [ErrMayNotBeKept] where all of it was handed over and nothing can
	// tell what becomes of it. The file is there after an error too, until
	// Remove, and a program that is content with one that may not be kept
	// leaves it there.
	Close() error
	// Remove takes away what was written of a file that is not whole. It
	// returns [ErrNotMade] for a file the program did not make, which it
	// leaves as far as it was written. It changes nothing after a Close
	// that returned nil, or while Close gives the file its name, and
	// returns an error.
	Remove() error
}

// A SavedFolder is a folder the user chose, to write files in. Its paths
// are relative, with slashes, as [io/fs.ValidPath] has them, and the
// folders on the way are made.
type SavedFolder interface {
	// Create makes a file at path to write. It returns an error that is
	// [io/fs.ErrExist] where there is one, and writes over none. Until
	// Close the file has another name where the folder allows it, so that
	// one that was cut short does not look whole.
	Create(path string) (SavedFile, error)
	// Mkdir makes a folder at path, and is content with one that is there.
	Mkdir(path string) error
	// Close lets the folder go. Files open in it go on.
	Close() error
}

// A StreamSaver is a [Window] that shows the system's dialog for saving a
// file, or for choosing a folder to write in, and writes there as the
// bytes come. Each returns nil where the user chose nothing, and may be
// called from any goroutine but the main one.
type StreamSaver interface {
	SaveStream(o SaveOptions) (SavedFile, error)
	SaveFolder(o ChooseOptions) (SavedFolder, error)
}

// A Remover is a [Driver] that takes away what an earlier run left of a
// file it was saving, by its [SavedFile.Place]. ForgetSaved gives back the
// rights that earlier runs kept for that, but for the places in but, and
// those this run uses.
type Remover interface {
	RemoveSaved(place string) error
	ForgetSaved(but []string)
}

// ErrNotMade is returned for a file the program did not make, and so does
// not take away.
var ErrNotMade = errors.New("driver: the file was there before")

// ErrMayNotBeKept is returned by a [SavedFile]'s Close where all of the file
// was handed to the application that keeps it, as a cloud's, and that has
// not said that it keeps it: it may fail as it stores the file, or sends it
// on, and tell no one. A program that would delete the only other copy of
// the file does not go by this.
var ErrMayNotBeKept = errors.New("driver: the file may not be kept")

// ErrNotSaved is returned where a file could not be kept, or taken away.
// It holds no word of the application that keeps the file.
var ErrNotSaved = errors.New("driver: the file was not kept")
