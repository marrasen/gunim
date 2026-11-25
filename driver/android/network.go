//go:build android

package android

/*
#include <stdlib.h>
#include "glue.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"syscall"
	"unsafe"
)

// A Network is one of the networks the device is on, as Wi-Fi or mobile
// data, by the number Java's android.net.Network.getNetworkHandle gives.
// Zero is the one Android picks.
//
// Go's sockets and lookups use the network Android picks for the
// program, whichever network a job of Android's was started for. A
// program that should reach a server over one network and no other looks
// the name up there and has each socket keep to it:
//
//	addrs, err := n.LookupNetIP(ctx, "ip", host)
//	...
//	d := net.Dialer{Control: n.Control}
//	conn, err := d.DialContext(ctx, "tcp", netip.AddrPortFrom(addrs[0], port).String())
//
// A socket kept to a network that goes away fails, and moves to no other.
type Network int64

// Control keeps a socket to n. It is a Control for a [net.Dialer] and a
// [net.ListenConfig].
func (n Network) Control(_, _ string, c syscall.RawConn) error {
	var err error
	if cerr := c.Control(func(fd uintptr) { err = n.Bind(fd) }); cerr != nil {
		return cerr
	}
	return err
}

// Bind keeps the socket fd, which has sent nothing yet, to n.
func (n Network) Bind(fd uintptr) error {
	if e := C.gunim_bind_socket(C.longlong(n), C.int(fd)); e != 0 {
		return fmt.Errorf("android: keep a socket to a network: %w", syscall.Errno(e))
	}
	return nil
}

// LookupNetIP looks host up with the name servers of n, as
// [net.Resolver.LookupNetIP] does with the program's: network is ip, ip4
// or ip6. Every failure but a network it does not know is a
// [net.DNSError]. A lookup cannot be stopped, so one whose ctx ends runs
// on unseen.
func (n Network) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	family, ok := map[string]C.int{"ip": C.AF_UNSPEC, "ip4": C.AF_INET, "ip6": C.AF_INET6}[network]
	if !ok {
		return nil, net.UnknownNetworkError(network)
	}
	if err := ctx.Err(); err != nil {
		return nil, lookupError(host, err)
	}
	type answer struct {
		addrs []netip.Addr
		err   error
	}
	ch := make(chan answer, 1)
	go func() {
		addrs, err := n.lookup(family, host)
		ch <- answer{addrs, err}
	}()
	select {
	case a := <-ch:
		return a.addrs, a.err
	case <-ctx.Done():
		return nil, lookupError(host, ctx.Err())
	}
}

// lookupError is the failure of a lookup whose context ended.
func lookupError(host string, err error) error {
	return &net.DNSError{Err: err.Error(), Name: host, UnwrapErr: err, IsTimeout: errors.Is(err, context.DeadlineExceeded)}
}

// lookup asks for the addresses of host in family.
func (n Network) lookup(family C.int, host string) ([]netip.Addr, error) {
	ch := C.CString(host)
	defer C.free(unsafe.Pointer(ch))
	var found [64]C.gunim_addr
	var failed [128]C.char
	var kind C.int
	count := C.gunim_lookup(C.longlong(n), ch, family, &found[0], C.int(len(found)), &failed[0], C.int(len(failed)), &kind)
	if kind != 0 {
		return nil, &net.DNSError{Err: C.GoString(&failed[0]), Name: host,
			IsNotFound: kind == C.GUNIM_NO_NAME, IsTemporary: kind == C.GUNIM_TRY_AGAIN}
	}
	addrs := make([]netip.Addr, 0, int(count))
	for _, f := range found[:count] {
		b := C.GoBytes(unsafe.Pointer(&f.b[0]), f.n)
		a, ok := netip.AddrFromSlice(b)
		if !ok {
			continue
		}
		if f.scope != 0 {
			a = a.WithZone(strconv.Itoa(int(f.scope)))
		}
		addrs = append(addrs, a)
	}
	return addrs, nil
}
