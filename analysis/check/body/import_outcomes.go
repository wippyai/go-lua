package body

import (
	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func calleeValueProvider(
	reg *axis.Registry,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	sources sourcevalue.SourceValues,
) CalleeValueFunc {
	return func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
		p := site.CalleePath()
		if p.IsEmpty() {
			return methodCalleeValue(reg, sources, ctx, site, in, read)
		}
		if value, ok := pathMethodCalleeValue(reg, facts, resolver, ctx, site, in); ok {
			return value, true
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
		projected, ok := luaPathTypeProjector(rootType, p)
		if !ok || projected == nil {
			return product.Value{}, false
		}
		return typevalue.WithWitness(reg, typevalue.FromType(reg, projected), projected), true
	}
}

func pathMethodCalleeValue(
	reg *axis.Registry,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	ctx transfer.NodeContext,
	site factflow.CallSite,
	in state.State,
) (product.Value, bool) {
	method := site.MethodName()
	if method == "" {
		return product.Value{}, false
	}
	receiverPath, ok := site.ReceiverPath()
	if !ok || receiverPath.IsEmpty() {
		return product.Value{}, false
	}
	config := readexprConfig(reg, facts, resolver)
	receiverValue, ok := readCalleePath(config, ctx.Point, receiverPath, in)
	if !ok {
		return product.Value{}, false
	}
	receiverType, ok := witnessedType(reg, receiverValue)
	if !ok || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return product.Value{}, false
	}
	return methodTypeValue(reg, receiverType, method)
}

// methodCalleeValue resolves the callee of a colon-method call whose receiver
// is an anonymous prior call result (a builder chain). The receiver carries no
// symbol path, so the method is resolved from the receiver's value type with
// the same member-call mechanism the diagnostics layer uses for argument
// checks: the receiver type's member must be a concrete callable, and Self is
// bound to the receiver type. When the receiver type is any/top or the member
// is unresolved, no value is produced so the result stays top.
func methodCalleeValue(
	reg *axis.Registry,
	sources sourcevalue.SourceValues,
	ctx transfer.NodeContext,
	site factflow.CallSite,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	method := site.MethodName()
	if method == "" || sources == nil {
		return product.Value{}, false
	}
	source, ok := site.ReceiverSource()
	if !ok {
		return product.Value{}, false
	}
	receiverValue, ok := sources.ValueOfSource(ctx.Point, source, in, read)
	if !ok {
		return product.Value{}, false
	}
	receiverType, ok := witnessedType(reg, receiverValue)
	if !ok || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return product.Value{}, false
	}
	return methodTypeValue(reg, receiverType, method)
}

func methodTypeValue(reg *axis.Registry, receiverType typ.Type, method string) (product.Value, bool) {
	memberType, status := typecall.MemberCall(receiverType, method)
	if status != typecall.MemberCallOK {
		return product.Value{}, false
	}
	callable, ok := typecall.Callable(memberType)
	if !ok || callable == nil {
		return product.Value{}, false
	}
	var methodType typ.Type = callable
	if substituted, ok := subst.Self(callable, receiverType).(*typ.Function); ok {
		methodType = substituted
	}
	if fn, ok := methodType.(*typ.Function); ok {
		if substituted, ok := subst.SelfRef(fn, receiverType).(*typ.Function); ok {
			methodType = substituted
		}
	}
	return typevalue.WithWitness(reg, typevalue.FromType(reg, methodType), methodType), true
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
