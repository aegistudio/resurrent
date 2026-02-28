// Package host defines the object storage engine that
// stores large files at specified path on host.
package host

import (
	"context"
	"encoding/hex"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/aegistudio/resurrent"
	"github.com/aegistudio/resurrent/format"
)

const EngineName = "host"

type Host struct {
	root string
}

func (o *Host) pathForObject(name string) (string, error) {
	id, err := uuid.Parse(name)
	if err != nil {
		return "", errors.Wrap(err, "parse uuid")
	}
	bs := []byte(id[:])
	var result []string
	result = append(result, o.root)
	for range 8 {
		current := bs[:1]
		bs = bs[1:]
		result = append(result, hex.EncodeToString(current))
	}
	for len(bs) > 0 {
		current := bs[:2]
		bs = bs[2:]
		result = append(result, hex.EncodeToString(current))
	}
	return filepath.Join(result...), nil
}

func (o *Host) openObject(
	objectURI *url.URL, flags int, perm os.FileMode,
) (*os.File, error) {
	name, err := url.PathUnescape(objectURI.Opaque)
	if err != nil {
		return nil, errors.Wrap(err, "unescape path")
	}
	remotePath, err := o.pathForObject(name)
	if err != nil {
		return nil, errors.Wrap(err, "evaluate path")
	}
	if flags&os.O_CREATE != 0 {
		if err := os.MkdirAll(
			filepath.Dir(remotePath),
			os.ModeDir|os.FileMode(0755),
		); err != nil {
			return nil, errors.Wrap(err, "create dirs")
		}
	}
	f, err := os.OpenFile(remotePath, flags, perm)
	if err != nil {
		return nil, errors.Wrap(err, "open remote file")
	}
	return f, nil
}

func (o *Host) Download(
	ctx context.Context, file string, objectURI *url.URL,
) error {
	dst, err := os.Create(file)
	if err != nil {
		return errors.Wrap(err, "create target file")
	}
	defer func() { _ = dst.Close() }()

	src, err := o.openObject(objectURI, os.O_RDONLY, 0)
	if err != nil {
		return errors.Wrap(err, "open source file")
	}
	defer func() { _ = src.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		return errors.Wrap(err, "transfer file")
	}
	return nil
}

func (o *Host) Upload(
	ctx context.Context, file string, oldObjectURI *url.URL,
) (objectURI *url.URL, err error) {
	if oldObjectURI != nil {
		resultURI, err := func() (*url.URL, error) {
			remoteFile, err := o.openObject(oldObjectURI, os.O_RDONLY, 0)
			if err != nil {
				return nil, errors.Wrap(err, "open remote file")
			}
			defer func() { _ = remoteFile.Close() }()

			localFile, err := os.Open(file)
			if err != nil {
				return nil, errors.Wrap(err, "open local file")
			}
			defer func() { _ = localFile.Close() }()

			remoteStat, err := remoteFile.Stat()
			if err != nil {
				// TODO: log?
				return nil, errors.Wrap(err, "stat remote file")
			}
			localStat, err := localFile.Stat()
			if err != nil {
				return nil, errors.Wrap(err, "stat local file")
			}
			if remoteStat.Size() != localStat.Size() {
				return nil, nil
			}
			return oldObjectURI, nil
		}()
		if err != nil {
			return nil, err
		}
		if resultURI != nil {
			return resultURI, nil
		}
	}

	id, err := uuid.NewRandom()
	resultURI := &url.URL{}
	resultURI.Opaque = url.PathEscape(id.String())
	remoteFile, err := o.openObject(
		resultURI, os.O_CREATE|os.O_EXCL|os.O_WRONLY,
		os.FileMode(0o666),
	)
	if err != nil {
		return nil, errors.Wrap(err, "create remote file")
	}
	defer func() { _ = remoteFile.Close() }()

	localFile, err := os.Open(file)
	if err != nil {
		return nil, errors.Wrap(err, "open local file")
	}
	defer func() { _ = localFile.Close() }()

	if _, err := io.Copy(remoteFile, localFile); err != nil {
		return nil, errors.Wrap(err, "transfer file")
	}
	return resultURI, nil
}

var _ resurrent.Engine = (*Host)(nil)

type Config struct {
	Root           map[string]string `yaml:"root"`
	SkipIfNotFound bool              `yaml:"skip_if_not_found,omitempty"`
}

func init() {
	format.RegisterEngineFormat(EngineName, func() any {
		return &Config{
			Root: make(map[string]string),
		}
	})

	resurrent.RegisterEngine(EngineName, func(
		e format.Engine,
	) (resurrent.Engine, error) {
		cfg := e.Data.(*Config)
		root, ok := cfg.Root[runtime.GOOS]
		if !ok {
			return nil, nil
		}
		stat, err := os.Stat(root)
		if cfg.SkipIfNotFound && os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, errors.Wrapf(err, "root %q stat", root)
		}
		if !stat.IsDir() {
			return nil, errors.Errorf("root %q is not directory", root)
		}
		return &Host{root}, nil
	})
}
