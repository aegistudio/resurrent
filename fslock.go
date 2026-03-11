package resurrent

import (
	"path/filepath"

	"github.com/pkg/errors"
)

// FSLock to lock the root of a resurrent filesystem.
//
// This is used by both the implementor of resurrent
// filesystem, and the migration scripts. The user may
// also use the lock when they want to perform file
// processing on the filesystem.
type FSLock struct {
	inner *fsLock
	root  string
}

// LockFS locks the resurrent filesystem.
func LockFS(root string) (*FSLock, error) {
	var err error
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, errors.Wrap(err, "evaluate absolute path")
	}
	fsLock, err := lockFS(
		filepath.Join(root, "resurrent.lock"),
	)
	if err != nil {
		return nil, errors.Wrap(err, "lock fs")
	}
	return &FSLock{
		root:  root,
		inner: fsLock,
	}, nil
}

// RootPath returns the absolute path of root.
func (l *FSLock) RootPath() string {
	return l.root
}

func (l *FSLock) Unlock() {
	defer l.inner.unlock()
}
