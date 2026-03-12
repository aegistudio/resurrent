package format

import (
	"time"

	utilYAML "github.com/aegistudio/resurrent/util/yaml"
	"go.yaml.in/yaml/v4"
)

const (
	CurrentFileMetaVersion = 1
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

func (meta *FileMeta) UnmarshalYAML(node *yaml.Node) error {
	type rawFileMeta FileMeta
	var rawMeta rawFileMeta
	if err := node.Load(&rawMeta); err != nil {
		return err
	}
	*meta = FileMeta(rawMeta)
	if ver := meta.Version; ver > CurrentFileMetaVersion {
		return versionTooNew(ver)
	}
	meta.Version = CurrentFileMetaVersion
	return nil
}

var _ yaml.Unmarshaler = (*FileMeta)(nil)

func (meta FileMeta) MarshalYAML() (any, error) {
	meta.Version = CurrentFileMetaVersion
	type rawFileMeta FileMeta
	return rawFileMeta(meta), nil
}

var _ yaml.Marshaler = (*FileMeta)(nil)

func LoadFileMeta(v []byte) (*FileMeta, error) {
	result := &FileMeta{}
	if err := utilYAML.LoadYAML(v, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (meta *FileMeta) Save() ([]byte, error) {
	return utilYAML.SaveYAML(meta)
}
