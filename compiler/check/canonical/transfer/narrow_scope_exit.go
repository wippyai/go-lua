package transfer

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// exitGuard synthesizes the branch guard a then-exit / else-exit ScopeExit node
// carries for unsplit conditions. Modern CFGs carry CondOrigin, which is the
// authoritative link back to the branch; the backward walk is a legacy/degenerate
// fallback for graphs that only copied CondVar/CondCheck.
func exitGuard(g *cfg.Graph, pred cfg.Point) (*cfg.BranchInfo, cfg.Point, bool) {
	node := g.Node(pred)
	if node == nil || node.Kind != cfg.NodeScopeExit {
		return nil, 0, false
	}
	if node.CondOriginSet {
		info := g.Branch(node.CondOrigin)
		if info == nil {
			return nil, 0, false
		}
		return info, node.CondOrigin, true
	}
	if node.CondVar == 0 {
		return nil, 0, false
	}
	if node.CondCheck.Kind == cfg.CheckNone {
		return nil, 0, false
	}
	if info, branch, ok := originatingBranch(g, pred, node.CondVar, node.CondCheck); ok {
		return info, branch, true
	}
	return &cfg.BranchInfo{
		CondVar:    g.NameOf(node.CondVar),
		CondSymbol: node.CondVar,
		CondCheck:  node.CondCheck,
	}, 0, false
}

// originatingBranch finds the branch a ScopeExit copied its guard markers from and
// returns that branch's full BranchInfo, including the intact condition AST. This
// prevents a ScopeExit's root-only marker from narrowing the wrong path.
func originatingBranch(g *cfg.Graph, exit cfg.Point, condSym cfg.SymbolID, check cfg.CondCheck) (*cfg.BranchInfo, cfg.Point, bool) {
	seen := map[cfg.Point]bool{exit: true}
	frontier := append([]cfg.Point(nil), g.Predecessors(exit)...)
	for len(frontier) > 0 {
		var next []cfg.Point
		for _, p := range frontier {
			if seen[p] {
				continue
			}
			seen[p] = true
			if info := g.Branch(p); info != nil && info.CondSymbol == condSym && info.CondCheck == check {
				return info, p, true
			}
			next = append(next, g.Predecessors(p)...)
		}
		frontier = next
	}
	return nil, 0, false
}

// scopeExitGuardPathMutated reports whether the arm between branch and its
// ScopeExit wrote the path whose value the branch guard tested. A ScopeExit guard
// is historical; if the arm overwrote the tested path or an ancestor, reapplying it
// to the current store would refine a replacement value with an obsolete fact.
func (t *Transfer) scopeExitGuardPathMutated(g *cfg.Graph, branch, exit cfg.Point, info *cfg.BranchInfo) bool {
	if g == nil || info == nil {
		return false
	}
	sym := t.condTestSymbol(info)
	if sym == 0 {
		return false
	}
	segments := t.condTestSegments(info)
	seen := map[cfg.Point]bool{exit: true}
	frontier := append([]cfg.Point(nil), g.Predecessors(exit)...)
	for len(frontier) > 0 {
		p := frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]
		if p == branch || seen[p] {
			continue
		}
		seen[p] = true
		if t.assignmentWritesGuardPath(g.Assign(p), sym, segments) {
			return true
		}
		frontier = append(frontier, g.Predecessors(p)...)
	}
	return false
}

func (t *Transfer) assignmentWritesGuardPath(info *cfg.AssignInfo, sym cfg.SymbolID, segments []constraint.Segment) bool {
	if info == nil || sym == 0 {
		return false
	}
	for _, target := range info.Targets {
		path, ok := t.staticPathOfAssignTarget(target)
		if !ok || path.Symbol != sym {
			continue
		}
		if pathWriteInvalidatesGuard(path.Segments, segments) {
			return true
		}
	}
	return false
}

func pathWriteInvalidatesGuard(write, guard []constraint.Segment) bool {
	if len(write) > len(guard) {
		return false
	}
	for i := range write {
		if write[i] != guard[i] {
			return false
		}
	}
	return true
}
