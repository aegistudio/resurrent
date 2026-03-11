package migrate

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"

	"github.com/pkg/errors"

	"github.com/aegistudio/resurrent"
)

var (
	localFileMode = os.FileMode(0o644)
	localDirMode  = os.ModeDir | os.FileMode(0o755)
)

type migrateContext struct {
	ctx  context.Context
	root string
}

type migrateFunc func(ctx *migrateContext) error

var registries []migrateFunc = []migrateFunc{
	// v0 -> v1: migrate hash buckets.
	migrateV1HashBuckets,
}

type option struct {
	maxVersion uint64
}

type Option func(*option) error

func MaxVersion(v uint64) Option {
	return func(o *option) error {
		o.maxVersion = v
		return nil
	}
}

func Migrate(
	ctx context.Context, root string,
	opts ...Option,
) error {
	fsl, err := resurrent.LockFS(root)
	if err != nil {
		return errors.Wrap(err, "lock fs")
	}
	defer fsl.Unlock()

	root = fsl.RootPath()
	migrateCtx := &migrateContext{
		ctx:  ctx,
		root: root,
	}

	var o option
	for _, opt := range opts {
		if err := opt(&o); err != nil {
			return err
		}
	}
	if o.maxVersion == 0 {
		o.maxVersion = uint64(len(registries))
	}
	o.maxVersion = min(o.maxVersion, uint64(len(registries)))

	versionPath := filepath.Join(root, "resurrent.ver")
	for {
		versionBytes, err := os.ReadFile(versionPath)
		if os.IsNotExist(err) {
			err = nil
		}
		if err != nil {
			return errors.Wrap(err, "read version file")
		}
		var ver uint64
		if len(versionBytes) > 0 {
			v, err := strconv.ParseUint(string(versionBytes), 10, 64)
			if err != nil {
				return errors.Wrap(err, "parse version file")
			}
			ver = v
		}
		if ver >= o.maxVersion {
			break
		}
		migrator := registries[ver]
		pc := reflect.ValueOf(migrator).Pointer()
		fun := runtime.FuncForPC(pc)
		log.Printf("Migrating v%d -> v%d: %s", ver, ver+1, fun.Name())
		if err := migrator(migrateCtx); err != nil {
			return errors.Wrapf(
				err, "migrate from version %d", ver,
			)
		}
		newVer := strconv.FormatUint(ver+1, 10)
		if err := os.WriteFile(
			versionPath, []byte(newVer), localFileMode,
		); err != nil {
			return errors.Wrap(err, "write version file")
		}
	}
	return nil
}
