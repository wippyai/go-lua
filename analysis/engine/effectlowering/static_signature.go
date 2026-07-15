package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/internal/typegraph"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/stringlib"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// StaticScalarSignatureReturns projects the first exact signature-Relation
// slice through the same return materializer used by SignatureOutcomeProvider.
// Only effect-free, non-generic scalar returns qualify. Borrow/mutation/
// iteration/operational effects and composite/contextual types fail closed.
func StaticScalarSignatureReturns(reg *axis.Registry, typeValues *typevalue.Cache, sig signature.Function) ([]product.Value, bool) {
	if reg == nil || sig.Type == nil || !sig.Effect.Pure() || (sig.OperationalEffects != nil && !sig.OperationalEffects.IsEmpty()) || len(sig.Type.TypeParams) != 0 {
		return nil, false
	}
	out := make([]product.Value, len(sig.Type.Returns))
	for i, ret := range sig.Type.Returns {
		if !staticScalarReturnType(ret) {
			return nil, false
		}
		out[i] = returnValueFromSignatureTypeCached(reg, typeValues, sig.Type, ret)
	}
	return out, true
}

// StaticScalarStringMethodReturns additionally proves that a method call's
// site-dependent result shape is represented exactly by sig. string.match is
// the exceptional stdlib method in this slice: captures change result arity,
// so only a scalar literal no-capture pattern can use its one-slot base
// signature. The caller must separately prove receiver authority and canonical
// method-name resolution; this projector validates shape, not name lookup.
func StaticScalarStringMethodReturns(reg *axis.Registry, typeValues *typevalue.Cache, sig signature.Function, site factflow.CallSiteView) ([]product.Value, bool) {
	if site.MethodName() == "" {
		return nil, false
	}
	if site.MethodName() != "match" {
		return StaticScalarSignatureReturns(reg, typeValues, sig)
	}
	refined, exact := RefineStaticStringMethodSignature(reg, sig, site)
	if !exact || !refined.Equals(sig) {
		return nil, false
	}
	if reg == nil || sig.Type == nil || !sig.Effect.Pure() || (sig.OperationalEffects != nil && !sig.OperationalEffects.IsEmpty()) || len(sig.Type.TypeParams) != 0 {
		return nil, false
	}
	out := make([]product.Value, len(sig.Type.Returns))
	for i, ret := range sig.Type.Returns {
		if !staticFiniteScalarReturnType(ret) {
			return nil, false
		}
		out[i] = returnValueFromSignatureTypeCached(reg, typeValues, sig.Type, ret)
	}
	return out, true
}

// RefineStaticStringMethodSignature returns an owned signature whose result
// tuple is exact for the represented call site. It never mutates a Function's
// exported slices because doing so would invalidate its canonical hash.
func RefineStaticStringMethodSignature(reg *axis.Registry, sig signature.Function, site factflow.CallSiteView) (signature.Function, bool) {
	if reg == nil || sig.Type == nil || site.MethodName() == "" {
		return signature.Function{}, false
	}
	refined := sig.Clone()
	if site.MethodName() != "match" {
		return refined, true
	}
	if site.Expanded() || site.OpenTail() || site.ResultTargetCount() != 1 {
		return signature.Function{}, false
	}
	target, ok := site.ResultTargetAt(0)
	if !ok || target.ResultIndex() != 0 {
		return signature.Function{}, false
	}
	patternSource, ok := site.ArgumentSourceAt(0)
	if !ok {
		return signature.Function{}, false
	}
	patternValue, ok := sourcevalue.StaticScalarValue(reg, patternSource)
	if !ok {
		return signature.Function{}, false
	}
	pattern, ok := typevalue.StringLiteralOf(reg, patternValue)
	if !ok || len(stringlib.CaptureTypes(pattern)) != 0 {
		return signature.Function{}, false
	}
	refined.Type = typ.RebuildFunction(typ.FunctionParts{
		TypeParams: refined.Type.TypeParams,
		Params:     refined.Type.Params,
		Variadic:   refined.Type.Variadic,
		Returns:    stringlib.MatchReturnTypes(pattern),
	})
	return refined, true
}

func staticScalarReturnType(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String:
		return true
	case kind.Literal:
		literal, ok := t.(*typ.Literal)
		if !ok {
			return false
		}
		switch literal.Base {
		case kind.Boolean, kind.Number, kind.Integer, kind.String:
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func staticFiniteScalarReturnType(t typ.Type) bool {
	scalar, productive := staticFiniteScalarReturnTypeSeen(t, &typegraph.Path{})
	return scalar && productive
}

func staticFiniteScalarReturnTypeSeen(t typ.Type, active *typegraph.Path) (bool, bool) {
	if t == nil {
		return false, true
	}
	if !active.Enter(t) {
		return true, false
	}
	defer active.Leave(t)
	switch tt := t.(type) {
	case *typ.Annotated:
		if tt.Inner == nil || tt.Inner == t {
			return false, false
		}
		return staticFiniteScalarReturnTypeSeen(tt.Inner, active)
	case *typ.Alias:
		return staticFiniteScalarReturnTypeSeen(tt.UnaliasedTarget(), active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return false, false
		}
		return staticFiniteScalarReturnTypeSeen(expanded, active)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return false, false
		}
		return staticFiniteScalarReturnTypeSeen(tt.Body, active)
	}
	switch t.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String:
		return true, true
	case kind.Literal:
		literal, ok := t.(*typ.Literal)
		if !ok {
			return false, true
		}
		switch literal.Base {
		case kind.Boolean, kind.Number, kind.Integer, kind.String:
			return true, true
		default:
			return false, true
		}
	case kind.Optional:
		optional, ok := t.(*typ.Optional)
		if !ok {
			return false, true
		}
		return staticFiniteScalarReturnTypeSeen(optional.Inner, active)
	case kind.Union:
		union, ok := t.(*typ.Union)
		if !ok || len(union.Members) == 0 {
			return false, true
		}
		productive := false
		for _, member := range union.Members {
			scalar, memberProductive := staticFiniteScalarReturnTypeSeen(member, active)
			if !memberProductive {
				continue
			}
			productive = true
			if !scalar {
				return false, true
			}
		}
		return true, productive
	default:
		return false, true
	}
}
