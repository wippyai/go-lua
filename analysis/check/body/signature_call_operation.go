package body

import (
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func signatureCallOperations(graph cfg.Graph, facts factflow.Facts, producer *effectlowering.SignatureProducer) map[cfg.Point]operationplan.SignatureCallOperation {
	if graph == nil || producer == nil {
		return nil
	}
	out := make(map[cfg.Point]operationplan.SignatureCallOperation, facts.CallSiteCount())
	for point := cfg.Point(0); int(point) < graph.Size(); point++ {
		site, ok := facts.CallSiteView(point)
		if !ok {
			continue
		}
		sig, ok := producer.SignatureForSite(transfer.NodeContext{Point: point}, site)
		if !ok {
			continue
		}
		op, ok := operationplan.NewSignatureCallOperation(sig)
		if !ok {
			continue
		}
		out[point] = op
	}
	return out
}
