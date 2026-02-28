package format

import (
	"os"
	"strconv"

	"github.com/pkg/errors"
	"go.yaml.in/yaml/v4"
)

type FilePerm os.FileMode

func ParseFilePerm(value string) (FilePerm, error) {
	parsed, err := strconv.ParseUint(value, 8, 64)
	if err != nil {
		return FilePerm(0), errors.Errorf("invalid permission %q", value)
	}
	return FilePerm(os.FileMode(parsed).Perm()), nil
}

func (perm *FilePerm) UnmarshalYAML(node *yaml.Node) error {
	var value string
	if err := node.Decode(&value); err != nil {
		return err
	}
	parsed, err := ParseFilePerm(value)
	if err != nil {
		return err
	}
	*perm = parsed
	return nil
}

var _ yaml.Unmarshaler = (*FilePerm)(nil)

func (t FilePerm) String() string {
	value := uint64(os.FileMode(t).Perm())
	return strconv.FormatUint(value, 8)
}

func (t FilePerm) MarshalYAML() (any, error) {
	return t.String(), nil
}

var _ yaml.Marshaler = FilePerm(os.FileMode(0o777))
