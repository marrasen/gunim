//go:build !windows

package filemanager

import "errors"

// readAttrs reports that there are no attributes to read here.
func readAttrs(string) (readOnly, hidden, ok bool, err error) { return false, false, false, nil }

// setAttrs fails: the attributes are Windows's.
func setAttrs(string, bool, bool) error {
	return errors.New("read-only and hidden are attributes of Windows")
}
