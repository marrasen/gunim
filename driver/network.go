package driver

import "errors"

// NetworkState is what the system says of the network an application's connections use.
type NetworkState struct {
	// Online says that there is a network. It may reach no further than itself, as a home's with its line down.
	Online bool
	// Internet says that the system has reached the internet over it.
	Internet bool
	// Metered says that what goes over it may cost the user, as mobile data does.
	Metered bool
	// Blocked says that the system keeps the application off the network now, though there may be one: Android 15
	// and later do a few seconds after it leaves the screen, unless a job of its own runs, and older ones as they save
	// battery or data.
	Blocked bool
	// Network names the network, as the Network of driver/android does, and is 0 with none.
	Network int64
	// Changed says that what was connected before may no longer be: since the last call the network became another, an
	// address of the device's on it went, or it was blocked or let go.
	Changed bool
}

// Usable reports whether the application can use a network now.
func (s NetworkState) Usable() bool { return s.Online && !s.Blocked }

// A NetworkWatcher is a [Driver] whose system tells of the network.
type NetworkWatcher interface {
	// WatchNetwork has fn hear how the network is, soon, and then each time that changes, on a goroutine of the
	// driver's, one call at a time, until stop. A fn slower than the changes hears the last of them. A call that has
	// begun as stop is called goes on.
	WatchNetwork(fn func(NetworkState)) (stop func(), err error)
}

// ErrNoNetworkWatch is returned where gunim does not hear of the network.
var ErrNoNetworkWatch = errors.New("driver: no news of the network on this platform")
