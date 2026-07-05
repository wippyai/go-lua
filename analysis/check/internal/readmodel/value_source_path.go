package readmodel

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
)

func (r Reader) valueSourcePath(source factflow.ValueSource) (path.Path, bool) {
	if r.result == nil {
		return path.Path{}, false
	}
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if p, ok := r.result.ExpressionPathRef(source.ExprRef); ok && !p.IsEmpty() {
			return p, true
		}
		return r.result.ExpressionRefPath(source.ExprRef)
	}
	if source.Kind != factflow.ValueSourcePath || source.PathKey == "" {
		return path.Path{}, false
	}
	ks := r.result.KeySpace()
	if ks == nil {
		return path.Path{}, false
	}
	key, ok := ks.FromStateKey(source.PathKey)
	if !ok || key.Sym == 0 {
		return path.Path{}, false
	}
	return path.Path{
		Symbol:   key.Sym,
		Segments: ks.Segments(key),
	}, true
}
