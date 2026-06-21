package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (l *lowerer) typeRefs(types []ast.TypeExpr) []factflow.TypeRef {
	if len(types) == 0 {
		return nil
	}
	out := make([]factflow.TypeRef, 0, len(types))
	for i := range types {
		if ref, ok := l.typeRef(types[i]); ok {
			out = append(out, ref)
		}
	}
	return out
}

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
