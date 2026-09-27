package main

import "github.com/marrasen/gunim"

// RowsLeft is a patch: the rows of listing Gen's page that went as it
// came, by their index in the listing before, for them to leave.
type RowsLeft struct {
	Gen  int
	Rows []int
}

func init() {
	gunim.RegisterType[RowsLeft]("files.rows-left")
}
