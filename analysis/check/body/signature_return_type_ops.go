package body

import (
	"github.com/wippyai/go-lua/analysis/engine/factapply/effectlowering"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/lua/typeprojection"
)

func signatureReturnTypeOps() effectlowering.ReturnTypeOps {
	return effectlowering.ReturnTypeOps{
		CallableReturn: typecall.CallableReturn,
		TypeProjection: typeprojection.Apply,
	}
}
