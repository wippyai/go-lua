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
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func calleeValueProvider(
	reg *axis.Registry,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	sources sourcevalue.SourceValues,
	typeValues *typevalue.Cache,
) CalleeValueFunc {
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
		p := site.CalleePath()
		if p.IsEmpty() {
			return methodCalleeValue(reg, typeValues, sources, ctx, site, in, read)
		}
		if value, ok := pathMethodCalleeValue(reg, typeValues, facts, resolver, ctx, site, in); ok {
			return value, true
		}
		config := readexpr.Config{Registry: reg, Facts: facts, Visibility: resolver, TypeValues: typeValues}
		if value, ok := readexpr.Project(config, ctx.Point, p, in); ok && hasTypeWitness(reg, value) {
			return value, true
		}
		if value, ok, authoritative := readablePrefixCalleeValue(reg, typeValues, config, ctx.Point, p, in); authoritative {
			return value, ok
		}
		if len(p.Segments) == 0 {
			return product.Value{}, false
		}
		root := p
		root.Segments = nil
		rootValue, ok := readexpr.Project(config, ctx.Point, root, in)
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
		return typeValues.FromTypeWithWitness(reg, projected), true
	}
}

func readablePrefixCalleeValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	config readexpr.Config,
	point cfg.Point,
	p pathdom.Path,
	in state.State,
) (product.Value, bool, bool) {
	if len(p.Segments) < 2 {
		return product.Value{}, false, false
	}
	for prefix := p.Parent(); !prefix.IsEmpty() && len(prefix.Segments) > 0; prefix = prefix.Parent() {
		value, ok := readexpr.Project(config, point, prefix, in)
		if !ok {
			continue
		}
		prefixType, ok := witnessedType(reg, value)
		if !ok || prefixType == nil || typ.IsAny(prefixType) || typ.IsUnknown(prefixType) || typ.IsNever(prefixType) {
			return product.Value{}, false, true
		}
		suffix := p.Segments[len(prefix.Segments):]
		projected, ok := luatypeprojection.ApplySegments(prefixType, suffix)
		if !ok || projected == nil {
			return product.Value{}, false, true
		}
		return typeValues.FromTypeWithWitness(reg, projected), true, true
	}
	return product.Value{}, false, false
}

func pathMethodCalleeValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
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
	config := readexpr.Config{Registry: reg, Facts: facts, Visibility: resolver, TypeValues: typeValues}
	receiverValue, ok := readexpr.Project(config, ctx.Point, receiverPath, in)
	if !ok {
		return product.Value{}, false
	}
	receiverType, ok := witnessedType(reg, receiverValue)
	if !ok || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return product.Value{}, false
	}
	return methodTypeValue(reg, typeValues, receiverType, method)
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
	typeValues *typevalue.Cache,
	sources sourcevalue.SourceValues,
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
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
	return methodTypeValue(reg, typeValues, receiverType, method)
}

func methodTypeValue(reg *axis.Registry, typeValues *typevalue.Cache, receiverType typ.Type, method string) (product.Value, bool) {
	methodType, status, ok := typecall.MemberCallable(receiverType, method)
	if status != typecall.MemberCallOK || !ok || methodType == nil {
		return product.Value{}, false
	}
	return typeValues.FromTypeWithWitness(reg, methodType), true
}

func channelMethodReceiverTypeProvider(
	reg *axis.Registry,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	sources sourcevalue.SourceValues,
	typeValues *typevalue.Cache,
) effectlowering.ReceiverTypeFunc {
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (typ.Type, bool) {
		if receiverPath, ok := site.ReceiverPath(); ok && !receiverPath.IsEmpty() {
			config := readexpr.Config{Registry: reg, Facts: facts, Visibility: resolver, TypeValues: typeValues}
			value, ok := readexpr.Project(config, ctx.Point, receiverPath, in)
			if !ok {
				return nil, false
			}
			return witnessedType(reg, value)
		}
		if sources == nil {
			return nil, false
		}
		source, ok := site.ReceiverSource()
		if !ok {
			return nil, false
		}
		value, ok := sources.ValueOfSource(ctx.Point, source, in, read)
		if !ok {
			return nil, false
		}
		return witnessedType(reg, value)
	}
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
