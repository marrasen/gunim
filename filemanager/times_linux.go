package filemanager

import (
	"io/fs"
	"syscall"
	"time"
)

// fileTimes returns when info's file was last read. Linux keeps no time
// it was made that Go reads, so created is zero.
func fileTimes(info fs.FileInfo) (created, accessed time.Time, ok bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	return time.Time{}, time.Unix(st.Atim.Unix()), true
}
