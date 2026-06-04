// Package indexread projects indexed read results through solved flow proofs.
package indexread

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	flowfacts "github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/numparse"
	"github.com/wippyai/go-lua/types/typ"
)

// Flow is the solved proof surface needed to refine indexed reads.
type Flow interface {
	BoundsAt(p cfg.Point, name string) (lower, upper int64, ok bool)
	ArrayLenBoundWithOffsetAt(p cfg.Point, varName string) (arrKey string, offset int64, ok bool)
	LengthBoundsAt(p cfg.Point, path constraint.Path) (lower, upper int64, ok bool)
	HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool
}

// PathOf maps an expression to its flow path at the read point.
type PathOf func(ast.Expr) constraint.Path

type IndexWriteAdmissionFlow interface {
	IndexWriteAdmission(q flowfacts.IndexWriteQuery) (typ.Type, bool)
}

// Query describes one indexed read projection.
type Query struct {
	Point     cfg.Point
	Container typ.Type
	Result    typ.Type
	Object    ast.Expr
	Key       ast.Expr
	KeyType   typ.Type
	Flow      Flow
	PathOf    PathOf
}

// ContextQuery lowers one indexed-read expression into an AST-free proof
// context.
type ContextQuery struct {
	Container typ.Type
	Object    ast.Expr
	Key       ast.Expr
	KeyType   typ.Type
	PathOf    PathOf
}

// ObservationQuery refines an already-selected path observation through a
// normalized indexed-read proof context.
type ObservationQuery struct {
	Point  cfg.Point
	Result typ.Type
	Index  flowfacts.PathObservationIndexRead
	Flow   Flow
}

// Refine returns a result type refined by solved index-read proofs.
func Refine(q Query) (typ.Type, bool) {
	index, ok := Context(ContextQuery{
		Container: q.Container,
		Object:    q.Object,
		Key:       q.Key,
		KeyType:   q.KeyType,
		PathOf:    q.PathOf,
	})
	if !ok {
		return nil, false
	}
	return RefineObservation(ObservationQuery{
		Point:  q.Point,
		Result: q.Result,
		Index:  index,
		Flow:   q.Flow,
	})
}

// Context returns an AST-free indexed-read context. The returned context is
// usable when at least one proof shape was recognized.
func Context(q ContextQuery) (flowfacts.PathObservationIndexRead, bool) {
	var out flowfacts.PathObservationIndexRead
	out.Container = q.Container
	out.KeyType = q.KeyType
	if q.PathOf != nil {
		out.TablePath = q.PathOf(q.Object)
		out.KeyPath = q.PathOf(q.Key)
	}
	if path, offset, ok := indexVarOffsetPathFromExpr(q.Key, q.PathOf); ok {
		out.IndexVarPath = path
		out.IndexVarOffset = offset
		out.HasIndexVar = true
	}
	if path, offset, ok := lenIndexPathFromExpr(q.Key, q.PathOf); ok {
		out.LengthPath = path
		out.LengthOffset = offset
		out.HasLength = true
	}
	if index, ok := integerLiteralIndex(q.Key); ok {
		out.LiteralIndex = index
		out.HasLiteralIndex = true
	}
	return out, !out.TablePath.IsEmpty() || !out.KeyPath.IsEmpty() || out.HasIndexVar || out.HasLength || out.HasLiteralIndex
}

// RefineObservation returns a result type refined by solved index-read proofs
// from a normalized context.
func RefineObservation(q ObservationQuery) (typ.Type, bool) {
	if q.Result == nil || q.Flow == nil {
		return nil, false
	}
	if refined, ok := refineObservationByIndexWriteAdmission(q); ok {
		return refined, true
	}
	if refined, ok := refineObservationByKeyPresence(q); ok {
		return refined, true
	}
	if refined, ok := refineObservationTupleIndexByNumericBounds(q); ok {
		return refined, true
	}
	if refined, ok := refineObservationSequenceIndexByLengthRelation(q); ok {
		return refined, true
	}
	if refined, ok := refineObservationSequenceIndexByLengthExpr(q); ok {
		return refined, true
	}
	if refined, ok := refineObservationSequenceIndexByLiteralLength(q); ok {
		return refined, true
	}
	return nil, false
}

func refineObservationByIndexWriteAdmission(q ObservationQuery) (typ.Type, bool) {
	flow, ok := q.Flow.(IndexWriteAdmissionFlow)
	if !ok || q.Index.TablePath.IsEmpty() {
		return nil, false
	}
	if q.Index.KeyPath.IsEmpty() && !indexWriteReadCanUseKeyValueOnly(q.Index.KeyType) {
		return nil, false
	}
	admitted, ok := flow.IndexWriteAdmission(flowfacts.IndexWriteQuery{
		Point:     q.Point,
		Target:    q.Index.TablePath,
		KeyPath:   q.Index.KeyPath,
		KeySymbol: q.Index.KeyPath.Symbol,
		KeyType:   q.Index.KeyType,
	})
	if !ok || typ.IsAbsentOrUnknown(admitted) || typ.IsAny(admitted) {
		return nil, false
	}
	return admitted, true
}

func refineObservationByKeyPresence(q ObservationQuery) (typ.Type, bool) {
	if q.Index.TablePath.IsEmpty() || q.Index.KeyPath.IsEmpty() || !q.Flow.HasKeyOf(q.Point, q.Index.TablePath, q.Index.KeyPath) {
		return nil, false
	}
	return removeNil(q.Result)
}

func refineObservationTupleIndexByNumericBounds(q ObservationQuery) (typ.Type, bool) {
	if !q.Index.HasIndexVar || q.Index.IndexVarPath.Root == "" {
		return nil, false
	}
	arity, ok := narrow.TupleArity(q.Index.Container)
	if !ok {
		return nil, false
	}
	lower, upper, ok := q.Flow.BoundsAt(q.Point, q.Index.IndexVarPath.Root)
	if !ok {
		return nil, false
	}
	lower += q.Index.IndexVarOffset
	upper += q.Index.IndexVarOffset
	if lower < 1 || upper > arity {
		return nil, false
	}
	return removeNil(q.Result)
}

func refineObservationSequenceIndexByLengthRelation(q ObservationQuery) (typ.Type, bool) {
	if !q.Index.HasIndexVar || q.Index.IndexVarPath.Root == "" || q.Index.TablePath.IsEmpty() {
		return nil, false
	}
	lower, _, ok := q.Flow.BoundsAt(q.Point, q.Index.IndexVarPath.Root)
	if !ok || lower+q.Index.IndexVarOffset < 1 {
		return nil, false
	}
	arrKey, lenOffset, ok := q.Flow.ArrayLenBoundWithOffsetAt(q.Point, q.Index.IndexVarPath.Root)
	if !ok {
		return nil, false
	}
	if string(q.Index.TablePath.Key()) != arrKey {
		return nil, false
	}
	if lenOffset > -q.Index.IndexVarOffset {
		return nil, false
	}
	return refineSequenceIndex(q.Index.Container, q.Result, lower+q.Index.IndexVarOffset)
}

func refineObservationSequenceIndexByLengthExpr(q ObservationQuery) (typ.Type, bool) {
	if !q.Index.HasLength || q.Index.TablePath.IsEmpty() || !q.Index.LengthPath.Equal(q.Index.TablePath) {
		return nil, false
	}
	if arity, ok := narrow.TupleArity(q.Index.Container); ok {
		refined := narrow.RefineLengthIndex(q.Index.Container, q.Result, arity, q.Index.LengthOffset)
		if refined != nil {
			return refined, true
		}
	}
	lower, _, ok := q.Flow.LengthBoundsAt(q.Point, q.Index.TablePath)
	if !ok {
		return nil, false
	}
	refined := narrow.RefineLengthIndex(q.Index.Container, q.Result, lower, q.Index.LengthOffset)
	return refined, refined != nil
}

func refineObservationSequenceIndexByLiteralLength(q ObservationQuery) (typ.Type, bool) {
	if !q.Index.HasLiteralIndex || q.Index.LiteralIndex < 1 || q.Index.TablePath.IsEmpty() {
		return nil, false
	}
	lower, _, ok := q.Flow.LengthBoundsAt(q.Point, q.Index.TablePath)
	if !ok || lower < q.Index.LiteralIndex {
		return nil, false
	}
	return refineSequenceIndex(q.Index.Container, q.Result, q.Index.LiteralIndex)
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

func indexWriteReadCanUseKeyValueOnly(keyType typ.Type) bool {
	if keyType == nil || typ.IsAbsentOrUnknown(keyType) {
		return false
	}
	return typ.UnwrapAnnotated(keyType).Kind() == kind.Literal
}

func integerLiteralIndex(expr ast.Expr) (int64, bool) {
	num, ok := expr.(*ast.NumberExpr)
	if !ok {
		return 0, false
	}
	return numparse.ParseIntegerLiteral(num.Value)
}

func indexVarOffsetPathFromExpr(expr ast.Expr, paths PathOf) (constraint.Path, int64, bool) {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		if e.Value == "" {
			return constraint.Path{}, 0, false
		}
		if paths != nil {
			path := paths(e)
			if !path.IsEmpty() {
				return path, 0, true
			}
		}
		return constraint.Path{Root: e.Value}, 0, true
	case *ast.ArithmeticOpExpr:
		ident, ok := e.Lhs.(*ast.IdentExpr)
		if !ok || ident.Value == "" {
			return constraint.Path{}, 0, false
		}
		if e.Operator != "+" && e.Operator != "-" {
			return constraint.Path{}, 0, false
		}
		k, ok := intConstFromExpr(e.Rhs)
		if !ok {
			return constraint.Path{}, 0, false
		}
		if e.Operator == "-" {
			k = -k
		}
		if paths != nil {
			path := paths(ident)
			if !path.IsEmpty() {
				return path, k, true
			}
		}
		return constraint.Path{Root: ident.Value}, k, true
	}
	return constraint.Path{}, 0, false
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
