package format

import (
	utilYAML "github.com/aegistudio/resurrent/util/yaml"
	"go.yaml.in/yaml/v4"
)

const (
	CurrentFSConfigVersion = 1
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

func (f *FSConfig) UnmarshalYAML(node *yaml.Node) error {
	type rawFSConfig FSConfig
	var rawCfg rawFSConfig
	if err := node.Load(&rawCfg); err != nil {
		return err
	}
	*f = FSConfig(rawCfg)
	if ver := f.Version; ver > CurrentFSConfigVersion {
		return versionTooNew(ver)
	}
	f.Version = CurrentFSConfigVersion
	if f.InlineMaxSize == 0 {
		f.InlineMaxSize = int64(DefaultInlineMaxSize)
	}
	return nil
}

var _ yaml.Unmarshaler = (*FileMeta)(nil)

func (f FSConfig) MarshalYAML() (any, error) {
	f.Version = CurrentFSConfigVersion
	type rawFSConfig FSConfig
	return rawFSConfig(f), nil
}

var _ yaml.Marshaler = (*FileMeta)(nil)

func LoadFSConfig(v []byte) (*FSConfig, error) {
	result := &FSConfig{}
	if err := utilYAML.LoadYAML(v, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (f *FSConfig) Save() ([]byte, error) {
	return utilYAML.SaveYAML(f)
}
