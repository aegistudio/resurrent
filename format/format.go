package format

import (
	"github.com/pkg/errors"
)

func versionTooNew(ver int) error {
	return errors.Errorf(
		`version %d is too new, please install newer version of resurrent`,
		ver,
	)
}

var (
	// DefaultInlineMaxSize is 32kB.
	DefaultInlineMaxSize = 32 * 1024
)
