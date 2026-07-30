package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

type dominatingMemberReadPresenceSource struct {
	point    cfg.Point
	receiver pathdom.Path
}

// DominatingRequiredMemberReadProvesPathPresent reports whether an earlier
// required member read proves target present on every normal path to point.
// The proof is intentionally strict: reads at point itself cannot justify the
// obligation currently being checked.
func (r *Result) DominatingRequiredMemberReadProvesPathPresent(point cfg.Point, target pathdom.Path) bool {
	if r == nil || target.IsEmpty() {
		return false
	}
	key, ok := newDominatingMemberReadPresenceKey(r.pathValueKeySpace(), point, target)
	if !ok {
		return r.dominatingRequiredMemberReadProvesPathPresent(point, target)
	}
	if r.queries.memberReadPresence != nil {
		if cached, ok := r.queries.memberReadPresence[key]; ok {
			return cached
		}
	}
	proven := r.dominatingRequiredMemberReadProvesPathPresent(point, target)
	if r.queries.memberReadPresence == nil {
		r.queries.memberReadPresence = make(map[dominatingMemberReadPresenceKey]bool)
	}
	r.queries.memberReadPresence[key] = proven
	return proven
}

func (r *Result) dominatingRequiredMemberReadProvesPathPresent(point cfg.Point, target pathdom.Path) bool {
	if r == nil || target.IsEmpty() {
		return false
	}
	for _, source := range r.dominatingMemberReadPresenceSources() {
		if source.point == point ||
			source.receiver.IsEmpty() ||
			!r.memberReadPresenceSourceMatchesTarget(source, target) ||
			!r.PointDominates(source.point, point) ||
			!r.PointCanReach(source.point, point) ||
			r.PathPresenceInvalidatedBetween(source.point, point, target) {
			continue
		}
		return true
	}
	return false
}

func (r *Result) dominatingMemberReadPresenceSources() []dominatingMemberReadPresenceSource {
	if r == nil {
		return nil
	}
	if r.queries.memberReadSourcesOK {
		return r.queries.memberReadSources
	}
	graph := r.Graph()
	if graph == nil {
		r.queries.memberReadSourcesOK = true
		return nil
	}
	var out []dominatingMemberReadPresenceSource
	for _, point := range graph.RPO() {
		if !r.PointNormallyReachable(point) {
			continue
		}
		r.collectRequiredMemberReadPresenceSourcesAtPoint(point, &out)
	}
	out = r.dedupeMemberReadPresenceSources(out)
	r.queries.memberReadSources = out
	r.queries.memberReadSourcesOK = true
	return out
}

func (r *Result) dedupeMemberReadPresenceSources(in []dominatingMemberReadPresenceSource) []dominatingMemberReadPresenceSource {
	if len(in) < 2 {
		return in
	}
	ks := r.pathValueKeySpace()
	seen := make(map[dominatingMemberReadPresenceKey]struct{}, len(in))
	out := in[:0]
	for _, source := range in {
		key, ok := newDominatingMemberReadPresenceKey(ks, source.point, source.receiver)
		if !ok {
			out = append(out, source)
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, source)
	}
	return out
}

func (r *Result) memberReadPresenceSourceMatchesTarget(source dominatingMemberReadPresenceSource, target pathdom.Path) bool {
	return source.receiver.EqualIgnoringVersion(target)
}

func (r *Result) collectRequiredMemberReadPresenceSourcesAtPoint(point cfg.Point, out *[]dominatingMemberReadPresenceSource) {
	for _, use := range r.expressionUsesAt(point) {
		if use.Role == ExpressionUseOrdinaryAssignmentTarget {
			continue
		}
		r.collectRequiredExprMemberReadPresenceSources(point, use.Expr, out)
	}
	for _, lhs := range r.assignmentLValueUsesAt(point) {
		r.collectRequiredLValueMemberReadPresenceSources(point, lhs, out)
	}
	if fact, ok := r.ExpressionEvaluation(point); ok {
		r.collectRequiredExprMemberReadPresenceSources(point, fact.Expr, out)
	}
	if fact, ok := r.NumericFor(point); ok {
		r.collectRequiredExprMemberReadPresenceSources(point, fact.Init, out)
		r.collectRequiredExprMemberReadPresenceSources(point, fact.Limit, out)
		r.collectRequiredExprMemberReadPresenceSources(point, fact.Step, out)
	}
	if fact, ok := r.GenericFor(point); ok {
		for _, expr := range fact.Exprs {
			r.collectRequiredExprMemberReadPresenceSources(point, expr, out)
		}
	}
}

func (r *Result) collectRequiredLValueMemberReadPresenceSources(point cfg.Point, expr ast.Expr, out *[]dominatingMemberReadPresenceSource) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.AttrGetExpr:
		r.collectRequiredExprMemberReadPresenceSources(point, e.Object, out)
		if e.KeySyntax == ast.AttrKeyIndex {
			r.collectRequiredExprMemberReadPresenceSources(point, e.Key, out)
		}
		r.collectAttrReceiverPresenceSource(point, e, out)
	case *ast.CastExpr:
		r.collectRequiredLValueMemberReadPresenceSources(point, e.Expr, out)
	case *ast.NonNilAssertExpr:
		r.collectRequiredLValueMemberReadPresenceSources(point, e.Expr, out)
	default:
		r.collectRequiredExprMemberReadPresenceSources(point, expr, out)
	}
}

func (r *Result) collectRequiredExprMemberReadPresenceSources(point cfg.Point, expr ast.Expr, out *[]dominatingMemberReadPresenceSource) {
	switch e := expr.(type) {
	case nil:
		return
	case *ast.AttrGetExpr:
		r.collectRequiredExprMemberReadPresenceSources(point, e.Object, out)
		if e.KeySyntax == ast.AttrKeyIndex {
			r.collectRequiredExprMemberReadPresenceSources(point, e.Key, out)
		}
		r.collectAttrReceiverPresenceSource(point, e, out)
	case *ast.FuncCallExpr:
		r.collectRequiredExprMemberReadPresenceSources(point, e.Func, out)
		r.collectRequiredExprMemberReadPresenceSources(point, e.Receiver, out)
		for _, arg := range e.Args {
			r.collectRequiredExprMemberReadPresenceSources(point, arg, out)
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex {
				r.collectRequiredExprMemberReadPresenceSources(point, field.Key, out)
			}
			r.collectRequiredExprMemberReadPresenceSources(point, field.Value, out)
		}
	case *ast.LogicalOpExpr:
		// The right side of `and`/`or` is conditional, so only the left side is
		// required on every normal continuation through this expression.
		r.collectRequiredExprMemberReadPresenceSources(point, e.Lhs, out)
	case *ast.RelationalOpExpr:
		r.collectRequiredExprMemberReadPresenceSources(point, e.Lhs, out)
		r.collectRequiredExprMemberReadPresenceSources(point, e.Rhs, out)
	case *ast.StringConcatOpExpr:
		r.collectRequiredExprMemberReadPresenceSources(point, e.Lhs, out)
		r.collectRequiredExprMemberReadPresenceSources(point, e.Rhs, out)
	case *ast.ArithmeticOpExpr:
		r.collectRequiredExprMemberReadPresenceSources(point, e.Lhs, out)
		r.collectRequiredExprMemberReadPresenceSources(point, e.Rhs, out)
	case *ast.UnaryMinusOpExpr:
		r.collectRequiredExprMemberReadPresenceSources(point, e.Expr, out)
	case *ast.UnaryNotOpExpr:
		r.collectRequiredExprMemberReadPresenceSources(point, e.Expr, out)
	case *ast.UnaryLenOpExpr:
		r.collectRequiredExprMemberReadPresenceSources(point, e.Expr, out)
	case *ast.UnaryBNotOpExpr:
		r.collectRequiredExprMemberReadPresenceSources(point, e.Expr, out)
	case *ast.CastExpr:
		r.collectRequiredExprMemberReadPresenceSources(point, e.Expr, out)
	case *ast.NonNilAssertExpr:
		r.collectRequiredExprMemberReadPresenceSources(point, e.Expr, out)
	}
}

func (r *Result) collectAttrReceiverPresenceSource(point cfg.Point, attr *ast.AttrGetExpr, out *[]dominatingMemberReadPresenceSource) {
	if attr == nil || attr.Object == nil {
		return
	}
	receiver, ok := r.ExpressionPath(attr.Object)
	if !ok || receiver.IsEmpty() {
		return
	}
	*out = append(*out, dominatingMemberReadPresenceSource{point: point, receiver: receiver})
}
