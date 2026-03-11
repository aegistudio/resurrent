package migrate

import (
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	utilYAML "github.com/aegistudio/resurrent/util/yaml"
	"github.com/pkg/errors"
)

func migrateV1HashBuckets(ctx *migrateContext) error {
	success := false

	// For every object of path
	//              1  2  3  4  5  6  7  8  9 a  b c  d e  f10
	// fsroot/objs/01/23/45/67/89/ab/cd/ef/0123/4567/89ab/cdef
	//
	// We put it into the bucket file
	//                 1  2  3
	// fsroot/objs-v2/01/23/45
	//
	// And the modification must subsume to the hash of file.
	// Since an object file is small enough, running the
	// conversion in parallel threads will not help. But this
	// simplify our implementation.
	objsPath := filepath.Join(ctx.root, "objs")
	objsV2Path := filepath.Join(ctx.root, "objs-v2")
	if err := os.MkdirAll(objsV2Path, localDirMode); err != nil {
		return errors.Wrap(err, "create objs-v2 directory")
	}
	defer func() {
		if !success {
			_ = os.RemoveAll(objsV2Path)
		}
	}()

	if err := filepath.Walk(objsPath, func(
		path string, info fs.FileInfo, err error,
	) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		relPath, err := filepath.Rel(objsPath, path)
		if err != nil {
			return errors.Wrapf(
				err, "evaluate relpath %q", path,
			)
		}
		relPath = filepath.Clean(relPath)
		relPath = filepath.ToSlash(relPath)
		objID := strings.ReplaceAll(relPath, "/", "")
		objIDBytes, err := hex.DecodeString(objID)
		if err != nil {
			return errors.Wrapf(
				err, "decode object ID %q", objID,
			)
		}
		// 32 bytes = 256 bits, for sha256.
		if objIDLen := len(objIDBytes); objIDLen != 32 {
			return errors.Wrapf(
				errors.Errorf("invalid object ID length %d", objIDLen),
				"decode object ID %q", objID,
			)
		}
		objID = hex.EncodeToString(objIDBytes)

		// Read and parse the source object first.
		objBytes, err := os.ReadFile(path)
		if err != nil {
			return errors.Wrapf(err, "read object %q", objID)
		}
		var obj utilYAML.Raw
		if err := utilYAML.LoadYAML(objBytes, &obj); err != nil {
			return errors.Wrapf(err, "parse object %q", objID)
		}

		// Fetch and create the bucket format then.
		bucketDir := filepath.Join(
			objsV2Path, objID[0:2], objID[2:4],
		)
		if err := os.MkdirAll(bucketDir, localDirMode); err != nil {
			return errors.Wrapf(
				err, "create bucket dir %q", bucketDir,
			)
		}

		var bucket struct {
			Version int                        `yaml:"version"`
			Objects []map[string]*utilYAML.Raw `yaml:"objs"`
		}
		bucketPath := filepath.Join(bucketDir, objID[4:6])
		bucketBytes, err := os.ReadFile(bucketPath)
		if os.IsNotExist(err) {
			err = nil
		}
		if err != nil {
			return errors.Wrapf(
				err, "read bucket %q", bucketPath,
			)
		}
		if bucketBytes != nil {
			err = utilYAML.LoadYAML(bucketBytes, &bucket)
			if err != nil {
				return errors.Wrapf(
					err, "parse bucket %q", bucketPath,
				)
			}
		}
		bucket.Version = 1
		objEntry := make(map[string]*utilYAML.Raw)
		objEntry[objID] = &obj
		bucket.Objects = append(bucket.Objects, objEntry)

		bucketBytes, err = utilYAML.SaveYAML(&bucket)
		if err != nil {
			return errors.Wrapf(
				err, "save bucket %q", bucketPath,
			)
		}
		if err := os.WriteFile(
			bucketPath, bucketBytes, localFileMode,
		); err != nil {
			return errors.Wrapf(
				err, "write bucket %q", bucketPath,
			)
		}
		return nil
	}); err != nil {
		return errors.Wrapf(err, "walk objects")
	}

	// Remove the old objs directory and replace
	// it with the one of ours.
	if err := os.RemoveAll(objsPath); err != nil {
		return errors.Wrap(err, "remove objs-v1")
	}
	if err := os.Rename(objsV2Path, objsPath); err != nil {
		return errors.Wrap(err, "rename objs-v2")
	}
	success = true
	return nil
}
