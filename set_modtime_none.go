//go:build !windows && !linux

package resurrent

import (
	"os"
	"time"
)

func setModtime(f *os.File, t time.Time) error {
	return nil
}
