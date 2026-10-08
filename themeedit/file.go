package themeedit

import (
	"os"

	"github.com/marrasen/gunim/theme"
)

// WriteFile writes the values over sets to the file at path, as JSON,
// for a program that keeps the user's theme in a file of its own.
func WriteFile(path string, over theme.Theme) error {
	data, err := theme.MarshalValues(over)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ReadFile reads values written by [WriteFile]. Values it cannot read
// are left out, and the error lists them; the theme returned holds the
// rest. A file that is not there gives an empty theme and an error that
// [os.IsNotExist] knows.
func ReadFile(path string) (theme.Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return theme.Make(""), err
	}
	return theme.UnmarshalValues(theme.Make(""), data)
}
