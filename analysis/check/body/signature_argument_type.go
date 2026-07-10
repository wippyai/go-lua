package body

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// SignatureArgumentTypeAtBoundary resolves a call argument through the same
// signature-argument provider used by call-outcome lowering. It is intended for
// diagnostics that need the contextual type of a function-expression argument.
func (r *Result) SignatureArgumentTypeAtBoundary(point cfg.Point, source factflow.ValueSource) (typ.Type, bool) {
	if r == nil || r.signatureArg == nil || r.registry == nil {
		return nil, false
	}
	graph := r.Graph()
	if graph == nil {
		return nil, false
	}
	in, ok := r.solvedStateAt(point)
	if !ok {
		return nil, false
	}
	return r.signatureArg(transfer.NodeContext{
		Graph:    graph,
		Registry: r.registry,
		Point:    point,
		Node:     graph.Node(point),
		Read:     r.boundaryRead,
	}, source, in, r.boundaryRead)
}
