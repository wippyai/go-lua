package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func stdlibSignatureReturnValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	sources sourcevalue.SourceValues,
	refinements sourcevalue.ExpressionRefinements,
	visibilityResolver *visibility.Resolver,
) effectlowering.SignatureReturnValueFunc {
	if reg == nil || sources == nil {
		return nil
	}
	sourceResolver := refinements.Bind(reg, sources)
	return func(ctx effectlowering.SignatureReturnContext) (product.Value, bool) {
		switch {
		case ctx.Name == "type" && ctx.Index == 0 && bareGlobalCall(ctx.Site, "type"):
			return typeCallReturnValue(reg, typeValues, sourceResolver, ctx)
		case ctx.Name == "select" && ctx.Index == 0 && bareGlobalCall(ctx.Site, "select"):
			return selectCountReturnValue(reg, sourceResolver, ctx)
		case ctx.Name == "tonumber" && ctx.Index == 0 && bareGlobalCall(ctx.Site, "tonumber") && ctx.Site.ArgumentSourceCount() >= 2:
			return returnValueWithType(reg, normalize.Optional(typ.Integer)), true
		case ctx.Name == "string.unpack" && ctx.Index == 0:
			return stringUnpackFirstReturnValue(reg, sourceResolver, ctx)
		case (ctx.Name == "unpack" || ctx.Name == "table.unpack") && ctx.Index == 0:
			return tableUnpackFirstReturnValue(reg, typeValues, sourceResolver, visibilityResolver, ctx)
		default:
			return product.Value{}, false
		}
	}
}

func typeCallReturnValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	resolver sourcevalue.SourceValues,
	ctx effectlowering.SignatureReturnContext,
) (product.Value, bool) {
	arg, ok := ctx.Site.ArgumentSourceAt(0)
	if !ok {
		return product.Value{}, false
	}
	value, ok := resolver.ValueOfSource(ctx.Node.Point, arg, ctx.In, ctx.Read)
	if !ok {
		return product.Value{}, false
	}
	kinds := product.Get(reg, value, runtimekind.Key)
	if kinds.IsTop() || kinds.IsBottom() {
		if t, ok := typeValues.TypeOf(reg, value); ok {
			kinds, _ = typevalue.RuntimeKindFromType(t)
		}
	}
	tags := kinds.Tags()
	if len(tags) != 1 {
		return product.Value{}, false
	}
	return returnValueWithType(reg, typ.LiteralString(tags[0].String())), true
}

func selectCountReturnValue(
	reg *axis.Registry,
	resolver sourcevalue.SourceValues,
	ctx effectlowering.SignatureReturnContext,
) (product.Value, bool) {
	arg, ok := ctx.Site.ArgumentSourceAt(0)
	if !ok {
		return product.Value{}, false
	}
	value, ok := resolver.ValueOfSource(ctx.Node.Point, arg, ctx.In, ctx.Read)
	if !ok || !isHashLiteral(reg, value) {
		return product.Value{}, false
	}
	return returnValueWithType(reg, typ.Integer), true
}

func bareGlobalCall(site factflow.CallSiteView, name string) bool {
	if site.MethodName() != "" {
		return false
	}
	callee := site.CalleePathRef()
	return callee.Root == name && callee.Symbol != 0 && len(callee.Segments) == 0
}

func isHashLiteral(reg *axis.Registry, value product.Value) bool {
	s, ok := typevalue.StringLiteralOf(reg, value)
	return ok && s == "#"
}

func stringUnpackFirstReturnValue(
	reg *axis.Registry,
	resolver sourcevalue.SourceValues,
	ctx effectlowering.SignatureReturnContext,
) (product.Value, bool) {
	format, ok := stringUnpackFormatLiteral(reg, resolver, ctx)
	if !ok {
		return product.Value{}, false
	}
	t, ok := firstStringUnpackValueType(format)
	if !ok {
		return product.Value{}, false
	}
	return returnValueWithType(reg, t), true
}

func tableUnpackFirstReturnValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	resolver sourcevalue.SourceValues,
	visibilityResolver *visibility.Resolver,
	ctx effectlowering.SignatureReturnContext,
) (product.Value, bool) {
	arg, ok := ctx.Site.ArgumentSourceAt(0)
	if !ok {
		return product.Value{}, false
	}
	value, ok := resolver.ValueOfSource(ctx.Node.Point, arg, ctx.In, ctx.Read)
	if !ok {
		return product.Value{}, false
	}
	if visibilityResolver != nil {
		if first, ok := sourcevalue.HeapMemberFromValue(reg, visibilityResolver.KeySpace(), ctx.In, value, []segment.Segment{{
			Kind:  segment.SegmentIndexInt,
			Index: 1,
		}}); ok {
			return first, true
		}
	}
	container, ok := typeValues.TypeOf(reg, value)
	if !ok {
		return product.Value{}, false
	}
	if first, ok := access.Index(container, typ.LiteralInt(1)); ok {
		return returnValueWithType(reg, first), true
	}
	elem, ok := projection.ElementOf(container)
	if !ok {
		return product.Value{}, false
	}
	out := returnValueWithType(reg, elem)
	if !typevalue.DefinitelyNonEmptyIndexContainer(container) {
		out = product.WithPresence(reg, out, presence.Maybe())
	}
	return out, true
}

func stringUnpackFormatLiteral(
	reg *axis.Registry,
	resolver sourcevalue.SourceValues,
	ctx effectlowering.SignatureReturnContext,
) (string, bool) {
	arg, ok := ctx.Site.ArgumentSourceAt(0)
	if !ok {
		return "", false
	}
	value, ok := resolver.ValueOfSource(ctx.Node.Point, arg, ctx.In, ctx.Read)
	if !ok {
		return "", false
	}
	return typevalue.StringLiteralOf(reg, value)
}

func firstStringUnpackValueType(format string) (typ.Type, bool) {
	for i := 0; i < len(format); {
		switch format[i] {
		case ' ', '\n', '\r', '\t', '\v', '\f', '<', '>', '=':
			i++
		case '!':
			i++
			i = skipDecimalDigits(format, i)
		case 'x':
			i++
		case 'b', 'B', 'h', 'H', 'l', 'L', 'j', 'J', 'T':
			return typ.Integer, true
		case 'i', 'I':
			i++
			_ = skipDecimalDigits(format, i)
			return typ.Integer, true
		case 'f', 'd', 'n':
			return typ.Number, true
		case 'c', 's':
			i++
			_ = skipDecimalDigits(format, i)
			return typ.String, true
		case 'z':
			return typ.String, true
		default:
			return nil, false
		}
	}
	return nil, false
}

func skipDecimalDigits(s string, i int) int {
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i
}

func returnValueWithType(reg *axis.Registry, t typ.Type) product.Value {
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
}
