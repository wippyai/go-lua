package cfgbuild

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (b *builder) appendNode(state flowState, kind cfg.NodeKind) flowState {
	return b.appendNodeForStmt(state, kind, nil)
}

func (b *builder) appendNodeForStmt(state flowState, kind cfg.NodeKind, stmt ast.Stmt) flowState {
	if !state.live {
		return state
	}
	point := b.graph.AddNode(kind)
	b.connect(state, point)
	b.recordStmtPoint(stmt, point)
	return flowState{current: point, live: true}
}

func (b *builder) appendAssign(state flowState, stmt ast.Stmt) flowState {
	return b.appendNodeForStmt(state, cfg.NodeAssign, stmt)
}

func (b *builder) appendCall(state flowState, stmt ast.Stmt) flowState {
	return b.appendNodeForStmt(state, cfg.NodeCall, stmt)
}

func (b *builder) appendBranch(state flowState, stmt ast.Stmt) flowState {
	if !state.live {
		return state
	}
	point := b.graph.AddBranch()
	b.connect(state, point)
	b.recordStmtPoint(stmt, point)
	return flowState{current: point, live: true}
}

func (b *builder) recordStmtPoint(stmt ast.Stmt, point cfg.Point) {
	if stmt == nil {
		return
	}
	if b.stmtPoints == nil {
		b.stmtPoints = make(map[ast.Stmt][]cfg.Point)
	}
	b.stmtPoints[stmt] = append(b.stmtPoints[stmt], point)
}

func (b *builder) takePendingGotos(label string) []cfg.Point {
	if len(b.pendingGotos) == 0 {
		return nil
	}
	points := b.pendingGotos[label]
	delete(b.pendingGotos, label)
	return points
}

func (b *builder) materializePendingCond(state flowState) flowState {
	if !state.live || !state.pendingCond {
		return state
	}
	return b.appendNode(state, cfg.NodeNoop)
}

func (b *builder) connect(state flowState, to cfg.Point) {
	if !state.live {
		return
	}
	b.graph.AddEdge(state.current, to, state.edgeCond())
}

func (state flowState) edgeCond() bool {
	if state.pendingCond {
		return state.cond
	}
	return false
}

func (b *builder) firstNewEdgeTarget(edgeStart int, from cfg.Point, cond bool) (cfg.Point, bool) {
	edges := b.graph.Edges()
	if edgeStart < 0 || edgeStart > len(edges) {
		edgeStart = len(edges)
	}
	for _, edge := range edges[edgeStart:] {
		if edge.From == from && edge.Cond == cond {
			return edge.To, true
		}
	}
	return 0, false
}
