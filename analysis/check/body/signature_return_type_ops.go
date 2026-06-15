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
		InstantiateGenericCall: func(fn *typ.Function, args []typ.Type) (*typ.Function, bool) {
			instantiated, violations := typecall.InstantiateGenericCall(fn, args)
			if len(violations) != 0 {
				return fn, false
			}
			return instantiated, instantiated != nil
		},
	}
}
