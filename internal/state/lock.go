package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// lock takes the feature's lock, waiting while another process holds it, and returns its
// release. The lock is a flock on a file that baton never replaces, and the kernel releases it
// when its process ends, so a crash never leaves a feature locked.
func lock(root *os.Root, slug string) (func(), error) {
	file, err := openLock(root, filepath.Join(folder(slug), lockFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, noFeature(slug)
	}

	if err != nil {
		return nil, fmt.Errorf("lock feature %s: %w", slug, err)
	}

	if err := flock(file); err != nil {
		_ = file.Close()

		return nil, fmt.Errorf("lock feature %s: %w", slug, err)
	}

	return func() { _ = file.Close() }, nil
}

// openLock opens the lock file, after making it if it's missing. It makes it with O_EXCL, then
// opens it if it exists: on macOS, the openat(2) that os.Root uses sometimes fails with "no such
// file or directory" when it makes a file with O_CREATE alone while another opener makes it too
// (fact 19 of docs/plan.md).
func openLock(root *os.Root, name string) (*os.File, error) {
	file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, privateFile)
	if errors.Is(err, fs.ErrExist) {
		return root.OpenFile(name, os.O_RDWR, 0)
	}

	return file, err
}

// flock takes an exclusive lock on the file, and waits again when a signal interrupts it.
func flock(file *os.File) error {
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}
