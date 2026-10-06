//go:build !(windows && (amd64 || arm64))

package filemanager

import (
	"errors"
	"image"
)

// systemIconsHere says the system gives no icons of its own for files here: the window draws its own.
const systemIconsHere = false

func systemIcon(string, string, bool) (large, small image.Image, err error) {
	return nil, nil, errors.ErrUnsupported
}

func ownIcon(string) bool { return false }
