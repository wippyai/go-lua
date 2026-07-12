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

func signatureAllocationOperations(plan *operationplan.Plan, owner uint64) map[cfg.Point]operationplan.SignatureAllocationOperation {
	if plan == nil || owner == 0 {
		return nil
	}
	out := make(map[cfg.Point]operationplan.SignatureAllocationOperation)
	ordinals := make(map[string]uint32)
	for rawPoint := 0; rawPoint < plan.PointCount(); rawPoint++ {
		point := cfg.Point(rawPoint)
		call, ok := plan.SignatureCallOperation(point)
		if !ok {
			continue
		}
		template, exact := effectlowering.StaticSignatureAllocationTemplate(call.Signature())
		if !exact {
			continue
		}
		templateOwner := string(template.Root)
		ordinals[templateOwner]++
		op, ok := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{
			Owner: owner, Template: template.Root, Ordinal: ordinals[templateOwner],
		}, template)
		if ok {
			out[point] = op
		}
	}
	return out
}
