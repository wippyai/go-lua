package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

func (l *lowerer) typedPresenceRefinement(target path.Path, value presence.Value) factflow.ValueRefinement {
	refinement := l.presenceRefinement(value)
	if !presence.Equal(value, presence.Present()) {
		return refinement
	}
	staticValue, ok := l.pathStaticValue(target)
	if !ok {
		return refinement
	}
	return refinement.WithConstraint(l.registry, staticValue)
}

func (l *lowerer) pathStaticValue(target path.Path) (product.Value, bool) {
	if l == nil || l.registry == nil || target.Symbol == 0 {
		return product.Value{}, false
	}
	t, ok := l.symbolTypes[target.Symbol]
	if !ok {
		return product.Value{}, false
	}
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return product.Value{}, false
	}
	t, ok = luatypeprojection.ApplySegments(t, target.Segments)
	if !ok {
		return product.Value{}, false
	}
	if !unwrap.IsOptionalLike(t) {
		return product.Value{}, false
	}
	present := unwrap.Optional(t)
	if present == nil {
		return product.Value{}, false
	}
	return l.valueFromTypeWithWitness(present), true
}
