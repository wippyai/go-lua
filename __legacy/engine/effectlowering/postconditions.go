package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// SignatureNoNormalReturnConfig carries signature lookup plus the generic fact
// model needed to mark calls that cannot complete normally.
type SignatureNoNormalReturnConfig struct {
	Graph      cfg.Graph
	Registry   *axis.Registry
	Signatures SignatureLookup
	NameFor    SignatureNameFunc
}

// SignatureNoNormalReturnPredicate returns a call-site predicate for declared
// signatures that cannot complete normally. The consuming lowerer decides where
// to store the resulting fact.
func SignatureNoNormalReturnPredicate(config SignatureNoNormalReturnConfig) func(cfg.Point, factflow.CallSiteView) bool {
	if config.Graph == nil || config.Registry == nil || config.Signatures == nil || config.NameFor == nil {
		return nil
	}
	return func(point cfg.Point, site factflow.CallSiteView) bool {
		name, ok := config.NameFor(transfer.NodeContext{
			Graph:    config.Graph,
			Registry: config.Registry,
			Point:    point,
			Node:     config.Graph.Node(point),
		}, factflow.NewCallProducerFromView(site))
		if !ok {
			return false
		}
		sig, ok := config.Signatures.Lookup(name)
		return ok && signatureHasNoNormalReturn(sig)
	}
}

func signatureHasNoNormalReturn(sig signature.Function) bool {
	return sig.Type != nil && len(sig.Type.Returns) == 1 && typ.IsNever(sig.Type.Returns[0])
}
