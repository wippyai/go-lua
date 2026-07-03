package body

import (
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func signatureReturnTypeOps() effectlowering.ReturnTypeOps {
	return effectlowering.ReturnTypeOps{
		CallableReturn: typecall.CallableReturn,
		ElementOf:      projection.ElementOf,
		TypeProjection: luatypeprojection.Apply,
		InstantiateGenericCall: func(fn *typ.Function, args []typ.Type) (effectlowering.GenericCallInstantiation, bool) {
			instantiated, violations, bindings := typecall.InstantiateGenericCallWithBindings(fn, args)
			if len(violations) != 0 {
				return effectlowering.GenericCallInstantiation{}, false
			}
			out := effectlowering.GenericCallInstantiation{Type: instantiated}
			for _, binding := range bindings {
				out.TypeParams = append(out.TypeParams, binding.Param)
				out.TypeArgs = append(out.TypeArgs, binding.Type)
			}
			return out, instantiated != nil
		},
	}
}
