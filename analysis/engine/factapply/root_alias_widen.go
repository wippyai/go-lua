package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
)

// applyCovariantArrayAliasWiden widens the source symbol's array element witness
// when a covariant array alias is declared over a source whose element writes the
// heap cannot track back. The lowerer attaches the alias-widen contract value
// only for a bare identifier source whose declared array element type is
// strictly narrower than the alias's. A heap-tracked source (one carrying an
// exact identity) stays sound through identity-keyed element flow, so it is left
// untouched and covariant aliasing remains precise. An opaque source has no heap
// object, so a mutable alias with a wider element type lets writes through the
// alias store wider values without flowing back; widening the source's element
// witness to the alias's contract makes later reads of the source reflect the
// wider element soundly.
func applyCovariantArrayAliasWiden(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	out state.State,
	fact factflow.RootAssignment,
) state.State {
	wideValue, ok := fact.AliasWidenValue()
	if !ok {
		return out
	}
	source := fact.Source()
	if !source.HasExpr {
		return out
	}
	sourcePath, ok := facts.ExpressionPath(source.ExprRef)
	if !ok || sourcePath.Symbol == 0 || len(sourcePath.Segments) != 0 {
		return out
	}
	if sourcePath.Symbol == fact.TargetSymbol() {
		return out
	}
	sourceKey := key.SymbolValue(sourcePath.Symbol)
	sourceValue := out.ReadValue(ctx.Registry, sourceKey)
	if product.Equal(ctx.Registry, sourceValue, product.Bottom(ctx.Registry)) {
		return out
	}
	if _, tracked := product.Get(ctx.Registry, sourceValue, identity.Key).ID(); tracked {
		return out
	}
	wideWitness := product.Get(ctx.Registry, wideValue, typewitness.Key)
	if wideWitness.IsTop() || wideWitness.IsBottom() {
		return out
	}
	sourceWitness := product.Get(ctx.Registry, sourceValue, typewitness.Key)
	if typewitness.Equal(sourceWitness, wideWitness) {
		return out
	}
	return out.WriteValue(ctx.Registry, sourceKey, product.Set(ctx.Registry, sourceValue, typewitness.Key, wideWitness))
}
