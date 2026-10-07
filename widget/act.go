package widget

import "github.com/marrasen/gunim"

// act plays cue c from n, then runs fn with a, when there is one, and sends what it returns from n. Every widget that
// acts for the user goes through it or [act0], so each sounds as it acts and runs its callback the same way.
func act[A any](u *gunim.UI, n gunim.Node, c gunim.Cue, fn func(A, *gunim.UI) gunim.Intent, a A) {
	u.Cue(c, n)
	if fn != nil {
		send(u, n, fn(a, u))
	}
}

// act0 is [act] for a callback that takes the UI alone.
func act0(u *gunim.UI, n gunim.Node, c gunim.Cue, fn func(*gunim.UI) gunim.Intent) {
	u.Cue(c, n)
	if fn != nil {
		send(u, n, fn(u))
	}
}
