//go:build !windows

package sessions

import (
	"golang.org/x/sys/unix"
	"os"
)

func lockFile(file *os.File) error {
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return err
	}
	lock := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	if err := unix.FcntlFlock(file.Fd(), unix.F_SETLK, &lock); err != nil {
		unix.Flock(int(file.Fd()), unix.LOCK_UN)
		return err
	}
	return nil
}
