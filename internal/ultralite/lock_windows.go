package ultralite

import (
	"os"

	"golang.org/x/sys/windows"
)

// lock takes a byte-range lock on f: shared for running instances, exclusive
// (and failing immediately) for restore.
func lock(f *os.File, exclusive bool) error {
	flags := uint32(0)
	if exclusive {
		flags = windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	return windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &windows.Overlapped{})
}
