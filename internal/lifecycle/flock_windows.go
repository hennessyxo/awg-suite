//go:build windows

package lifecycle

// On Windows the lifecycle store is never written by two processes at once: the
// panel daemon and the `awg-panel client-*` CLI both run only on the Linux
// server. The desktop GUI imports this package for its types but never opens a
// Store on Windows. So the lock is a no-op here, which keeps the cross-platform
// build green without pulling in Windows file-locking APIs.
type fileLock struct{}

func acquireLock(_ string) (*fileLock, error) { return &fileLock{}, nil }

func (l *fileLock) release() {}
