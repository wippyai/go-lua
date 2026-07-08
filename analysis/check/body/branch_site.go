package body

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/compiler/ast"
)

type BranchKind uint8

const (
	BranchUnknown BranchKind = iota
	BranchIf
	BranchWhile
	BranchRepeat
	BranchShortCircuit
)

// BranchConditionFact is the body-owned source projection for a branch point.
// WIR owns the normalized predicate; this view only binds that predicate back
// to source topology and spans for diagnostics/readmodel consumers.
type BranchConditionFact struct {
	Kind BranchKind

	Stmt      ast.Stmt
	If        *ast.IfStmt
	While     *ast.WhileStmt
	Repeat    *ast.RepeatStmt
	Condition ast.Expr
	Check     branchcond.Check
}

type branchSite struct {
	kind      BranchKind
	stmt      ast.Stmt
	ifStmt    *ast.IfStmt
	whileStmt *ast.WhileStmt
	repeat    *ast.RepeatStmt
	condition ast.Expr
}

func (r *Result) branchSite(point cfg.Point) (branchSite, bool) {
	if r == nil {
		return branchSite{}, false
	}
	sites := r.branchSites()
	site, ok := sites[point]
	return site, ok
}

func (r *Result) branchSites() map[cfg.Point]branchSite {
	if r == nil {
		return nil
	}
	if r.queries.branchSitesOK {
		return r.queries.branchSites
	}
	out := r.computeBranchSites()
	r.queries.branchSites = out
	r.queries.branchSitesOK = true
	return out
}

func (r *Result) computeBranchSites() map[cfg.Point]branchSite {
	if r == nil || r.cfg == nil || r.cfg.Graph == nil {
		return nil
	}
	out := map[cfg.Point]branchSite{}
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch stmt := stmt.(type) {
			case *ast.IfStmt:
				r.addSourceBranchSite(out, stmt, BranchIf, stmt.Condition)
				walk(stmt.Then)
				walk(stmt.Else)
			case *ast.WhileStmt:
				r.addSourceBranchSite(out, stmt, BranchWhile, stmt.Condition)
				walk(stmt.Stmts)
			case *ast.RepeatStmt:
				walk(stmt.Stmts)
				r.addSourceBranchSite(out, stmt, BranchRepeat, stmt.Condition)
			case *ast.DoBlockStmt:
				walk(stmt.Stmts)
			case *ast.NumberForStmt:
				walk(stmt.Stmts)
			case *ast.GenericForStmt:
				walk(stmt.Stmts)
			}
		}
	}
	walk(r.sourceStmts)
	r.addShortCircuitBranchSites(out)
	return out
}

func (r *Result) addSourceBranchSite(out map[cfg.Point]branchSite, stmt ast.Stmt, kind BranchKind, condition ast.Expr) {
	if r == nil || r.cfg == nil || stmt == nil || condition == nil {
		return
	}
	point, ok := r.branchPointForStmt(stmt)
	if !ok {
		return
	}
	site := branchSite{kind: kind, stmt: stmt, condition: condition}
	switch stmt := stmt.(type) {
	case *ast.IfStmt:
		site.ifStmt = stmt
	case *ast.WhileStmt:
		site.whileStmt = stmt
	case *ast.RepeatStmt:
		site.repeat = stmt
	}
	out[point] = site
}

func (r *Result) branchPointForStmt(stmt ast.Stmt) (cfg.Point, bool) {
	if r == nil || r.cfg == nil || r.cfg.Graph == nil {
		return 0, false
	}
	points := r.cfg.StmtPoints.PointsFor(stmt)
	for i := len(points) - 1; i >= 0; i-- {
		point := points[i]
		if r.cfg.Graph.IsBranch(point) {
			return point, true
		}
	}
	return 0, false
}

func (r *Result) addShortCircuitBranchSites(out map[cfg.Point]branchSite) {
	if r == nil || r.cfg == nil || r.cfg.Graph == nil {
		return
	}
	points := r.cfg.Meta.ShortCircuitGuardPoints()
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
	for _, point := range points {
		guard, ok := r.cfg.Meta.ShortCircuitGuard(point)
		if !ok || guard.Condition == nil || !r.cfg.Graph.IsBranch(point) {
			continue
		}
		if _, exists := out[point]; exists {
			continue
		}
		out[point] = branchSite{
			kind:      BranchShortCircuit,
			stmt:      guard.Stmt,
			condition: guard.Condition,
		}
	}
}

func branchConditionFactFromSite(site branchSite, check branchcond.Check) BranchConditionFact {
	return BranchConditionFact{
		Kind:      site.kind,
		Stmt:      site.stmt,
		If:        site.ifStmt,
		While:     site.whileStmt,
		Repeat:    site.repeat,
		Condition: site.condition,
		Check:     cloneBranchConditionCheck(check),
	}
}

func cloneBranchConditionCheck(check branchcond.Check) branchcond.Check {
	check.Path = check.Path.Clone()
	check.OtherPath = check.OtherPath.Clone()
	return check
}
