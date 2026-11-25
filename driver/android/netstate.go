//go:build android

package android

/*
#include "glue.h"
*/
import "C"

import (
	"errors"

	"github.com/marrasen/gunim/driver"
)

// errNetworkState is returned where Android tells nothing of the network.
var errNetworkState = errors.New("android: no news of the network; gunimapk's -permissions network-state lets a program hear it")

// netNews tells of the network, and listens while anything watches it.
var netNews = driver.NetworkNews{Listen: func(run int64, on bool) error {
	if C.gunim_watch_network(C.longlong(run), cBool(on)) == 0 {
		return errNetworkState
	}
	return nil
}}

// WatchNetwork implements [driver.NetworkWatcher]. Before Android 10 it
// hears that the program is blocked, or let go, as something else on the
// network changes.
func (d *Driver) WatchNetwork(fn func(driver.NetworkState)) (stop func(), err error) {
	return netNews.Watch(fn)
}

//export goNetwork
func goNetwork(run, network C.longlong, online, internet, metered, blocked, moved C.uchar) {
	netNews.Set(int64(run), driver.NetworkState{
		Network: int64(network), Online: online != 0, Internet: internet != 0, Metered: metered != 0, Blocked: blocked != 0,
	}, moved != 0)
}
