package driver

import "time"

// A BackKeeper is a [Window] on a system whose user goes back by a gesture or a button of the system's, as on Android.
// The driver sends each Back as [BackAsked], until SetBackLeaves(true) says that the next leaves the application, which
// the system then does itself. The engine calls it as that changes.
type BackKeeper interface {
	SetBackLeaves(leaves bool)
	// LeaveApp leaves the application, as a Back it does not take does: it goes to the background, and runs on.
	LeaveApp()
}

// BackAsked is sent on [Window.Input] as the user goes back.
type BackAsked struct{ Time time.Time }
