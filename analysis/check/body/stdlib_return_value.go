package body

import (
	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
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
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/stringlib"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func stdlibSignatureReturnValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	facts factflow.Facts,
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
		case ctx.Name == "string.byte":
			return stringByteReturnValue(reg), true
		case ctx.Name == "string.match":
			return stringMatchReturnValue(reg, sourceResolver, ctx)
		case ctx.Name == "string.find":
			return stringFindReturnValue(reg, sourceResolver, ctx)
		case ctx.Name == "string.gmatch" || ctx.Name == "string.gfind":
			return stringGMatchReturnValue(reg, sourceResolver, ctx)
		case ctx.Name == "string.unpack":
			return stringUnpackReturnValue(reg, sourceResolver, ctx)
		case (ctx.Name == "unpack" || ctx.Name == "table.unpack") && ctx.Index == 0:
			return tableUnpackFirstReturnValue(reg, typeValues, facts, sourceResolver, visibilityResolver, ctx)
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

func stringUnpackReturnValue(
	reg *axis.Registry,
	resolver sourcevalue.SourceValues,
	ctx effectlowering.SignatureReturnContext,
) (product.Value, bool) {
	format, ok := stringUnpackFormatLiteral(reg, resolver, ctx)
	if !ok {
		if ctx.Index == 0 {
			return product.Value{}, false
		}
		return returnValueWithType(reg, typ.Any), true
	}
	returns, ok := stringUnpackValueTypes(format)
	if !ok {
		return product.Value{}, false
	}
	if ctx.Index >= len(returns) {
		return typevalue.Nil(reg), true
	}
	return returnValueWithType(reg, returns[ctx.Index]), true
}

func tableUnpackFirstReturnValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	facts factflow.Facts,
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
	if arg.Kind == factflow.ValueSourceExpression && arg.HasExpr {
		if p, ok := facts.ExpressionPathRef(arg.ExprRef); ok && !p.IsEmpty() {
			firstPath := p.AppendSegments([]segment.Segment{{
				Kind:  segment.SegmentIndexInt,
				Index: 1,
			}})
			if first, ok := readexpr.Project(readexpr.Config{
				Registry:        reg,
				Facts:           facts,
				Visibility:      visibilityResolver,
				TypeValues:      typeValues,
				ProofVisibility: visibilityResolver,
			}, ctx.Node.Point, firstPath, ctx.In); ok {
				return first, true
			}
		}
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

func stringByteReturnValue(reg *axis.Registry) product.Value {
	return returnValueWithType(reg, normalize.Optional(typ.Integer))
}

func stringMatchReturnValue(
	reg *axis.Registry,
	resolver sourcevalue.SourceValues,
	ctx effectlowering.SignatureReturnContext,
) (product.Value, bool) {
	if pattern, ok := stringPatternLiteral(reg, resolver, ctx, 1); ok {
		captures := stringlib.CaptureTypes(pattern)
		if len(captures) == 0 {
			if ctx.Index == 0 {
				return returnValueWithType(reg, normalize.Optional(typ.String)), true
			}
			return typevalue.Nil(reg), true
		}
		if ctx.Index < len(captures) {
			return returnValueWithType(reg, stringlib.OptionalCaptureValue(captures[ctx.Index])), true
		}
		return typevalue.Nil(reg), true
	}
	return returnValueWithType(reg, stringlib.OptionalGeneralCaptureValue()), true
}

func stringFindReturnValue(
	reg *axis.Registry,
	resolver sourcevalue.SourceValues,
	ctx effectlowering.SignatureReturnContext,
) (product.Value, bool) {
	if ctx.Index < 2 {
		return returnValueWithType(reg, normalize.Optional(typ.Integer)), true
	}
	if plain, ok := stringBoolLiteralArg(reg, resolver, ctx, 3); ok && plain {
		return typevalue.Nil(reg), true
	}
	if pattern, ok := stringPatternLiteral(reg, resolver, ctx, 1); ok {
		captures := stringlib.CaptureTypes(pattern)
		captureIndex := ctx.Index - 2
		if captureIndex < len(captures) {
			return returnValueWithType(reg, stringlib.OptionalCaptureValue(captures[captureIndex])), true
		}
		return typevalue.Nil(reg), true
	}
	return returnValueWithType(reg, stringlib.OptionalGeneralCaptureValue()), true
}

func stringGMatchReturnValue(
	reg *axis.Registry,
	resolver sourcevalue.SourceValues,
	ctx effectlowering.SignatureReturnContext,
) (product.Value, bool) {
	if ctx.Index != 0 {
		return product.Value{}, false
	}
	if pattern, ok := stringPatternLiteral(reg, resolver, ctx, 1); ok {
		return returnValueWithType(reg, stringlib.GMatchIterator(stringlib.CaptureTypes(pattern))), true
	}
	return returnValueWithType(reg, stringlib.GeneralGMatchIterator()), true
}

func stringPatternLiteral(
	reg *axis.Registry,
	resolver sourcevalue.SourceValues,
	ctx effectlowering.SignatureReturnContext,
	signatureIndex int,
) (string, bool) {
	arg, ok := stringSignatureSourceAt(ctx, signatureIndex)
	if !ok {
		return "", false
	}
	value, ok := resolver.ValueOfSource(ctx.Node.Point, arg, ctx.In, ctx.Read)
	if !ok {
		return "", false
	}
	return typevalue.StringLiteralOf(reg, value)
}

func stringBoolLiteralArg(
	reg *axis.Registry,
	resolver sourcevalue.SourceValues,
	ctx effectlowering.SignatureReturnContext,
	signatureIndex int,
) (bool, bool) {
	arg, ok := stringSignatureSourceAt(ctx, signatureIndex)
	if !ok {
		return false, false
	}
	value, ok := resolver.ValueOfSource(ctx.Node.Point, arg, ctx.In, ctx.Read)
	if !ok {
		return false, false
	}
	t, ok := typevalue.WitnessOf(reg, value)
	if !ok {
		return false, false
	}
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.Boolean {
		return false, false
	}
	b, ok := lit.Value.(bool)
	return b, ok
}

func stringSignatureArgSourceIndex(ctx effectlowering.SignatureReturnContext, signatureIndex int) int {
	if ctx.Site.MethodName() != "" {
		return signatureIndex - 1
	}
	return signatureIndex
}

func stringSignatureSourceAt(ctx effectlowering.SignatureReturnContext, signatureIndex int) (factflow.ValueSource, bool) {
	if ctx.Site.MethodName() != "" && signatureIndex == 0 {
		return ctx.Site.ReceiverSource()
	}
	argIndex := stringSignatureArgSourceIndex(ctx, signatureIndex)
	if argIndex < 0 {
		return factflow.ValueSource{}, false
	}
	return ctx.Site.ArgumentSourceAt(argIndex)
}

func stringUnpackFormatLiteral(
	reg *axis.Registry,
	resolver sourcevalue.SourceValues,
	ctx effectlowering.SignatureReturnContext,
) (string, bool) {
	arg, ok := stringSignatureSourceAt(ctx, 0)
	if !ok {
		return "", false
	}
	value, ok := resolver.ValueOfSource(ctx.Node.Point, arg, ctx.In, ctx.Read)
	if !ok {
		return "", false
	}
	return typevalue.StringLiteralOf(reg, value)
}

func stringUnpackValueTypes(format string) ([]typ.Type, bool) {
	var out []typ.Type
	for i := 0; i < len(format); {
		switch format[i] {
		case ' ', '\n', '\r', '\t', '\v', '\f', '<', '>', '=':
			i++
		case '!':
			i++
			i = skipDecimalDigits(format, i)
		case 'x', 'X':
			i++
		case 'b', 'B', 'h', 'H', 'l', 'L', 'j', 'J', 'T':
			i++
			out = append(out, typ.Integer)
		case 'i', 'I':
			i++
			i = skipDecimalDigits(format, i)
			out = append(out, typ.Integer)
		case 'f', 'd', 'n':
			i++
			out = append(out, typ.Number)
		case 'c', 's':
			i++
			i = skipDecimalDigits(format, i)
			out = append(out, typ.String)
		case 'z':
			i++
			out = append(out, typ.String)
		default:
			return nil, false
		}
	}
	out = append(out, typ.Integer)
	return out, true
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
