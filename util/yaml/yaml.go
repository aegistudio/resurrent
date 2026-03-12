package utilYAML

import (
	"bytes"

	"github.com/pkg/errors"
	"go.yaml.in/yaml/v4"
)

func LoadYAML(v []byte, obj any) error {
	loader, err := yaml.NewLoader(
		bytes.NewReader(v),
		yaml.WithKnownFields(true),
	)
	if err != nil {
		return err
	}
	return loader.Load(obj)
}

func SaveYAML(obj any) ([]byte, error) {
	var buf bytes.Buffer
	dumper, err := yaml.NewDumper(
		&buf,
		yaml.WithIndent(2),
	)
	if err != nil {
		return nil, err
	}
	if err := dumper.Dump(obj); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func LoadYAMLNode(v []byte) (*yaml.Node, error) {
	result := &yaml.Node{}
	if err := LoadYAML(v, result); err != nil {
		return nil, err
	}
	return result, nil
}

func SaveYAMLNode(obj any) (*yaml.Node, error) {
	v, err := SaveYAML(obj)
	if err != nil {
		return nil, err
	}
	return LoadYAMLNode(v)
}

func MergeYAMLDocs(d1, d2 *yaml.Node) (*yaml.Node, error) {
	result := &yaml.Node{
		Kind: yaml.MappingNode,
	}
	record := make(map[string]int)
	process := func(node *yaml.Node) error {
		if node.Kind != yaml.DocumentNode {
			return errors.New("invalid non-document node")
		}
		if len(node.Content) == 0 {
			return nil
		}
		doc := node.Content[0]
		if doc.Kind != yaml.MappingNode {
			return errors.New("invalid non-mapping node")
		}
		for i := 0; i < len(doc.Content); i += 2 {
			key := doc.Content[i]
			value := doc.Content[i+1]
			if key.Kind != yaml.ScalarNode {
				return errors.New("invalid key node")
			}
			if idx, found := record[key.Value]; found {
				result.Content[idx+1] = value
			} else {
				record[key.Value] = len(result.Content)
				result.Content = append(result.Content, key, value)
			}
		}
		return nil
	}
	if err := process(d1); err != nil {
		return nil, err
	}
	if err := process(d2); err != nil {
		return nil, err
	}
	return &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{result},
	}, nil
}
