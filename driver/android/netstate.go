//go:build android

package android

/*
#include "glue.h"
*/
import "C"

import (
	"errors"
	"sync"

	"github.com/marrasen/gunim/driver"
)

// errNetworkState is returned where Android tells nothing of the network.
var errNetworkState = errors.New("android: no news of the network; gunimapk's -permissions network-state lets a program hear it")

// netState tells of the network while anything watches it.
var netState struct {
	mu       sync.Mutex
	watchers int
	news     driver.NetworkNews
}

// WatchNetwork implements [driver.NetworkWatcher]. Before Android 10 it
// hears that the program is blocked, or let go, as something else on the
// network changes.
func (d *Driver) WatchNetwork(fn func(driver.NetworkState)) (stop func(), err error) {
	n := &netState
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.watchers == 0 && C.gunim_watch_network(1) == 0 {
		return nil, errNetworkState
	}
	n.watchers++
	stopNews := n.news.Watch(fn)
	return sync.OnceFunc(func() {
		stopNews()
		n.mu.Lock()
		defer n.mu.Unlock()
		n.watchers--
		if n.watchers == 0 {
			C.gunim_watch_network(0)
			n.news.Forget()
		}
	}), nil
}

//export goNetwork
func goNetwork(network C.longlong, online, internet, metered, blocked, moved C.uchar) {
	netState.news.Set(driver.NetworkState{
		Network: int64(network), Online: online != 0, Internet: internet != 0, Metered: metered != 0, Blocked: blocked != 0,
	}, moved != 0)
}
