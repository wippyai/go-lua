package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/kind"
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

func staticScalarReturnType(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Literal:
		return true
	default:
		return false
	}
}
