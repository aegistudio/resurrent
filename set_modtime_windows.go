package resurrent

import (
	"os"
	"runtime"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

func setModtime(f *os.File, t time.Time) error {
	wtime := windows.NsecToFiletime(t.UnixNano())
	err := windows.SetFileTime(
		windows.Handle(f.Fd()),
		nil, nil, &wtime,
	)
	runtime.KeepAlive(wtime)
	if err == windows.STATUS_SUCCESS {
		err = nil
	}
	if err != nil {
		return errors.Wrap(err, "set filetime")
	}
	return nil
}
