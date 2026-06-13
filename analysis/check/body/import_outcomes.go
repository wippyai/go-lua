package body

import (
	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func calleeValueProvider(
	reg *axis.Registry,
	facts factflow.Facts,
	resolver *visibility.Resolver,
) effectlowering.CalleeValueFunc {
	return func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
		p := site.CalleePath()
		if p.IsEmpty() {
			return product.Value{}, false
		}
		config := readexprConfig(reg, facts, resolver)
		if value, ok := readCalleePath(config, ctx.Point, p, in); ok && hasTypeWitness(reg, value) {
			return value, true
		}
		if len(p.Segments) == 0 {
			return product.Value{}, false
		}
		root := p
		root.Segments = nil
		rootValue, ok := readCalleePath(config, ctx.Point, root, in)
		if !ok {
			return product.Value{}, false
		}
		rootType, ok := witnessedType(reg, rootValue)
		if !ok {
			return product.Value{}, false
		}
		projected, ok := typeaccess.ProjectSegments(rootType, p.Segments)
		if !ok || projected == nil {
			return product.Value{}, false
		}
		return typevalue.WithWitness(reg, typevalue.FromType(reg, projected), projected), true
	}
}

func readexprConfig(reg *axis.Registry, facts factflow.Facts, resolver *visibility.Resolver) readexpr.Config {
	return readexpr.Config{Registry: reg, Facts: facts, Visibility: resolver}
}

func readCalleePath(config readexpr.Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	return readexpr.Project(config, point, p, in)
}

func hasTypeWitness(reg *axis.Registry, value product.Value) bool {
	_, ok := witnessedType(reg, value)
	return ok
}

func witnessedType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if reg == nil {
		return nil, false
	}
	witness := product.Get(reg, value, typewitness.Key)
	return witness.Type()
}
