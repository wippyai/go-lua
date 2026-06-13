package body

import (
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/lua/typeprojection"
)

func signatureReturnTypeOps() effectlowering.ReturnTypeOps {
	return effectlowering.ReturnTypeOps{
		CallableReturn: typecall.CallableReturn,
		ElementOf:      typeprojection.ElementOf,
		TypeProjection: typeprojection.Apply,
	}
}
