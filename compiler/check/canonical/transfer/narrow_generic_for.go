package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/flow"
)

func (t *Transfer) genericForBodyEdgeState(g *cfg.Graph, branch cfg.Point, out flow.PointState) flow.PointState {
	node := g.Node(branch)
	if node == nil || !node.LoopPreheaderSet {
		return out
	}
	info := g.Assign(node.LoopPreheader)
	if info == nil || len(info.IterExprs) == 0 {
		return out
	}
	iterCall, ok := info.IterExprs[0].(*ast.FuncCallExpr)
	if !ok {
		iterCall = nil
	}
	res := flow.ClonePointState(out)
	t.applyGenericForBinding(&res, info, iterCall, nil)
	return res
}
