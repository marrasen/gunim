//go:build !(windows && (amd64 || arm64))

package main

import "image"

// cachedShellThumb returns the thumbnail the system already keeps of the file at path. Only Windows keeps one to ask
// for.
func cachedShellThumb(string, int) (image.Image, error) { return nil, nil }
