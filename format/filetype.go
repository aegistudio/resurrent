package format

import (
	"github.com/pkg/errors"
	"go.yaml.in/yaml/v4"
)

type FileType uint64

const (
	TypeUnknown = FileType(iota)

	// TypeFile is the type when the target
	// is a storage file.
	TypeFile

	// TypeDir is the type when the target
	// is a directory.
	TypeDir
)

func (t FileType) String() string {
	switch t {
	case TypeFile:
		return "file"
	case TypeDir:
		return "dir"
	default:
		return ""
	}
}

func ParseFileType(s string) (FileType, error) {
	switch s {
	case "file":
		return TypeFile, nil
	case "dir":
		return TypeDir, nil
	default:
		return TypeUnknown, errors.Errorf("unknown file type %q", s)
	}
}

func (t FileType) Valid() bool {
	switch t {
	case TypeFile, TypeDir:
		return true
	default:
		return false
	}
}

func (t *FileType) UnmarshalYAML(node *yaml.Node) error {
	var value string
	if err := node.Decode(&value); err != nil {
		return err
	}
	v, err := ParseFileType(value)
	if err != nil {
		return err
	}
	*t = v
	return nil
}

var _ yaml.Unmarshaler = (*FileType)(nil)

func (t FileType) MarshalYAML() (any, error) {
	if !t.Valid() {
		return nil, errors.New("invalid file type")
	}
	return t.String(), nil
}

var _ yaml.Marshaler = TypeUnknown
