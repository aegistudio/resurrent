package resurrent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"

	"github.com/aegistudio/resurrent/format"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/winfsp/go-winfsp/treelock"
	"golang.org/x/sync/errgroup"
)

type objectLocker struct {
	mtx     sync.Mutex
	waiters map[string]chan struct{}
}

type objectLock struct {
	once sync.Once
	ol   *objectLocker
	id   string
}

func newObjectLocker() *objectLocker {
	return &objectLocker{
		waiters: make(map[string]chan struct{}),
	}
}

func (ol *objectLocker) lock(id string) *objectLock {
	for {
		waitCh := func() chan struct{} {
			ol.mtx.Lock()
			defer ol.mtx.Unlock()
			waitCh, ok := ol.waiters[id]
			if ok {
				return waitCh
			}
			ol.waiters[id] = make(chan struct{})
			return nil
		}()
		if waitCh != nil {
			<-waitCh
			continue
		}
		return &objectLock{
			ol: ol,
			id: id,
		}
	}
}

func (lock *objectLock) unlock() {
	runtime.SetFinalizer(lock, nil)
	lock.once.Do(func() {
		ol, id := lock.ol, lock.id
		ol.mtx.Lock()
		defer ol.mtx.Unlock()
		close(ol.waiters[id])
		delete(ol.waiters, id)
	})
}

// filter is a layer of filter.
type filter interface {
	serialize() (string, error)

	encode(w io.Writer, r io.Reader) error

	decode(w io.Writer, r io.Reader) error
}

type filterFactory func(spec *url.URL) (filter, error)

var (
	filterRegistry = make(map[string]filterFactory)
)

func registerFilter(name string, f filterFactory) {
	filterRegistry[name] = f
}

type filterChain struct {
	filters []filter
}

func (fc *filterChain) decode(
	rootCtx context.Context,
	w io.WriteCloser, r io.ReadCloser,
) error {
	grp, ctx := errgroup.WithContext(rootCtx)
	_ = ctx
	lastReader := r
	for i := len(fc.filters); i > 0; i-- {
		filter := fc.filters[i-1]
		pipeReader, pipeWriter := io.Pipe()
		currentReader := lastReader
		grp.Go(func() error {
			defer pipeWriter.Close()
			defer currentReader.Close()
			return filter.decode(pipeWriter, currentReader)
		})
		lastReader = pipeReader
	}
	grp.Go(func() error {
		defer w.Close()
		defer lastReader.Close()
		_, err := io.Copy(w, lastReader)
		return err
	})
	return grp.Wait()
}

func (fc *filterChain) encode(
	rootCtx context.Context,
	w io.WriteCloser, r io.ReadCloser,
) error {
	grp, ctx := errgroup.WithContext(rootCtx)
	_ = ctx
	lastReader := r
	for i := 0; i < len(fc.filters); i++ {
		filter := fc.filters[i]
		pipeReader, pipeWriter := io.Pipe()
		currentReader := lastReader
		grp.Go(func() error {
			defer pipeWriter.Close()
			defer currentReader.Close()
			return filter.encode(pipeWriter, currentReader)
		})
		lastReader = pipeReader
	}
	grp.Go(func() error {
		defer w.Close()
		defer lastReader.Close()
		_, err := io.Copy(w, lastReader)
		return err
	})
	return grp.Wait()
}

func parseFilters(filterSpecs []string) (*filterChain, error) {
	// XXX: we don't plan to support extensible
	// filters, since the filter choosing logic
	// has been tied to the process of upload.
	var result []filter
	for _, filterSpec := range filterSpecs {
		filterURI, err := url.Parse(filterSpec)
		if err != nil {
			return nil, errors.Wrapf(
				err, "parse filter %q URI", filterSpec,
			)
		}
		filterType := filterURI.Scheme
		filterFactory, ok := filterRegistry[filterType]
		if !ok {
			return nil, errors.Wrapf(
				err, "unknown filter type %q", filterType,
			)
		}
		filter, err := filterFactory(filterURI)
		if err != nil {
			return nil, errors.Wrapf(
				err, "parse filter %q", filterSpec,
			)
		}
		result = append(result, filter)
	}
	return &filterChain{result}, nil
}

// Engine of this object storage.
type Engine interface {
	// Upload the file to this object storage.
	//
	// If the file has already been uploaded to this
	// storage before, the previous upload will also
	// be passed, so that the engine can determine,
	// whether it should reuse the stored one, or
	// create a new one. The path to the created
	// object will be returned.
	Upload(
		ctx context.Context, file string,
		oldObjectURI *url.URL,
	) (objectURI *url.URL, err error)

	// Download the file from this object storage.
	Download(
		ctx context.Context, file string,
		objectURI *url.URL,
	) error
}

// EngineFactory creates the object storage engine.
//
// Specially, it is allowed to return nil, nil, so
// the the filesystem will skip loading this engine.
// It is useful when the filesystem should be used
// across multiple operating systems, and want to
// disable on the unsupported ones.
type EngineFactory func(format.Engine) (Engine, error)

var engineFactories sync.Map

func RegisterEngine(name string, f EngineFactory) {
	if _, loaded := engineFactories.LoadOrStore(name, f); loaded {
		panic(fmt.Sprintf("engine %q already exists", name))
	}
}

type downloadKey struct {
	dst string
	obj string
}

type downloadTask struct {
	err    error
	doneCh chan struct{}
}

func (fs *FS) pathForObject(obj string) (p string, err error) {
	bs, err := hex.DecodeString(obj)
	if err != nil {
		return "", err
	}
	// SHA256 has 32 bytes.
	if len(bs) != 32 {
		return "", err
	}
	var components []string
	components = append(components, fs.root, "objs")
	for range 8 {
		current := bs[:1]
		bs = bs[1:]
		components = append(
			components, hex.EncodeToString(current),
		)
	}
	for len(bs) > 0 {
		current := bs[:2]
		bs = bs[2:]
		components = append(
			components, hex.EncodeToString(current),
		)
	}
	return filepath.Join(components...), nil
}

func (fs *FS) downloadAndSetupFile(
	filterChain *filterChain,
	localPath string, rc io.ReadCloser,
	fileMeta *format.FileMeta,
) error {
	f, err := os.OpenFile(
		localPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, localFileMode,
	)
	if err != nil {
		return errors.Wrapf(err, "create file %q", localPath)
	}
	defer func() { _ = f.Close() }()

	if err := filterChain.decode(fs.ctx, f, rc); err != nil {
		return errors.Wrapf(err, "download file %q", localPath)
	}

	f2, err := os.OpenFile(localPath, os.O_RDWR, 0)
	if err != nil {
		return errors.Wrapf(err, "open file %q", localPath)
	}
	defer func() { _ = f2.Close() }()

	if t := fileMeta.ModifiedAt; !t.IsZero() {
		if err := setModtime(f2, t); err != nil {
			return errors.Wrapf(err, "set modtime %q", localPath)
		}
	}
	return nil
}

func (fs *FS) runDownloadTask(
	dst string, fileMeta *format.FileMeta,
) error {
	obj := fileMeta.Object
	lock := fs.ol.lock(obj)
	defer lock.unlock()

	var err error
	objPath, err := fs.pathForObject(obj)
	if err != nil {
		return errors.Wrapf(err, "path for object %q", obj)
	}
	objData, err := os.ReadFile(objPath)
	if err != nil {
		return errors.Wrapf(err, "read object %q", obj)
	}
	objMeta, err := format.LoadObject(objData)
	if err != nil {
		return errors.Wrapf(err, "load object %q", obj)
	}
	filterChain, err := parseFilters(objMeta.Filters)
	if err != nil {
		return errors.Wrapf(err, "parse filters %q", obj)
	}

	// Fast path: if there's only one storage that is
	// base64, then we can decode it without creating
	// the temporary download directory and file.
	base64URI, err := func() (*url.URL, error) {
		if len(objMeta.Storages) != 1 {
			return nil, nil
		}
		item := objMeta.Storages[0]
		parsedURL, err := url.Parse(item)
		if err != nil {
			return nil, errors.Wrapf(
				err, "parse storage %q", item,
			)
		}
		if parsedURL.Scheme != "base64" {
			return nil, nil
		}
		return parsedURL, nil
	}()
	if err != nil {
		return err
	}
	if base64URI != nil {
		val, err := url.PathUnescape(base64URI.Opaque)
		if err != nil {
			return errors.Wrapf(
				err, "path unescape %q", base64URI.String(),
			)
		}
		inlineData, err := base64.URLEncoding.DecodeString(val)
		if err != nil {
			return errors.Wrapf(
				err, "decode inline data %q", val,
			)
		}
		inlineReader := io.NopCloser(bytes.NewReader(inlineData))
		if err := fs.downloadAndSetupFile(
			filterChain, dst, inlineReader, fileMeta,
		); err != nil {
			return errors.Wrapf(
				err, "download and setup %q", dst,
			)
		}
		return nil
	}

	// Slow path: now we have to create the download
	// directory for object transfer.
	workDir, err := os.MkdirTemp(
		filepath.Join(fs.root, "work"),
		"download-*",
	)
	if err != nil {
		return errors.Wrapf(err, "create workdir object %q", obj)
	}
	defer func() { _ = os.RemoveAll(workDir) }()
	remotePath := filepath.Join(workDir, "remote")
	localPath := filepath.Join(workDir, "local")

	// Download files from object storage first.
	var errs []error
	for _, storage := range objMeta.Storages {
		err := func() error {
			var err error
			_ = os.Remove(remotePath)
			_ = os.Remove(localPath)
			storageURI, err := url.Parse(storage)
			if err != nil {
				return errors.Wrapf(err, "parse storage %q", storage)
			}
			engineName := storageURI.Scheme
			engine, ok := fs.engines[engineName]
			if !ok {
				return errors.Errorf(
					"engine %q not found or unsupported", engineName,
				)
			}
			err = engine.Download(fs.ctx, remotePath, storageURI)
			if err != nil {
				return errors.Wrapf(err, "download %q", storage)
			}
			remoteFile, err := os.Open(remotePath)
			if err != nil {
				return errors.Wrapf(err, "open remote %q", remotePath)
			}
			defer func() { _ = remoteFile.Close() }()

			if err := fs.downloadAndSetupFile(
				filterChain, localPath, remoteFile, fileMeta,
			); err != nil {
				return errors.Wrapf(
					err, "download and setup %q", dst,
				)
			}
			return nil
		}()
		errs = append(errs, err)
		if err == nil {
			break
		}
	}
	if len(errs) == 0 {
		return errors.Errorf("no supported storage for object %q", obj)
	}
	if errs[len(errs)-1] != nil {
		return multierror.Append(
			errors.Errorf(
				"all download attempts failed for object %q", obj,
			), errs...,
		)
	}

	// Move the decoded file to the destination.
	if err := os.Rename(localPath, dst); err != nil {
		return errors.Wrap(err, "move download result")
	}
	return nil
}

func (fs *FS) findOrNewDownloadTask(
	dst string, fileMeta *format.FileMeta,
) *downloadTask {
	obj := fileMeta.Object
	if fileMeta.Type != format.TypeFile {
		panic("invalid non-file to download")
	}
	if obj == "" {
		panic("Invalid file with no object to download")
	}
	key := downloadKey{
		dst: dst,
		obj: obj,
	}
	fs.downloadMtx.Lock()
	defer fs.downloadMtx.Unlock()
	if task, ok := fs.downloadTasks[key]; ok {
		return task
	} else {
		task := &downloadTask{
			doneCh: make(chan struct{}),
		}
		fs.downloadTasks[key] = task
		fs.grp.Go(func() error {
			defer func() {
				fs.downloadMtx.Lock()
				defer fs.downloadMtx.Unlock()
				delete(fs.downloadTasks, key)
			}()
			defer close(task.doneCh)
			noPanic := false
			defer func() {
				if !noPanic && task.err == nil {
					task.err = syscall.EACCES
				}
			}()
			task.err = fs.runDownloadTask(dst, fileMeta)
			noPanic = true
			return nil
		})
		return task
	}
}

// Download the file if it is not present locally.
//
// The file will not be downloaded if it has already
// been present, or has no remote object. It will not
// be downloaded if any other object is modifying it.
func (fs *FS) Download(p string) error {
	p = fs.cleanFilterPath(p)
	var plock *treelock.PathLock
	defer func() {
		if plock != nil {
			plock.Unlock()
		}
	}()
	task, err := func() (*downloadTask, error) {
		fs.mtx.Lock()
		defer fs.mtx.Unlock()

		stat, err := fs.statClean(p)
		if err != nil {
			return nil, errors.Wrapf(err, "stat file %q", p)
		}
		if stat.Meta.Type != format.TypeFile {
			// Only file can be downloaded.
			return nil, nil
		}
		if stat.Present() {
			// Those with present file will not
			// need to be downloaded.
			return nil, nil
		}
		if stat.Meta.Object == "" {
			// No remote object to download.
			return nil, nil
		}

		writeLock := fs.tl.TryWLockFile(p)
		if writeLock == nil {
			return nil, syscall.EACCES
		}
		defer func() {
			if writeLock != nil {
				writeLock.Unlock()
			}
		}()

		dir, base := fs.splitAndClean(p)
		fileDataPath := fs.evaluateDataPath(
			dir, base, format.TypeFile,
		)
		task := fs.findOrNewDownloadTask(
			fileDataPath, &stat.Meta,
		)
		plock, writeLock = writeLock, nil
		return task, nil
	}()
	if err != nil {
		return err
	}
	if task != nil {
		<-task.doneCh
		return task.err
	}
	return nil
}

type bytesBufferCloser struct {
	bytes.Buffer
}

func (bbc *bytesBufferCloser) Close() error {
	return nil
}

func (fs *FS) runUploadTask(src, engineName string) error {
	engine, ok := fs.engines[engineName]
	if !ok {
		return errors.Errorf("engine %q not found", engineName)
	}
	fileMeta, err := fs.statClean(src)
	if err != nil {
		return err
	}
	if fileMeta.Meta.Type != format.TypeFile {
		// No need to upload unless file.
		return nil
	}
	if !fileMeta.Present() {
		// No need to upload if not present locally.
		return nil
	}

	dir, base := fs.splitAndClean(src)
	if base == "" {
		panic("invalid empty base")
	}
	size := fileMeta.Size()
	dataPath := fs.evaluateDataPath(dir, base, format.TypeFile)

	// Determine the object ID to upload first.
	var (
		nonce uint64
		lock  *objectLock
	)
	defer func() {
		if lock != nil {
			lock.unlock()
		}
	}()
	for {
		if err := func() error {
			var err error
			// Compute the hash of object first.
			hasher := sha256.New()
			var nonceBuffer [8]byte
			binary.BigEndian.PutUint64(nonceBuffer[:], nonce)
			if _, err := hasher.Write(nonceBuffer[:]); err != nil {
				return errors.Wrap(err, "write nonce for hash")
			}
			f, err := os.Open(dataPath)
			if err != nil {
				return errors.Wrap(err, "open data file")
			}
			defer func() { _ = f.Close() }()
			if _, err := io.Copy(hasher, f); err != nil {
				return errors.Wrap(err, "write file for hash")
			}
			hash := hasher.Sum(nil)
			obj := hex.EncodeToString(hash)

			// Find out if there's a object then.
			candidateLock := fs.ol.lock(obj)
			defer func() {
				if candidateLock != nil {
					candidateLock.unlock()
				}
			}()
			objPath, err := fs.pathForObject(obj)
			if err != nil {
				return errors.Wrapf(err, "path for object %q", obj)
			}
			v, err := os.ReadFile(objPath)
			if os.IsNotExist(err) {
				// If the corresponding object file does not
				// exist, we will occupy this ID.
				lock, candidateLock = candidateLock, nil
				return nil
			}
			if err != nil {
				return errors.Wrapf(err, "read object %q", obj)
			}
			objMeta, err := format.LoadObject(v)
			if err != nil {
				return errors.Wrapf(err, "load object %q", obj)
			}
			if objMeta.Nonce != nonce {
				return nil
			}
			if objMeta.Size != size {
				return nil
			}
			lock, candidateLock = candidateLock, nil
			return nil
		}(); err != nil {
			return errors.Wrap(err, "compute object")
		}
		if lock != nil {
			break
		}
		if nonce == math.MaxUint64 {
			panic("impossible nonce overflow")
		}
		nonce += 1
	}

	// We now schedule the next upload.
	obj := lock.id
	objMeta, err := func() (*format.Object, error) {
		objPath, err := fs.pathForObject(obj)
		if err != nil {
			return nil, errors.Wrapf(err, "path for object %q", obj)
		}
		v, err := os.ReadFile(objPath)
		if os.IsNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, errors.Wrapf(err, "read object %q", obj)
		}
		objMeta, err := format.LoadObject(v)
		if err != nil {
			return nil, errors.Wrapf(err, "load object %q", obj)
		}
		return objMeta, nil
	}()
	if err != nil {
		return err
	}
	if objMeta == nil {
		objMeta = &format.Object{
			Version: format.CurrentVersion,
			Size:    size,
			Nonce:   nonce,
		}
	}

	// Create directory for this upload task.
	workDir, err := os.MkdirTemp(
		filepath.Join(fs.root, "work"), "upload-*",
	)
	if err != nil {
		return errors.Wrap(err, "create work dir")
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	// If there has not been any storage, then we
	// will try to figure out the filters, or we
	// will just apply the filter by engine.
	if len(objMeta.Storages) == 0 {
		objMeta.Filters = nil
		contentSize := size
		if err := func() error {
			srcFile, err := os.Open(dataPath)
			if err != nil {
				return errors.Wrap(err, "open data file")
			}
			defer func() { _ = srcFile.Close() }()
			dstPath := filepath.Join(workDir, "flate-test")
			dstFile, err := os.OpenFile(
				dstPath,
				os.O_WRONLY|os.O_CREATE|os.O_EXCL, localFileMode,
			)
			if err != nil {
				return errors.Wrap(err, "create flate test file")
			}
			defer func() { _ = dstFile.Close() }()

			fc := &filterChain{
				filters: []filter{
					&filterFlate{},
				},
			}
			if err := fc.encode(fs.ctx, dstFile, srcFile); err != nil {
				return errors.Wrap(err, "test flate file")
			}

			stat, err := os.Stat(dstPath)
			if err != nil {
				return errors.Wrap(err, "stat flate file")
			}
			if contentSize > stat.Size() {
				flateFilter, err := (&filterFlate{}).serialize()
				if err != nil {
					return errors.Wrap(err, "serialize flate")
				}
				objMeta.Filters = append(objMeta.Filters, flateFilter)
				contentSize = stat.Size()
			}
			return nil
		}(); err != nil {
			return err
		}
		if contentSize <= fs.inlineMaxSize {
			fc, err := parseFilters(objMeta.Filters)
			if err != nil {
				return errors.Wrap(err, "parse filter chain")
			}
			origContent, err := os.ReadFile(dataPath)
			if err != nil {
				return errors.Wrap(err, "read data file")
			}
			var w bytesBufferCloser
			if err := fc.encode(
				fs.ctx, &w,
				io.NopCloser(bytes.NewReader(origContent)),
			); err != nil {
				return errors.Wrap(err, "inline content")
			}
			inlineContent := w.Bytes()
			var inlineURI url.URL
			inlineURI.Scheme = "base64"
			content := base64.URLEncoding.EncodeToString(inlineContent)
			inlineURI.Opaque = url.PathEscape(content)
			objMeta.Storages = append(objMeta.Storages, inlineURI.String())
		} else {
			filterObject, err := randomFilterSioAES256GCM()
			if err != nil {
				return errors.Wrap(err, "create random encrypt filter")
			}
			encryptFilter, err := filterObject.serialize()
			if err != nil {
				return errors.Wrap(err, "serialize encrypt")
			}
			objMeta.Filters = append(objMeta.Filters, encryptFilter)
		}
	}

	storages := make(map[string]*url.URL)
	for _, storageItem := range objMeta.Storages {
		objectURI, err := url.Parse(storageItem)
		if err != nil {
			return errors.Wrapf(err, "parse object URI %q", storageItem)
		}
		storages[objectURI.Scheme] = objectURI
	}

	_, inlineData := storages["base64"]
	if !inlineData {
		fc, err := parseFilters(objMeta.Filters)
		if err != nil {
			return errors.Wrapf(err, "parse filter chain")
		}
		encodedPath := filepath.Join(workDir, "encoded")
		if err := func() error {
			dataFile, err := os.Open(dataPath)
			if err != nil {
				return errors.Wrap(err, "open data file")
			}
			defer func() { _ = dataFile.Close() }()
			encodedFile, err := os.OpenFile(
				encodedPath,
				os.O_WRONLY|os.O_CREATE|os.O_EXCL, localFileMode,
			)
			if err != nil {
				return errors.Wrap(err, "create encoded file")
			}
			defer func() { _ = encodedFile.Close() }()
			if err := fc.encode(
				fs.ctx, encodedFile, dataFile,
			); err != nil {
				return errors.Wrap(err, "encode file")
			}
			return nil
		}(); err != nil {
			return err
		}
		oldObjectURI, _ := storages[engineName]
		newObjectURI, err := engine.Upload(
			fs.ctx, encodedPath, oldObjectURI,
		)
		if err != nil {
			return errors.Wrapf(err, "upload file %q", src)
		}
		newObjectURI.Scheme = engineName
		storages[engineName] = newObjectURI
	}

	objMeta.Storages = nil
	for _, storage := range storages {
		objMeta.Storages = append(objMeta.Storages, storage.String())
	}

	objPath, err := fs.pathForObject(obj)
	if err != nil {
		return errors.Wrapf(err, "encode object path %q", obj)
	}
	newObjData, err := objMeta.Save()
	if err != nil {
		return errors.Wrap(err, "encode object data")
	}
	if err := os.MkdirAll(
		filepath.Dir(objPath), localDirMode,
	); err != nil {
		return errors.Wrapf(err, "create object dir %q", obj)
	}
	if err := os.WriteFile(
		objPath, newObjData, localFileMode,
	); err != nil {
		return errors.Wrapf(err, "write object data %q", obj)
	}
	meta := fileMeta.Meta
	meta.Object = obj
	meta.Size = size
	meta.ModifiedAt = fileMeta.ModTime()
	fileMetaData, err := meta.Save()
	if err != nil {
		return errors.Wrapf(err, "encode file metadata")
	}
	fileMetaPath := fs.evaluateMetaPath(dir, base)
	if err := os.WriteFile(
		fileMetaPath, fileMetaData, localFileMode,
	); err != nil {
		return errors.Wrapf(err, "save file metadata")
	}

	return nil
}

func (fs *FS) Upload(p, engineName string) error {
	p = fs.cleanFilterPath(p)
	var uploadLock *treelock.PathLock
	defer func() {
		if uploadLock != nil {
			uploadLock.Unlock()
		}
	}()
	type uploadTask struct {
		doneCh chan struct{}
		err    error
	}
	task, err := func() (*uploadTask, error) {
		fs.mtx.Lock()
		defer fs.mtx.Unlock()

		stat, err := fs.statClean(p)
		if err != nil {
			return nil, err
		}
		if stat.Meta.Type != format.TypeFile {
			return nil, nil
		}

		plock := fs.tl.TryWLockFile(p)
		if plock == nil {
			return nil, syscall.EACCES
		}
		defer func() {
			if plock != nil {
				plock.Unlock()
			}
		}()
		if plock.CurrentRefs() != 1 {
			return nil, syscall.EACCES
		}

		task := &uploadTask{
			doneCh: make(chan struct{}),
		}
		fs.grp.Go(func() error {
			defer close(task.doneCh)
			noPanic := false
			defer func() {
				if !noPanic && task.err == nil {
					task.err = syscall.EACCES
				}
			}()
			task.err = fs.runUploadTask(p, engineName)
			noPanic = true
			return nil
		})
		return task, nil
	}()
	if err != nil {
		return err
	}
	if task != nil {
		<-task.doneCh
		return task.err
	}
	return nil
}

// Evict will attempt to evict the file from disk.
//
// The term "evict" stems from viewing the local
// resurrent filesystem as a cache of the remote
// object storage. In this case, removing the local
// file is just like evicting a cache line.
//
// To evict a file, the file must have already been
// stored in one of the objects. The file must not
// be open or in use. The current data of the file
// must match the one (size and hash) in object storage.
func (fs *FS) Evict(p string) error {
	p = fs.cleanFilterPath(p)
	fs.mtx.Lock()
	defer fs.mtx.Unlock()

	stat, err := fs.statClean(p)
	if err != nil {
		return errors.Wrapf(err, "stat %q", p)
	}
	if stat.Meta.Type != format.TypeFile {
		// Cannot evict non-regular file.
		return nil
	}
	if !stat.Present() {
		// No need to do anything if not present.
		return nil
	}
	obj := stat.Meta.Object
	if obj == "" {
		// No need to do anything if no object.
		return nil
	}
	if !stat.Meta.ModifiedAt.Equal(stat.ModTime()) {
		// No need to do anything if file is modified.
		return nil
	}

	// We must be the only one holding the file.
	plock := fs.tl.TryWLockFile(p)
	if plock == nil {
		return syscall.EACCES
	}
	defer plock.Unlock()
	if plock.CurrentRefs() != 1 {
		return syscall.EACCES
	}

	// Now lock the object.
	olock := fs.ol.lock(obj)
	defer olock.unlock()

	// Load and compare the object.
	objPath, err := fs.pathForObject(obj)
	if err != nil {
		return errors.Wrapf(err, "path for object %q", obj)
	}
	objData, err := os.ReadFile(objPath)
	if err != nil {
		return errors.Wrapf(err, "read object %q", obj)
	}
	objMeta, err := format.LoadObject(objData)
	if err != nil {
		return errors.Wrapf(err, "load object %q", obj)
	}

	// Evaluate and compare hash.
	hasher := sha256.New()
	var nonceBuffer [8]byte
	binary.BigEndian.PutUint64(nonceBuffer[:], objMeta.Nonce)
	if _, err := hasher.Write(nonceBuffer[:]); err != nil {
		return errors.Wrap(err, "write nonce for hash")
	}
	dir, base := fs.splitAndClean(p)
	dataPath := fs.evaluateDataPath(dir, base, format.TypeFile)
	if err := func() error {
		f, err := os.Open(dataPath)
		if err != nil {
			return errors.Wrap(err, "open data file")
		}
		defer func() { _ = f.Close() }()
		if _, err := io.Copy(hasher, f); err != nil {
			return errors.Wrap(err, "write file for hash")
		}
		return nil
	}(); err != nil {
		return err
	}
	hash := hasher.Sum(nil)
	if hex.EncodeToString(hash) != obj {
		// Hash mismatch, will need to upload.
		return nil
	}

	// Remove the file on disk.
	if err := os.Remove(dataPath); err != nil {
		return errors.Wrapf(err, "remove local file %q", p)
	}
	return nil
}
