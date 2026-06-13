package transferfacts

import factflow "github.com/wippyai/go-lua/analysis/engine/factflow"

func (l *lowerer) exprRef(expr any) (factflow.ExprRef, bool) {
	if expr == nil {
		return 0, false
	}
	if ref, ok := l.exprs[expr]; ok {
		return ref, true
	}
	ref := factflow.ExprRef(len(l.exprs) + 1)
	l.exprs[expr] = ref
	return ref, true
}
