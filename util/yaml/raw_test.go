package utilYAML

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStructEmbed(t *testing.T) {
	assert := assert.New(t)
	var err error
	var data struct {
		Type string `yaml:"type"`
		Data Raw    `yaml:"data"`
	}
	err = LoadYAML([]byte(`
type: some_type
data:
  a: a
  b: b
  c: 123
`), &data)
	assert.NoError(err)
	assert.Equal("some_type", data.Type)
	t.Logf("The content of data: %v", string(Raw(data.Data)))
}

func TestLoadRaw(t *testing.T) {
	assert := assert.New(t)
	var err error
	var r Raw
	err = LoadYAML([]byte(`
a: 123
b: '456'
c: false
`), &r)
	assert.NoError(err)
	t.Logf("content of raw: %v", string(r))
}
