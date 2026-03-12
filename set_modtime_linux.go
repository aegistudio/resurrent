package resurrent

import (
	"os"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

func setModtime(f *os.File, t time.Time) error {
	if err := unix.Futimes(
		int(f.Fd()),
		[]unix.Timeval{
			// Access time
			{
				Usec: unix.UTIME_OMIT,
			},
			// Modify time
			unix.NsecToTimeval(t.UnixNano()),
		},
	); err != nil {
		return errors.Wrap(err, "futimes file")
	}
	return nil
}
