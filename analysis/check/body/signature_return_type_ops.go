package body

import (
	"github.com/wippyai/go-lua/analysis/check/body/internal/typeprojection"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
)

func signatureReturnTypeOps() effectlowering.ReturnTypeOps {
	return effectlowering.ReturnTypeOps{
		CallableReturn: typecall.CallableReturn,
		ElementOf:      typeprojection.ElementOf,
		TypeProjection: typeprojection.Apply,
	}
}
