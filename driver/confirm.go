package driver

import (
	"context"
	"errors"
	"unicode/utf8"
)

// ConfirmOptions say what the system's prompt asks the user to confirm; see [Confirmer].
type ConfirmOptions struct {
	// Title is what the prompt asks for, as "Open the notes", and Text says more. Both are in the user's language. An
	// empty Title takes the application's name.
	Title, Text string
	// Strong leaves out the faces and fingers that Android counts as weak, as a face read with a plain camera. The PIN,
	// the pattern or the password is taken all the same. Before Android 11 it changes nothing.
	Strong bool
}

// A Confirmer is a [Driver] whose system asks whether it is the user who holds the device, by a finger, a face, or the
// PIN, pattern or password of its screen lock.
type Confirmer interface {
	// ConfirmPerson shows the system's prompt, and returns nil once the user has confirmed, and only then. It waits for
	// the application to be in front, and blocks until the user answers or ctx ends, which takes the prompt away. It
	// fails with [ErrNotConfirmed], [ErrNoScreenLock] or [ErrConfirming], or ctx's error.
	ConfirmPerson(ctx context.Context, o ConfirmOptions) error
	// HasScreenLock reports whether the device has a screen lock with a PIN, a pattern or a password.
	HasScreenLock() bool
}

// ErrNotConfirmed is returned where the user did not confirm: they left the prompt, or it went as they left the
// application or the screen turned off, or they tried too often.
var ErrNotConfirmed = errors.New("driver: the user did not confirm")

// ErrNoScreenLock is returned where the device has no screen lock for the user to confirm with.
var ErrNoScreenLock = errors.New("driver: the device has no screen lock")

// ErrConfirming is returned where a prompt is up, or waits for the application to be in front.
var ErrConfirming = errors.New("driver: the user is being asked to confirm already")

// ErrNoConfirmer is returned where gunim has no such prompt to show.
var ErrNoConfirmer = errors.New("driver: no prompt to confirm with on this platform")

// ErrConfirmText is returned for a prompt whose text is not UTF-8.
var ErrConfirmText = errors.New("driver: the prompt's text is not UTF-8")

// CheckConfirm returns [ErrConfirmText] for a prompt whose text is not UTF-8.
func CheckConfirm(o ConfirmOptions) error {
	if !utf8.ValidString(o.Title) || !utf8.ValidString(o.Text) {
		return ErrConfirmText
	}
	return nil
}
