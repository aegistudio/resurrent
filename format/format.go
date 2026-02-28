package format

import (
	"time"
)

const (
	CurrentVersion = 1
)

// FileMeta is the format of metadata.
//
// It contains the pure metadata, plus
// the revision of the file. Please notice
// other format does not have revision.
type FileMeta struct {
	Version int `yaml:"version"`

	Name string   `yaml:"name"`
	Type FileType `yaml:"type"`
	Perm FilePerm `yaml:"perm"`

	// Size is the redundant copy of Size
	// in the object storage. It is
	// deliberately placed here to reduce
	// the number of required IO operations.
	// It is available only if
	// Meta.Type == TypeFile.
	Size int64 `yaml:"size,omitempty"`

	// ModifiedAt is the modify time of the
	// file when it was snapshotted.
	//
	// If the data of the file is present
	// under `files/`, then the state of data
	// will be reporeted, otherwise this value
	// will be reported.
	//
	// This value is also involved in the upload
	// and download process, to judge whether
	// a new upload check is required.
	ModifiedAt time.Time `yaml:"modified_at,omitempty"`

	// Object is the ID of object stored
	// in the `objs/`. It is available only
	// if Meta.Type == TypeFile.
	Object string `yaml:"object,omitempty"`
}

func LoadFileMeta(v []byte) (*FileMeta, error) {
	result := &FileMeta{}
	if err := loadYAML(v, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (f *FileMeta) Save() ([]byte, error) {
	return saveYAML(f)
}

// Object is the format of object stored
// under `objs/`. The object is indexed by
// its SHA256 hash.
type Object struct {
	Version int `yaml:"version"`

	// Size is the actual size of the object.
	Size int64 `yaml:"size,omitempty"`

	// Nonce is used to resolve collision.
	//
	// The Nonce will be written out to
	// the hasher, and then the file content.
	// If there were a collision, the nonce
	// will be incremented by one.
	Nonce uint64 `yaml:"nonce,omitempty"`

	// Filters are applied before being sent
	// to any of the storages.
	//
	// Every filter item must be a valid
	// URI, and they are predefined.
	Filters []string `yaml:"filters"`

	// Storages stores the object storage information.
	//
	// Every storage item must be a valid
	// URI, the hostname will be the engine
	// specified in `resurrent.yaml`.
	Storages []string `yaml:"storages"`
}

func LoadObject(v []byte) (*Object, error) {
	result := &Object{}
	if err := loadYAML(v, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (o *Object) Save() ([]byte, error) {
	return saveYAML(o)
}

var (
	// DefaultInlineMaxSize is 32kB.
	DefaultInlineMaxSize = 32 * 1024
)

// FSConfig is the filesystem configuration
// format stored at "/resurrent.yml".
type FSConfig struct {
	Version int `yaml:"version"`

	// Engines for performing object storages.
	Engines []Engine

	// CaseSensitive indicates whether the
	// filesystem should be case sensitive.
	//
	// By default, it's case insensitive so
	// that it works fluently under Windows
	// and Mac OS.
	CaseSensitive bool `yaml:"case_sensitive,omitempty"`

	// InlineMaxSize is the size threshold
	// before the filesystem decides to send
	// it into one of the object storage engines.
	//
	// By default, it's DefaultInlineMaxSize,
	// after being DEFLATE-compressed.
	InlineMaxSize int64 `yaml:"inline_max_size,omitempty"`

	// DownloadOnDemand is the list of Glob
	// patterns that matches files to trigger
	// automatic download when open.
	DownloadOnDemand []string `yaml:"download_on_demand,omitempty"`
}

func LoadFSConfig(v []byte) (*FSConfig, error) {
	result := &FSConfig{}
	if err := loadYAML(v, result); err != nil {
		return result, err
	}
	if result.InlineMaxSize == 0 {
		result.InlineMaxSize = int64(DefaultInlineMaxSize)
	}

	return result, nil
}

func (f *FSConfig) Save() ([]byte, error) {
	return saveYAML(f)
}
