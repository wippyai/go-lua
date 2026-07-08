package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func (l *lowerer) typeRef(typ any) (factflow.TypeRef, bool) {
	if typ == nil {
		return 0, false
	}
	if ref, ok := l.types[typ]; ok {
		return ref, true
	}
	if l.types == nil {
		l.types = make(map[any]factflow.TypeRef)
	}
	ref := factflow.TypeRef(len(l.types) + 1)
	l.types[typ] = ref
	return ref, true
}
