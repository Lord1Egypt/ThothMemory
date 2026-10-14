//go:build !windows

package ultralite

import (
	"os"

	"golang.org/x/sys/unix"
)

// lock takes an advisory lock on f: shared for running instances, exclusive
// (and non-blocking) for restore.
func lock(f *os.File, exclusive bool) error {
	how := unix.LOCK_SH
	if exclusive {
		how = unix.LOCK_EX | unix.LOCK_NB
	}
	return unix.Flock(int(f.Fd()), how)
}
