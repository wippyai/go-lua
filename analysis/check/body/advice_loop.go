package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// LoopInfo is the body-owned source projection of one natural loop.
type LoopInfo struct {
	Head cfg.Point
	Span SourceSpan
}

// InvariantLoopReadOccurrence is a loop-contained static member/index read
// whose read path is stable through the loop and whose receiver is non-nil.
type InvariantLoopReadOccurrence struct {
	Point        cfg.Point
	LoopHead     cfg.Point
	ReadLabel    string
	ReceiverPath pathdom.Path
	ReadPath     pathdom.Path
	ReceiverType typ.Type
	ReadSpan     SourceSpan
	LoopSpan     SourceSpan
}

// ForEachInvariantLoopReadOccurrence visits loop-contained member/index reads
// whose read path is stable through the loop and whose receiver is non-nil.
func (r *Result) ForEachInvariantLoopReadOccurrence(visit func(InvariantLoopReadOccurrence) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	visited := false
	r.ForEachStaticMemberReadOccurrence(func(occ StaticMemberReadOccurrence) bool {
		if !occ.HasReceiverPath || occ.ReceiverPath.IsEmpty() ||
			!occ.HasReadPath || occ.ReadPath.IsEmpty() ||
			!occ.HasReceiverTypeBeforeBoundary ||
			occ.ReceiverTypeBeforeBoundary == nil ||
			typ.IsTopLike(occ.ReceiverTypeBeforeBoundary) ||
			typ.IsNever(occ.ReceiverTypeBeforeBoundary) ||
			typevalue.TypeIncludesNil(occ.ReceiverTypeBeforeBoundary) {
			return true
		}
		loop, ok := r.InnermostLoopForPoint(occ.Point)
		if !ok {
			return true
		}
		if r.PathInvalidatedInLoop(loop.Head, occ.ReadPath) {
			return true
		}
		visited = true
		return visit(InvariantLoopReadOccurrence{
			Point:        occ.Point,
			LoopHead:     loop.Head,
			ReadLabel:    occ.ReadLabel,
			ReceiverPath: occ.ReceiverPath,
			ReadPath:     occ.ReadPath,
			ReceiverType: occ.ReceiverTypeBeforeBoundary,
			ReadSpan:     occ.Span,
			LoopSpan:     loop.Span,
		})
	})
	return visited
}

// InnermostLoopForPoint returns the innermost source loop whose CFG cycle
// contains point. Loop condition/head reads are excluded; advice currently
// targets reads in the loop body.
func (r *Result) InnermostLoopForPoint(point cfg.Point) (LoopInfo, bool) {
	if r == nil || r.cfg == nil || r.cfg.Graph == nil || point == 0 {
		return LoopInfo{}, false
	}
	var found LoopInfo
	var ok bool
	r.forEachSourceLoop(func(loop LoopInfo) bool {
		if loop.Head == point {
			return true
		}
		if r.PointCanReach(loop.Head, point) && r.PointCanReach(point, loop.Head) {
			found = loop
			ok = true
		}
		return true
	})
	return found, ok
}

func (r *Result) forEachSourceLoop(visit func(LoopInfo) bool) bool {
	if r == nil || visit == nil || r.cfg == nil {
		return false
	}
	visited := false
	var walk func([]ast.Stmt) bool
	walk = func(stmts []ast.Stmt) bool {
		for _, stmt := range stmts {
			switch stmt := stmt.(type) {
			case *ast.WhileStmt:
				if !r.visitSourceLoop(stmt, visit, &visited) || !walk(stmt.Stmts) {
					return false
				}
			case *ast.RepeatStmt:
				if !r.visitSourceLoop(stmt, visit, &visited) || !walk(stmt.Stmts) {
					return false
				}
			case *ast.NumberForStmt:
				if !r.visitSourceLoop(stmt, visit, &visited) || !walk(stmt.Stmts) {
					return false
				}
			case *ast.GenericForStmt:
				if !r.visitSourceLoop(stmt, visit, &visited) || !walk(stmt.Stmts) {
					return false
				}
			case *ast.IfStmt:
				if !walk(stmt.Then) || !walk(stmt.Else) {
					return false
				}
			case *ast.DoBlockStmt:
				if !walk(stmt.Stmts) {
					return false
				}
			}
		}
		return true
	}
	walk(r.sourceStmts)
	return visited
}

func (r *Result) visitSourceLoop(stmt ast.Stmt, visit func(LoopInfo) bool, visited *bool) bool {
	head, ok := r.branchPointForStmt(stmt)
	if !ok {
		return true
	}
	*visited = true
	return visit(LoopInfo{
		Head: head,
		Span: sourceSpanFromAST(ast.SpanOf(stmt)),
	})
}

// PathInvalidatedInLoop reports whether target is written or invalidated by a
// point in the loop cycle headed by loopHead.
func (r *Result) PathInvalidatedInLoop(loopHead cfg.Point, target pathdom.Path) bool {
	if r == nil || target.IsEmpty() {
		return false
	}
	graph := r.Graph()
	if graph == nil {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == loopHead || !r.PointNormallyReachable(candidate) {
			continue
		}
		if !r.PointCanReach(loopHead, candidate) || !r.PointCanReach(candidate, loopHead) {
			continue
		}
		if invalidation, ok := r.PathDescendantInvalidation(candidate); ok && target.HasStrictPrefix(invalidation.ContainerPath()) {
			return true
		}
		if r.assignmentInvalidatesMemberPathAt(candidate, target) {
			return true
		}
		if r.CallMayInvalidateGuardFact(candidate, target) {
			return true
		}
	}
	return false
}
