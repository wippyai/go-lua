package subst

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

const expandMemoMaxEntries = 2048

type expandMode uint8

const (
	expandModeStructural expandMode = iota
	expandModeTablePolicy
	expandModeRoot
)

type expandMemoKey struct {
	t    typ.Type
	mode expandMode
}

type expandState struct {
	memo   map[expandMemoKey]typ.Type
	active []*activeInstantiation
}

type activeInstantiation struct {
	generic *typ.Generic
	args    []typ.Type
	mu      *typ.Recursive
	used    bool
}

func (s *expandState) matchingActive(v *typ.Instantiated) *activeInstantiation {
	if s == nil || v == nil {
		return nil
	}
	for i := len(s.active) - 1; i >= 0; i-- {
		candidate := s.active[i]
		if candidate.generic != v.Generic || len(candidate.args) != len(v.TypeArgs) {
			continue
		}
		equal := true
		for j := range candidate.args {
			if !typ.TypeEquals(candidate.args[j], v.TypeArgs[j]) {
				equal = false
				break
			}
		}
		if equal {
			return candidate
		}
	}
	return nil
}

func (s *expandState) hasActiveGeneric(generic *typ.Generic) bool {
	for i := len(s.active) - 1; i >= 0; i-- {
		if s.active[i].generic == generic {
			return true
		}
	}
	return false
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
