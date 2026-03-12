package format

import (
	"sort"

	utilYAML "github.com/aegistudio/resurrent/util/yaml"
	"github.com/pkg/errors"
	"go.yaml.in/yaml/v4"
)

const (
	CurrentBucketVersion = 1
)

// Bucket is the format of object buckets
// stored under `objs/`. The object bucket
// stores objects of common SHA256 prefix.
type Bucket struct {
	Version int                `yaml:"version"`
	Objects map[string]*Object `yaml:"objs"`
}

type bucketFormat struct {
	Version int                  `yaml:"version"`
	Objects []map[string]*Object `yaml:"objs"`
}

func (b *Bucket) UnmarshalYAML(node *yaml.Node) error {
	var rawBucket bucketFormat
	if err := node.Load(&rawBucket); err != nil {
		return err
	}
	b.Version = rawBucket.Version
	if ver := b.Version; ver > CurrentBucketVersion {
		return versionTooNew(ver)
	}
	b.Version = CurrentBucketVersion
	b.Objects = make(map[string]*Object)
	for _, entry := range rawBucket.Objects {
		if len(entry) != 1 {
			return errors.New("invalid object entry")
		}
		for id, value := range entry {
			b.Objects[id] = value
		}
	}
	return nil
}

var _ yaml.Unmarshaler = (*Object)(nil)

func (b Bucket) MarshalYAML() (any, error) {
	var rawBucket bucketFormat
	rawBucket.Version = CurrentBucketVersion
	var objs []string
	for obj := range b.Objects {
		objs = append(objs, obj)
	}
	sort.Strings(objs)
	for _, obj := range objs {
		entry := make(map[string]*Object)
		entry[obj] = b.Objects[obj]
		rawBucket.Objects = append(rawBucket.Objects, entry)
	}
	return rawBucket, nil
}

var _ yaml.Marshaler = (*FileMeta)(nil)

func LoadBucket(v []byte) (*Bucket, error) {
	result := &Bucket{}
	if err := utilYAML.LoadYAML(v, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (o *Bucket) Save() ([]byte, error) {
	return utilYAML.SaveYAML(o)
}
