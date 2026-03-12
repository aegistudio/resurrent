package format

import (
	"testing"

	utilYAML "github.com/aegistudio/resurrent/util/yaml"
	"github.com/stretchr/testify/assert"
)

func TestUnknownEngine(t *testing.T) {
	var err error
	assert := assert.New(t)

	var engine Engine
	err = utilYAML.LoadYAML([]byte(`
name: engine_name
type: unknown-engine
a: 123
b: 456
`), &engine)
	assert.NoError(err)
	if err != nil {
		return
	}

	assert.Equal("engine_name", engine.Name)
	assert.Equal("unknown-engine", engine.Type)
	raw, ok := engine.Data.(*Raw)
	assert.True(ok)
	t.Logf("decoded data: %v", string(*raw))
}

func TestCustomEngine(t *testing.T) {
	var err error
	assert := assert.New(t)

	type customEngine struct {
		A int `yaml:"a"`
		B int `yaml:"b"`
	}
	RegisterEngineFormat("test-engine", func() any {
		return &customEngine{}
	})

	var engine Engine
	err = utilYAML.LoadYAML([]byte(`
name: engine_name
type: test-engine
a: 123
b: 456
`), &engine)
	assert.NoError(err)
	if err != nil {
		return
	}

	assert.Equal("engine_name", engine.Name)
	assert.Equal("test-engine", engine.Type)
	asserted, ok := engine.Data.(*customEngine)
	assert.True(ok)
	assert.Equal(123, asserted.A)
	assert.Equal(456, asserted.B)
}
