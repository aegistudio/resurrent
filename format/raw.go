package format

import (
	"bytes"

	"go.yaml.in/yaml/v4"
)

type Raw []byte

func (r Raw) MarshalYAML() (any, error) {
	result := &yaml.Node{}
	dec := yaml.NewDecoder(bytes.NewReader(r))
	if err := dec.Decode(result); err != nil {
		return nil, err
	}
	return result, nil
}

var _ yaml.Marshaler = Raw(nil)

func (r *Raw) UnmarshalYAML(v *yaml.Node) error {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	if err := enc.Encode(v); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	*r = Raw(b.Bytes())
	return nil
}

var _ yaml.Unmarshaler = (*Raw)(nil)
