package main

import "github.com/marrasen/gunim"

// OpSpeed is a patch: how fast operation ID moves bytes now, in bytes a
// second, how many seconds are left, and the file it is on.
type OpSpeed struct {
	ID   int
	Rate float64
	Left float64
	File string
}

// OpDone is a patch: operation ID has ended, well with OK, and leaves the
// panel once it has shown so.
type OpDone struct {
	ID int
	OK bool
}

func init() {
	gunim.RegisterType[OpSpeed]("files.op-speed")
	gunim.RegisterType[OpDone]("files.op-done")
}
