package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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
	// RawLoadWitness proves this is a direct physical-table member read, not
	// a metatable lookup. Codegen must require it before issuing a hoist license.
	RawLoadWitness bool
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
		if !r.staticMemberReadHasRawLoadWitness(occ) {
			return true
		}
		visited = true
		return visit(InvariantLoopReadOccurrence{
			Point:          occ.Point,
			LoopHead:       loop.Head,
			ReadLabel:      occ.ReadLabel,
			ReceiverPath:   occ.ReceiverPath,
			ReadPath:       occ.ReadPath,
			ReceiverType:   occ.ReceiverTypeBeforeBoundary,
			ReadSpan:       occ.Span,
			LoopSpan:       loop.Span,
			RawLoadWitness: true,
		})
	})
	return visited
}

// staticMemberReadHasRawLoadWitness proves that the read cannot enter Lua's
// metatable lookup path. A declared record alone is insufficient because a
// runtime cast can hide a stateful __index metamethod. The receiver must retain
// an exact heap identity whose root has a no-__index metatable proof, and the
// addressed member must be physically present at that identity.
func (r *Result) staticMemberReadHasRawLoadWitness(occ StaticMemberReadOccurrence) bool {
	if r == nil || r.registry == nil || !occ.HasReceiverValueBeforeBoundary ||
		!occ.HasReceiverTypeBeforeBoundary || occ.ReceiverTypeBeforeBoundary == nil ||
		!occ.HasReceiverPath || !occ.HasReadPath {
		return false
	}
	if len(occ.ReadPath.Segments) != len(occ.ReceiverPath.Segments)+1 {
		return false
	}
	id, ok := identityvalue.ExactID(r.registry, occ.ReceiverValueBeforeBoundary)
	if !ok {
		return false
	}
	st, ok := r.StateAt(occ.Point)
	if !ok {
		return false
	}
	object := st.ReadHeapTableObject(r.registry, id)
	root := object.Root()
	rootID, ok := identityvalue.ExactID(r.registry, root)
	if !ok || rootID != id || product.Equal(r.registry, product.Meet(r.registry, root, occ.ReceiverValueBeforeBoundary), product.Bottom(r.registry)) {
		return false
	}
	rootType, ok := typevalue.TypeOf(r.registry, root)
	if !ok || !rawTableTypeHasNoIndexMetamethod(rootType) ||
		!rawTableTypeHasNoIndexMetamethod(occ.ReceiverTypeBeforeBoundary) {
		return false
	}
	memberKey, ok := heapidentity.StaticMemberSuffixKey(r.KeySpace(), occ.ReadPath.Segments[len(occ.ReceiverPath.Segments):])
	if !ok {
		return false
	}
	member, ok := object.StaticMember(memberKey)
	return ok && presence.Equal(product.PresenceOf(member), presence.Present()) && !typevalue.HasOnlyNilType(r.registry, member)
}

func rawTableTypeHasNoIndexMetamethod(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	switch value := unwrap.Annotated(t).(type) {
	case *typ.Record:
		return value.Metatable == nil || TypeFieldProvablyAbsent(value.Metatable, "__index")
	case *typ.Union:
		if len(value.Members) == 0 {
			return false
		}
		for _, member := range value.Members {
			if !rawTableTypeHasNoIndexMetamethod(member) {
				return false
			}
		}
		return true
	case *typ.Optional:
		return rawTableTypeHasNoIndexMetamethod(value.Inner)
	case *typ.Alias:
		return rawTableTypeHasNoIndexMetamethod(value.UnaliasedTarget())
	case *typ.Recursive:
		return value.Body != nil && value.Body != t && rawTableTypeHasNoIndexMetamethod(value.Body)
	default:
		return false
	}
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
