package driver

import (
	"sync"

	"github.com/marrasen/gunim/driver/internal/news"
)

// NetworkNews tells those that watch of the network, for a driver. The zero value is ready, and says nothing until Set.
type NetworkNews struct {
	mu     sync.Mutex
	latest *news.Latest[NetworkState]
	last   NetworkState
	has    bool
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

// Set tells of s, and moved says that an address on the network is another. It works Changed out, and tells nothing
// where nothing is new.
func (n *NetworkNews) Set(s NetworkState, moved bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	s.Changed = false
	if n.has && n.last == s && !moved {
		return
	}
	had, was := n.has, n.last
	n.has, n.last = true, s
	s.Changed = had && (moved || was.Network != s.Network || was.Online != s.Online || was.Blocked != s.Blocked)
	n.hub().Set(s)
}

// Forget has those that start to watch hear nothing until the next Set, as where the driver stops listening.
func (n *NetworkNews) Forget() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.has, n.last = false, NetworkState{}
	n.hub().Forget()
}

// Watch is [NetworkWatcher.WatchNetwork]'s part that tells fn.
func (n *NetworkNews) Watch(fn func(NetworkState)) (stop func()) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.hub().Watch(fn)
}

// OffscreenNetwork is a network of a test's own: it implements [NetworkWatcher], and a test's [Driver] embeds it. The
// zero value is ready, and says nothing until SetNetwork.
type OffscreenNetwork struct{ news NetworkNews }

// SetNetwork tells those that watch of s. It works Changed out itself.
func (o *OffscreenNetwork) SetNetwork(s NetworkState) { o.news.Set(s, false) }

// WatchNetwork implements [NetworkWatcher].
func (o *OffscreenNetwork) WatchNetwork(fn func(NetworkState)) (stop func(), err error) {
	return o.news.Watch(fn), nil
}
