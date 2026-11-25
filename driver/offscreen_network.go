package driver

import (
	"sync"

	"github.com/marrasen/gunim/driver/internal/news"
)

// NetworkNews tells those that watch of the network, for a driver. It says nothing until Set. The zero value is ready.
type NetworkNews struct {
	// Listen, where it is set, has the driver start to listen to the network, as the first watches, and stop, as the
	// last has stopped. run numbers the time it listens: news of one that has stopped, which may have been on its way,
	// is left out by it. Listen must not wait for a Set.
	Listen func(run int64, on bool) error

	mu       sync.Mutex
	latest   *news.Latest[NetworkState]
	last     NetworkState
	has      bool
	watchers int
	run      int64
}

// hub returns the news. It runs with mu held.
func (n *NetworkNews) hub() *news.Latest[NetworkState] {
	if n.latest == nil {
		n.latest = &news.Latest[NetworkState]{
			Merge: func(old, next NetworkState) NetworkState {
				next.Changed = next.Changed || old.Changed
				return next
			},
			First: func(s NetworkState) NetworkState {
				s.Changed = false
				return s
			},
		}
	}
	return n.latest
}

// Set tells of s, as the run of Listen's heard it, or 0 with no Listen. moved says that the network reaches the device in
// another way, so that what was connected may be no longer. It works Changed out, and tells nothing where nothing is new.
func (n *NetworkNews) Set(run int64, s NetworkState, moved bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.Listen != nil && (run != n.run || n.watchers == 0) {
		return
	}
	s.Changed = false
	if n.has && n.last == s && !moved {
		return
	}
	had, was := n.has, n.last
	n.has, n.last = true, s
	s.Changed = had && (moved || was.Network != s.Network || was.Online != s.Online || was.Blocked != s.Blocked)
	n.hub().Set(s)
}

// Watch is [NetworkWatcher.WatchNetwork].
func (n *NetworkNews) Watch(fn func(NetworkState)) (stop func(), err error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.watchers == 0 && n.Listen != nil {
		n.run++
		if err := n.Listen(n.run, true); err != nil {
			return nil, err
		}
	}
	n.watchers++
	stopNews := n.hub().Watch(fn)
	return sync.OnceFunc(func() {
		stopNews()
		n.mu.Lock()
		defer n.mu.Unlock()
		n.watchers--
		if n.watchers > 0 || n.Listen == nil {
			return
		}
		// There is no more to do where it does not stop.
		_ = n.Listen(n.run, false)
		n.has, n.last = false, NetworkState{}
		n.hub().Forget()
	}), nil
}

// OffscreenNetwork is a network of a test's own: it implements [NetworkWatcher], and a test's [Driver] embeds it. The
// zero value is ready, and says nothing until SetNetwork.
type OffscreenNetwork struct{ news NetworkNews }

// SetNetwork tells those that watch of s. It works Changed out itself.
func (o *OffscreenNetwork) SetNetwork(s NetworkState) { o.news.Set(0, s, false) }

// WatchNetwork implements [NetworkWatcher].
func (o *OffscreenNetwork) WatchNetwork(fn func(NetworkState)) (stop func(), err error) {
	return o.news.Watch(fn)
}
