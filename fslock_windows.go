package resurrent

import (
	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

type fsLock struct {
	handle windows.Handle
}

func lockFS(lockPath string) (*fsLock, error) {
	lockPathPtr, err := windows.UTF16PtrFromString(lockPath)
	if err != nil {
		return nil, errors.Wrap(err, "encode lock path")
	}
	handle, err := windows.CreateFile(
		lockPathPtr,
		windows.GENERIC_ALL,
		0, // SHARE_NOTHING,
		nil,
		windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_DELETE_ON_CLOSE,
		windows.Handle(0),
	)
	if err != nil {
		return nil, errors.Wrap(err, "lock filesystem")
	}
	return &fsLock{handle}, nil
}

func (l *fsLock) unlock() {
	_ = windows.Close(l.handle)
}
