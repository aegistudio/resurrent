package fusefs

import (
	"sync"

	"github.com/aegistudio/resurrent"
)

type FS struct {
	*resurrent.FS
	m        sync.Map
	inodeMtx sync.Mutex
	rootNode *resurrent.Node
}

func (fs *FS) Close() {
	defer fs.rootNode.Free()
}

func (fs *FS) RootNode() *Node {
	return fs.createNode(fs.rootNode)
}

func New(innerFS *resurrent.FS) *FS {
	return &FS{
		FS:       innerFS,
		rootNode: innerFS.RootNode(),
	}
}
