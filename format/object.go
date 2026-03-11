package format

import (
	"go.yaml.in/yaml/v4"
)

const (
	CurrentObjectVersion = 1
)

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

func (o *Object) UnmarshalYAML(node *yaml.Node) error {
	type rawObject Object
	var rawObj rawObject
	if err := node.Load(&rawObj); err != nil {
		return err
	}
	*o = Object(rawObj)
	if ver := o.Version; ver > CurrentObjectVersion {
		return versionTooNew(ver)
	}
	o.Version = CurrentObjectVersion
	return nil
}

var _ yaml.Unmarshaler = (*Object)(nil)

func (o Object) MarshalYAML() (any, error) {
	o.Version = CurrentObjectVersion
	type rawObject Object
	return rawObject(o), nil
}

var _ yaml.Marshaler = (*FileMeta)(nil)
