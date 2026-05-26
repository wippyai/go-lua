// Package indexread projects indexed read results through solved flow proofs.
package indexread

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/numparse"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Flow is the solved proof surface needed to refine indexed reads.
type Flow interface {
	BoundsAt(p cfg.Point, name string) (lower, upper int64, ok bool)
	ArrayLenBoundWithOffsetAt(p cfg.Point, varName string) (arrKey string, offset int64, ok bool)
	LengthBoundsAt(p cfg.Point, path constraint.Path) (lower, upper int64, ok bool)
	HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool
}

// PathOf maps an expression to its canonical flow path at the read point.
type PathOf func(ast.Expr) constraint.Path

// Query describes one indexed read projection.
type Query struct {
	Point     cfg.Point
	Container typ.Type
	Result    typ.Type
	Object    ast.Expr
	Key       ast.Expr
	Flow      Flow
	PathOf    PathOf
}

// Refine returns a result type refined by solved index-read proofs.
func Refine(q Query) (typ.Type, bool) {
	if q.Result == nil || q.Flow == nil {
		return nil, false
	}
	if refined, ok := refineByKeyPresence(q); ok {
		return refined, true
	}
	if refined, ok := refineTupleIndexByNumericBounds(q); ok {
		return refined, true
	}
	if refined, ok := refineSequenceIndexByLengthRelation(q); ok {
		return refined, true
	}
	if refined, ok := refineSequenceIndexByLengthExpr(q); ok {
		return refined, true
	}
	if refined, ok := refineSequenceIndexByLiteralLength(q); ok {
		return refined, true
	}
	return nil, false
}

func refineByKeyPresence(q Query) (typ.Type, bool) {
	if q.PathOf == nil {
		return nil, false
	}
	tablePath := q.PathOf(q.Object)
	keyPath := q.PathOf(q.Key)
	if tablePath.IsEmpty() || keyPath.IsEmpty() || !q.Flow.HasKeyOf(q.Point, tablePath, keyPath) {
		return nil, false
	}
	return removeNil(q.Result)
}

func refineTupleIndexByNumericBounds(q Query) (typ.Type, bool) {
	name, offset, ok := indexVarOffsetFromExpr(q.Key)
	if !ok {
		return nil, false
	}
	tuple, ok := unwrap.Alias(q.Container).(*typ.Tuple)
	if !ok || tuple == nil || len(tuple.Elements) == 0 {
		return nil, false
	}
	lower, upper, ok := q.Flow.BoundsAt(q.Point, name)
	if !ok {
		return nil, false
	}
	lower += offset
	upper += offset
	if lower < 1 || upper > int64(len(tuple.Elements)) {
		return nil, false
	}
	return removeNil(q.Result)
}

func refineSequenceIndexByLengthRelation(q Query) (typ.Type, bool) {
	name, offset, ok := indexVarOffsetFromExpr(q.Key)
	if !ok || q.PathOf == nil {
		return nil, false
	}
	lower, _, ok := q.Flow.BoundsAt(q.Point, name)
	if !ok || lower+offset < 1 {
		return nil, false
	}
	arrKey, lenOffset, ok := q.Flow.ArrayLenBoundWithOffsetAt(q.Point, name)
	if !ok {
		return nil, false
	}
	tablePath := q.PathOf(q.Object)
	if tablePath.IsEmpty() || string(tablePath.Key()) != arrKey {
		return nil, false
	}
	if lenOffset > -offset {
		return nil, false
	}
	return refineSequenceIndex(q.Container, q.Result, lower+offset)
}

func refineSequenceIndexByLengthExpr(q Query) (typ.Type, bool) {
	if q.PathOf == nil {
		return nil, false
	}
	tablePath := q.PathOf(q.Object)
	if tablePath.IsEmpty() {
		return nil, false
	}
	lenPath, offset, ok := lenIndexPathFromExpr(q.Key, q.PathOf)
	if !ok || !lenPath.Equal(tablePath) {
		return nil, false
	}
	lower, _, ok := q.Flow.LengthBoundsAt(q.Point, tablePath)
	if !ok {
		return nil, false
	}
	refined := narrow.RefineLengthIndex(q.Container, q.Result, lower, offset)
	return refined, refined != nil
}

func refineSequenceIndexByLiteralLength(q Query) (typ.Type, bool) {
	index, ok := integerLiteralIndex(q.Key)
	if !ok || index < 1 || q.PathOf == nil {
		return nil, false
	}
	tablePath := q.PathOf(q.Object)
	if tablePath.IsEmpty() {
		return nil, false
	}
	lower, _, ok := q.Flow.LengthBoundsAt(q.Point, tablePath)
	if !ok || lower < index {
		return nil, false
	}
	return refineSequenceIndex(q.Container, q.Result, index)
}

func refineSequenceIndex(container, result typ.Type, index int64) (typ.Type, bool) {
	refined := narrow.RefineSequenceIndex(container, result, index)
	return refined, refined != nil
}

func removeNil(t typ.Type) (typ.Type, bool) {
	if !narrow.NilPresenceIsOnlyFlowUncertainty(t) {
		return nil, false
	}
	refined := narrow.RemoveNil(t)
	return refined, refined != nil && !typ.IsNever(refined) && !typ.TypeEquals(refined, t)
}

func integerLiteralIndex(expr ast.Expr) (int64, bool) {
	num, ok := expr.(*ast.NumberExpr)
	if !ok {
		return 0, false
	}
	return numparse.ParseIntegerLiteral(num.Value)
}

func indexVarOffsetFromExpr(expr ast.Expr) (string, int64, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if e.Value == "" {
			return "", 0, false
		}
		return e.Value, 0, true
	case *ast.ArithmeticOpExpr:
		ident, ok := e.Lhs.(*ast.IdentExpr)
		if !ok || ident.Value == "" {
			return "", 0, false
		}
		if e.Operator != "+" && e.Operator != "-" {
			return "", 0, false
		}
		k, ok := intConstFromExpr(e.Rhs)
		if !ok {
			return "", 0, false
		}
		if e.Operator == "-" {
			k = -k
		}
		return ident.Value, k, true
	}
	return "", 0, false
}

func lenIndexPathFromExpr(expr ast.Expr, paths PathOf) (constraint.Path, int64, bool) {
	switch e := expr.(type) {
	case *ast.UnaryLenOpExpr:
		path := paths(e.Expr)
		return path, 0, !path.IsEmpty()
	case *ast.ArithmeticOpExpr:
		if e.Operator != "+" && e.Operator != "-" {
			return constraint.Path{}, 0, false
		}
		path, offset, ok := lenIndexPathFromExpr(e.Lhs, paths)
		if !ok {
			return constraint.Path{}, 0, false
		}
		k, ok := intConstFromExpr(e.Rhs)
		if !ok {
			return constraint.Path{}, 0, false
		}
		if e.Operator == "-" {
			k = -k
		}
		return path, offset + k, true
	}
	return constraint.Path{}, 0, false
}

func intConstFromExpr(expr ast.Expr) (int64, bool) {
	switch v := expr.(type) {
	case *ast.NumberExpr:
		return numparse.ParseIntegerLiteral(v.Value)
	case *ast.UnaryMinusOpExpr:
		if n, ok := intConstFromExpr(v.Expr); ok {
			return -n, true
		}
	}
	return 0, false
}
