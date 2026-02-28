package format

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"
	"go.yaml.in/yaml/v4"
)

type EngineHeader struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

type Engine struct {
	EngineHeader

	// Data is the parsed data of the engine.
	//
	// If there's any engine format registered
	// by RegisterEngineFormat, then the registered
	// factory function will be called, and the
	// data will be decoded into it.
	//
	// Otherwise, a *format.Raw object is created,
	// and the original content is loaded into
	// the engine data.
	Data any
}

var engineFormats sync.Map

// RegisterEngineFormat register an engine format
// factory that would be decoded unmarshaled later.
//
// If there's no corresponding engine factory,
// then format.Raw will take its place.
func RegisterEngineFormat(typ string, f func() any) {
	if _, loaded := engineFormats.LoadOrStore(typ, f); loaded {
		panic(fmt.Sprintf(
			"engine format %q already exists", typ,
		))
	}
}

func (e Engine) MarshalYAML() (any, error) {
	dataNode, err := saveYAMLNode(e.Data)
	if err != nil {
		return nil, errors.Wrap(err, "marshal engine data")
	}
	headerNode, err := saveYAMLNode(e.EngineHeader)
	if err != nil {
		return nil, errors.Wrap(err, "marshal header node")
	}
	node, err := mergeYAMLDocs(dataNode, headerNode)
	if err != nil {
		return nil, errors.Wrap(err, "merge documents")
	}
	// The merged result is a document, and the root
	// node is a mapping node.
	return node.Content[0], nil
}

var _ yaml.Marshaler = (*Engine)(nil)

func (e *Engine) UnmarshalYAML(node *yaml.Node) error {
	if err := node.Decode(&e.EngineHeader); err != nil {
		return errors.Wrap(err, "decode header")
	}
	if f, ok := engineFormats.Load(e.Type); ok {
		e.Data = (f.(func() any))()
	} else {
		e.Data = &Raw{}
	}
	if err := node.Decode(e.Data); err != nil {
		return errors.Wrap(err, "decode data")
	}
	return nil
}

var _ yaml.Unmarshaler = (*Engine)(nil)
