package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// SignatureNoNormalReturnConfig carries signature lookup plus the generic fact
// model needed to mark calls that cannot complete normally.
type SignatureNoNormalReturnConfig struct {
	Graph      cfg.Graph
	Registry   *axis.Registry
	Signatures SignatureLookup
	NameFor    SignatureNameFunc
	Facts      factflow.Facts
}

// WithSignatureNoNormalReturns returns Facts extended with call points whose
// declared signature has no normal return value.
func WithSignatureNoNormalReturns(config SignatureNoNormalReturnConfig) factflow.Facts {
	if config.Graph == nil || config.Registry == nil || config.Signatures == nil || config.NameFor == nil {
		return config.Facts
	}
	points := signatureNoNormalReturnFacts(config)
	return config.Facts.WithNoNormalReturns(points)
}

func signatureNoNormalReturnFacts(config SignatureNoNormalReturnConfig) map[cfg.Point]struct{} {
	out := make(map[cfg.Point]struct{})
	for _, point := range config.Graph.RPO() {
		site, ok := config.Facts.CallSite(point)
		if !ok {
			continue
		}
		name, ok := config.NameFor(transfer.NodeContext{
			Graph:    config.Graph,
			Registry: config.Registry,
			Point:    point,
			Node:     config.Graph.Node(point),
		}, callproducer.FromSite(site))
		if !ok {
			continue
		}
		sig, ok := config.Signatures.Lookup(name)
		if !ok || !signatureHasNoNormalReturn(sig) {
			continue
		}
		out[point] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func signatureHasNoNormalReturn(sig signature.Function) bool {
	return sig.Type != nil && len(sig.Type.Returns) == 1 && typ.IsNever(sig.Type.Returns[0])
}
