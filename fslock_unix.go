//go:build !windows

package resurrent

import (
	"os"
)

type fsLock struct {
	lockPath string
}

func lockFS(lockPath string) (*fsLock, error) {
	f, err := os.OpenFile(
		lockPath, os.O_CREATE|os.O_EXCL, localFileMode,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return &fsLock{lockPath}, nil
}

func (l *fsLock) unlock() {
	_ = os.Remove(l.lockPath)
}
