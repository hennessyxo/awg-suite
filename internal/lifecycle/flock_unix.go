//go:build !windows

package lifecycle

import (
	"os"
	"syscall"
)

// fileLock is an advisory, cross-process exclusive lock held on a lock file.
// It serializes load-modify-save transactions between the panel daemon and any
// `awg-panel client-*` CLI invocation that edits the same store.
type fileLock struct{ f *os.File }

// acquireLock opens (creating if needed) the lock file and takes an exclusive
// flock. It blocks until the lock is available.
func acquireLock(path string) (*fileLock, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &fileLock{f: f}, nil
}

func (l *fileLock) release() {
	if l == nil || l.f == nil {
		return
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
}
