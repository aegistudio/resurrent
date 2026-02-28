package format

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v4"
)

func TestMergeDoc(t *testing.T) {
	assert := assert.New(t)
	var err error
	var a struct {
		A int
		B int
		C bool
	}
	a.A = 123
	a.B = 456
	a.C = true
	var b struct {
		A int
		D string
	}
	b.A = 789
	b.D = "abc"

	aNode, err := saveYAMLNode(a)
	assert.NoError(err)
	if err != nil {
		return
	}

	bNode, err := saveYAMLNode(b)
	assert.NoError(err)
	if err != nil {
		return
	}

	cNode, err := mergeYAMLDocs(aNode, bNode)
	assert.NoError(err)
	if err != nil {
		return
	}

	cData, err := saveYAML(cNode)
	assert.NoError(err)
	if err != nil {
		return
	}
	t.Logf("merged doc: %v", string(cData))

	dec := yaml.NewDecoder(bytes.NewReader(cData))
	err = dec.Decode(&a)
	assert.NoError(err)
	if err != nil {
		return
	}

	assert.Equal(789, a.A)
	assert.Equal(456, a.B)
	assert.Equal(true, a.C)
}
