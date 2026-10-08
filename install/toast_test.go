package install

import (
	"strings"
	"testing"
)

type (
	toastFetch   struct{}
	toastRestart struct{}
	toastNotes   struct{}
)

// TestTheUpdateToastsAskAndTell makes each toast, and finds its words,
// that it asks, and the intent its first button sends.
func TestTheUpdateToastsAskAndTell(t *testing.T) {
	a := App{Name: "Studio", Version: "v0.4.0"}
	for _, c := range []struct {
		name  string
		title string
		body  string
		send  any
		make  func() (string, string, int, any)
	}{
		{"available", "Studio 0.4.1 is out", "Fetch it now?", toastFetch{}, func() (string, string, int, any) {
			to := AvailableToast(a, "v0.4.1", toastFetch{})
			return to.Title, to.Body, len(to.Buttons), to.Buttons[0].OnClick(false, nil)
		}},
		{"ready", "Studio 0.4.1 is ready", "Restart now", toastRestart{}, func() (string, string, int, any) {
			to := ReadyToast(a, "v0.4.1", toastRestart{})
			return to.Title, to.Body, len(to.Buttons), to.Buttons[0].OnClick(false, nil)
		}},
		{"updated", "Studio is updated to 0.4.0", "It updated itself from 0.3.2.", toastNotes{}, func() (string, string, int, any) {
			to := UpdatedToast(a, "v0.3.2", toastNotes{})
			return to.Title, to.Body, len(to.Buttons), to.Buttons[0].OnClick(false, nil)
		}},
	} {
		title, body, buttons, send := c.make()
		if title != c.title || !strings.HasPrefix(body, c.body) || buttons != 2 || send != c.send {
			t.Errorf("%s: %q, %q, %d buttons, sends %v", c.name, title, body, buttons, send)
		}
	}
}
