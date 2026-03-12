package resurrent

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gobwas/glob"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/pkg/errors"
	"github.com/winfsp/go-winfsp/treelock"
	"golang.org/x/sync/errgroup"

	"github.com/aegistudio/resurrent/format"
)

// FS is the operational filesystem.
type FS struct {
	once   sync.Once
	ctx    context.Context
	cancel context.CancelFunc
	grp    *errgroup.Group
	grpErr error

	mtx sync.RWMutex

	// tl is the treelock for mediating file
	// transfer operation and rename.
	//
	// It must only be try locked, after
	// obtaining the mutex.
	tl *treelock.TreeLocker

	fsl *FSLock

	// ol is the lock to synchronize objects.
	ol *objectLocker
	// bl is the lock to synchronize bucket.
	// Must first obtain object lock and then
	// the bucket lock.
	bl *objectLocker

	root          string
	caseSensitive bool
	inlineMaxSize int64

	engines map[string]Engine

	downloadOnDemand glob.Glob
	downloadMtx      sync.Mutex
	downloadTasks    map[downloadKey]*downloadTask

	prefixDirPathLRU *lru.Cache[string, string]
}

func (fs *FS) Close() error {
	fs.once.Do(func() {
		defer fs.fsl.Unlock()
		defer func() {
			_ = os.RemoveAll(filepath.Join(fs.root, "work"))
		}()
		fs.cancel()
		fs.grpErr = fs.grp.Wait()
	})
	return fs.grpErr
}

func (fs *FS) Done() <-chan struct{} {
	return fs.ctx.Done()
}

func (fs *FS) CaseSensitive() bool {
	return fs.caseSensitive
}

var (
	cleanRoot = filepath.FromSlash("/")

	localFileMode = os.FileMode(0o644)
	localDirMode  = os.ModeDir | os.FileMode(0o755)
)

func compileGlobs(globs []string, caseSensitive bool) (glob.Glob, error) {
	var patterns []string
	for _, pattern := range globs {
		pattern = strings.ReplaceAll(pattern, "{", `\{`)
		pattern = strings.ReplaceAll(pattern, "}", `\}`)
		pattern = strings.ReplaceAll(pattern, ",", `\,`)
		if !caseSensitive {
			pattern = strings.ToUpper(pattern)
		}
		shouldPrependDoubleStar := false
		if strings.HasPrefix(pattern, "/") {
			shouldPrependDoubleStar = false
		}
		if strings.HasPrefix(pattern, "**") {
			shouldPrependDoubleStar = false
		}
		if shouldPrependDoubleStar {
			pattern = "**/" + pattern
		}
		patterns = append(patterns, pattern)
	}
	return glob.Compile("{"+strings.Join(patterns, ",")+"}", '/')
}

const (
	CurrentFSVersion = 1
)

func verifyFSVersion(root string) error {
	var version uint64
	b, err := os.ReadFile(filepath.Join(root, "resurrent.ver"))
	if os.IsNotExist(err) {
		err = nil
	}
	if err != nil {
		return errors.Wrap(err, "read version file")
	}
	if len(b) > 0 {
		v, err := strconv.ParseUint(string(b), 10, 64)
		if err != nil {
			return errors.Wrap(err, "parse version file")
		}
		version = v
	}
	if version < CurrentFSVersion {
		return errors.Errorf(
			`version %d is too old, please run "resurrent migrate"`,
			version,
		)
	}
	if version > CurrentFSVersion {
		return errors.Errorf(
			`version %d is too new, please use newer version of resurrent`,
			version,
		)
	}
	return nil
}

func New(root string) (*FS, error) {
	var success bool
	cancelCtx, cancel := context.WithCancel(context.Background())
	grp, ctx := errgroup.WithContext(cancelCtx)
	defer func() {
		if !success {
			cancel()
			_ = grp.Wait()
		}
	}()

	fsl, err := LockFS(root)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !success {
			fsl.Unlock()
		}
	}()
	fsConfigData, err := os.ReadFile(
		filepath.Join(root, "resurrent.yaml"),
	)
	if err != nil {
		return nil, errors.Wrap(err, "read fs config")
	}
	fsConfig, err := format.LoadFSConfig(fsConfigData)
	if err != nil {
		return nil, errors.Wrap(err, "load fs config")
	}
	if err := verifyFSVersion(root); err != nil {
		return nil, errors.Wrap(err, "verify fs version")
	}

	downloadOnDemand, err := compileGlobs(
		fsConfig.DownloadOnDemand, fsConfig.CaseSensitive,
	)
	if err != nil {
		return nil, errors.Wrap(err, "compile downloadOnDemand patterns")
	}

	// Create the essential paths.
	if err := os.MkdirAll(
		filepath.Join(root, "files"), localDirMode,
	); err != nil {
		return nil, errors.Wrap(err, "mkdir files")
	}
	if err := os.MkdirAll(
		filepath.Join(root, "objs"), localDirMode,
	); err != nil {
		return nil, errors.Wrap(err, "mkdir objs")
	}
	rootDir := filepath.Join(root, "work")
	if err := os.MkdirAll(rootDir, localDirMode); err != nil {
		return nil, errors.Wrap(err, "mkdir work")
	}
	defer func() {
		if !success {
			_ = os.RemoveAll(rootDir)
		}
	}()

	// TODO: create engines.
	engines := make(map[string]Engine)
	for _, cfg := range fsConfig.Engines {
		if _, ok := engines[cfg.Name]; ok {
			return nil, errors.Errorf(
				"engine named %q already exists",
				cfg.Name,
			)
		}
		f, ok := engineFactories.Load(cfg.Type)
		if !ok {
			return nil, errors.Errorf(
				"unknown engine type %q", cfg.Type,
			)
		}
		engine, err := (f.(EngineFactory))(cfg)
		if err != nil {
			return nil, errors.Wrapf(
				err, "create engine %q", cfg.Name,
			)
		}
		if engine == nil {
			continue
		}
		engines[cfg.Name] = engine
	}

	// TODO: make cache size configurable.
	prefixDirPathLRU, err := lru.New[string, string](128)
	if err != nil {
		return nil, errors.Wrap(err, "create prefix dir path cache")
	}
	result := &FS{
		ctx:              ctx,
		cancel:           cancel,
		grp:              grp,
		tl:               treelock.New(),
		ol:               newObjectLocker(),
		bl:               newObjectLocker(),
		fsl:              fsl,
		root:             root,
		caseSensitive:    fsConfig.CaseSensitive,
		inlineMaxSize:    fsConfig.InlineMaxSize,
		engines:          engines,
		downloadOnDemand: downloadOnDemand,
		downloadTasks:    make(map[downloadKey]*downloadTask),
		prefixDirPathLRU: prefixDirPathLRU,
	}
	success = true
	return result, nil
}

func Init(root string, initFSConfig *format.FSConfig) error {
	var err error
	fs, err := LockFS(root)
	if err != nil {
		return err
	}
	defer fs.Unlock()

	fsConfigPath := filepath.Join(root, "resurrent.yaml")
	fsConfigData, err := os.ReadFile(fsConfigPath)
	if os.IsNotExist(err) {
		fsVersionPath := filepath.Join(root, "resurrent.ver")
		value := strconv.FormatUint(CurrentFSVersion, 10)
		if err1 := os.WriteFile(
			fsVersionPath, []byte(value), localFileMode,
		); err1 != nil {
			return errors.Wrap(err1, "init fs version")
		}
		err = nil
	}
	if err != nil {
		return errors.Wrap(err, "read fs config")
	}
	if err := verifyFSVersion(root); err != nil {
		return errors.Wrap(err, "verify fs version")
	}
	fsConfig := initFSConfig
	if len(fsConfigData) > 0 {
		fsConfig, err = format.LoadFSConfig(fsConfigData)
		if err != nil {
			return errors.Wrap(err, "parse fs config")
		}
	}
	fsConfigData, err = fsConfig.Save()
	if err != nil {
		return errors.Wrap(err, "marshal config")
	}
	if err := os.WriteFile(
		fsConfigPath, fsConfigData, localFileMode,
	); err != nil {
		return errors.Wrap(err, "write fs config")
	}

	if err := os.MkdirAll(
		filepath.Join(root, "files"), localDirMode,
	); err != nil {
		return errors.Wrap(err, "mkdir files")
	}
	if err := os.MkdirAll(
		filepath.Join(root, "objs"), localDirMode,
	); err != nil {
		return errors.Wrap(err, "mkdir objs")
	}
	return nil
}
