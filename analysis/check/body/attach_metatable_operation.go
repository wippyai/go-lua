package body

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func attachMetatableOperations(graph cfg.Graph, facts factflow.Facts, identity *signatureIdentityResolver) map[cfg.Point]operationplan.AttachMetatableOperation {
	if graph == nil || identity == nil {
		return nil
	}
	out := make(map[cfg.Point]operationplan.AttachMetatableOperation)
	for point := cfg.Point(0); int(point) < graph.Size(); point++ {
		site, ok := facts.CallSiteView(point)
		if !ok || site.ArgumentSourceCount() != 2 {
			continue
		}
		ctx := transfer.NodeContext{Graph: graph, Point: point, Node: graph.Node(point)}
		if !identity.canonicalStdlibGlobalCall(ctx, site, "setmetatable") {
			continue
		}
		table, tableOK := site.ArgumentSourceAt(0)
		metatable, metatableOK := site.ArgumentSourceAt(1)
		if !tableOK || !metatableOK {
			continue
		}
		op, ok := operationplan.NewAttachMetatableOperation(table, metatable)
		if ok {
			out[point] = op
		}
	}
	return out
}
