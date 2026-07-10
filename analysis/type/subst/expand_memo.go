package subst

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

const expandMemoMaxEntries = 2048

type expandMode uint8

const (
	expandModeStructural expandMode = iota
	expandModeTablePolicy
)

type expandMemoKey struct {
	t    typ.Type
	mode expandMode
}

var expandMemoPool = sync.Pool{
	New: func() any {
		return make(map[expandMemoKey]typ.Type, 32)
	},
}

func getExpandMemo() map[expandMemoKey]typ.Type {
	return expandMemoPool.Get().(map[expandMemoKey]typ.Type)
}

func putExpandMemo(m map[expandMemoKey]typ.Type) {
	if len(m) > expandMemoMaxEntries {
		expandMemoPool.Put(make(map[expandMemoKey]typ.Type, 32))
		return
	}
	clear(m)
	expandMemoPool.Put(m)
}
