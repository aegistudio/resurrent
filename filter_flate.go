package resurrent

import (
	"compress/flate"
	"io"
	"net/url"

	"github.com/pkg/errors"
)

const (
	filterTypeFlate = "flate"
)

type filterFlate struct {
}

func (f *filterFlate) encode(w io.Writer, r io.Reader) error {
	wc, err := flate.NewWriter(w, flate.BestCompression)
	if err != nil {
		return errors.Wrap(err, "create compressor")
	}
	if _, err := io.Copy(wc, r); err != nil {
		return errors.Wrap(err, "compress data")
	}
	if err := wc.Close(); err != nil {
		return errors.Wrap(err, "close compressor")
	}
	return nil
}

func (f *filterFlate) decode(w io.Writer, r io.Reader) error {
	rc := flate.NewReader(r)
	if _, err := io.Copy(w, rc); err != nil {
		return errors.Wrap(err, "decompress data")
	}
	if err := rc.Close(); err != nil {
		return errors.Wrap(err, "close compressor")
	}
	return nil
}

func (f *filterFlate) serialize() (string, error) {
	var result url.URL
	result.Scheme = filterTypeFlate
	return result.String(), nil
}

var _ filter = (*filterFlate)(nil)

func init() {
	registerFilter(filterTypeFlate, func(
		spec *url.URL,
	) (filter, error) {
		return &filterFlate{}, nil
	})
}
