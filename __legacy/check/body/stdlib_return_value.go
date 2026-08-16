package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/domain/type/access"
	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	"github.com/wippyai/go-lua/analysis/domain/type/projection"
	"github.com/wippyai/go-lua/analysis/domain/type/stringlib"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func stdlibSignatureReturnValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	input effectlowering.SignatureOutcomeInputProgram,
) effectlowering.SignatureReturnValueProgram {
	if reg == nil || !input.Valid() {
		return effectlowering.SignatureReturnValueProgram{}
	}
	heapInput, err := input.WithHeapMemberQuery()
	if err != nil {
		panic(err)
	}
	operands, err := effectlowering.SealSignatureReturnValueProgram(input, func(site effectlowering.SignatureOutcomeSite) bool {
		return !stdlibTableUnpackResultSite(site)
	}, func(ctx effectlowering.SignatureReturnValueInputContext) (product.Value, bool) {
		switch {
		case ctx.Name == "type" && ctx.Index == 0 && bareGlobalCall(ctx.Site, "type"):
			return typeCallReturnValue(reg, typeValues, ctx)
		case ctx.Name == "select" && ctx.Index == 0 && bareGlobalCall(ctx.Site, "select"):
			return selectCountReturnValue(reg, ctx)
		case ctx.Name == "tonumber" && ctx.Index == 0 && bareGlobalCall(ctx.Site, "tonumber") && ctx.Site.ArgumentSourceCount() >= 2:
			return returnValueWithType(reg, normalize.Optional(typ.Integer)), true
		case ctx.Name == "string.byte":
			return stringByteReturnValue(reg), true
		case ctx.Name == "string.match":
			return stringMatchReturnValue(reg, ctx)
		case ctx.Name == "string.find":
			return stringFindReturnValue(reg, ctx)
		case ctx.Name == "string.gmatch" || ctx.Name == "string.gfind":
			return stringGMatchReturnValue(reg, ctx)
		case ctx.Name == "string.unpack":
			return stringUnpackReturnValue(reg, ctx)
		default:
			return product.Value{}, false
		}
	})
	if err != nil {
		panic(err)
	}
	heap, err := effectlowering.SealSignatureReturnValueProgram(heapInput, stdlibTableUnpackResultSite, func(ctx effectlowering.SignatureReturnValueInputContext) (product.Value, bool) {
		if ctx.Index != 0 {
			return product.Value{}, false
		}
		return tableUnpackFirstReturnValue(reg, typeValues, ctx)
	})
	if err != nil {
		panic(err)
	}
	return effectlowering.ComposeSignatureReturnValuePrograms(operands, heap)
}

func stdlibTableUnpackResultSite(site effectlowering.SignatureOutcomeSite) bool {
	if site.Name != "unpack" && site.Name != "table.unpack" {
		return false
	}
	observed := false
	site.Site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		if target.ResultIndex() == 0 {
			observed = true
			return false
		}
		return true
	})
	return observed
}

func typeCallReturnValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	ctx effectlowering.SignatureReturnValueInputContext,
) (product.Value, bool) {
	value, ok := ctx.Input.Argument(0)
	if !ok {
		return product.Value{}, false
	}
	return sourcevalue.LuaTypeNameValue(reg, typeValues, value)
}

func selectCountReturnValue(
	reg *axis.Registry,
	ctx effectlowering.SignatureReturnValueInputContext,
) (product.Value, bool) {
	value, ok := ctx.Input.Argument(0)
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
	ctx effectlowering.SignatureReturnValueInputContext,
) (product.Value, bool) {
	format, ok := stringUnpackFormatLiteral(reg, ctx)
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
	ctx effectlowering.SignatureReturnValueInputContext,
) (product.Value, bool) {
	value, ok := ctx.Input.Argument(0)
	if !ok {
		return product.Value{}, false
	}
	if first, ok, err := ctx.Input.HeapMember(value, []segment.Segment{{
		Kind:  segment.SegmentIndexInt,
		Index: 1,
	}}); err != nil {
		panic(err)
	} else if ok {
		return first, true
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
	ctx effectlowering.SignatureReturnValueInputContext,
) (product.Value, bool) {
	if pattern, ok := stringPatternLiteral(reg, ctx, 1); ok {
		returns := stringlib.MatchReturnTypes(pattern)
		if ctx.Index < len(returns) {
			return returnValueWithType(reg, returns[ctx.Index]), true
		}
		return typevalue.Nil(reg), true
	}
	return returnValueWithType(reg, stringlib.OptionalGeneralCaptureValue()), true
}

func stringFindReturnValue(
	reg *axis.Registry,
	ctx effectlowering.SignatureReturnValueInputContext,
) (product.Value, bool) {
	if ctx.Index < 2 {
		return returnValueWithType(reg, normalize.Optional(typ.Integer)), true
	}
	if plain, ok := stringBoolLiteralArg(reg, ctx, 3); ok && plain {
		return typevalue.Nil(reg), true
	}
	if pattern, ok := stringPatternLiteral(reg, ctx, 1); ok {
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
	ctx effectlowering.SignatureReturnValueInputContext,
) (product.Value, bool) {
	if ctx.Index != 0 {
		return product.Value{}, false
	}
	if pattern, ok := stringPatternLiteral(reg, ctx, 1); ok {
		return returnValueWithType(reg, stringlib.GMatchIterator(stringlib.CaptureTypes(pattern))), true
	}
	return returnValueWithType(reg, stringlib.GeneralGMatchIterator()), true
}

func stringPatternLiteral(
	reg *axis.Registry,
	ctx effectlowering.SignatureReturnValueInputContext,
	signatureIndex int,
) (string, bool) {
	value, ok := stringSignatureValueAt(ctx, signatureIndex)
	if !ok {
		return "", false
	}
	return typevalue.StringLiteralOf(reg, value)
}

func stringBoolLiteralArg(
	reg *axis.Registry,
	ctx effectlowering.SignatureReturnValueInputContext,
	signatureIndex int,
) (bool, bool) {
	value, ok := stringSignatureValueAt(ctx, signatureIndex)
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

func stringSignatureArgSourceIndex(ctx effectlowering.SignatureReturnValueInputContext, signatureIndex int) int {
	if ctx.Site.MethodName() != "" {
		return signatureIndex - 1
	}
	return signatureIndex
}

func stringSignatureValueAt(ctx effectlowering.SignatureReturnValueInputContext, signatureIndex int) (product.Value, bool) {
	if ctx.Site.MethodName() != "" && signatureIndex == 0 {
		return ctx.Input.Receiver()
	}
	argIndex := stringSignatureArgSourceIndex(ctx, signatureIndex)
	if argIndex < 0 {
		return product.Value{}, false
	}
	return ctx.Input.Argument(argIndex)
}

func stringUnpackFormatLiteral(
	reg *axis.Registry,
	ctx effectlowering.SignatureReturnValueInputContext,
) (string, bool) {
	value, ok := stringSignatureValueAt(ctx, 0)
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
