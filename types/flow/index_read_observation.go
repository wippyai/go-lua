package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// IndexReadObservationProofs exposes solved proof facts needed by the indexed
// read observation reducer.
type IndexReadObservationProofs interface {
	NumericBoundsAt(p cfg.Point, sym cfg.SymbolID) (lower, upper int64, ok bool)
	ArrayLenRefPathAt(p cfg.Point, sym cfg.SymbolID) (array constraint.Path, offset int64, ok bool)
	LengthBoundsAt(p cfg.Point, path constraint.Path) (lower, upper int64, ok bool)
	HasKeyOf(p cfg.Point, tablePath, keyPath constraint.Path) bool
	IndexReadPointFacts(p cfg.Point, view PathReadView) PointFacts
}

// IndexReadObservationQuery describes one AST-free indexed read observation.
type IndexReadObservationQuery struct {
	Point  cfg.Point
	View   PathReadView
	Result typ.Type
	Index  PathObservationIndexRead
	Proofs IndexReadObservationProofs
}

// RefineIndexReadObservation applies solved indexed-read proofs to an observed
// runtime read type. The reducer owns the canonical proof order shared by
// observation and synthesis: readback, key-presence, numeric bounds, length
// relation, length expression, literal length.
func RefineIndexReadObservation(q IndexReadObservationQuery) (typ.Type, bool) {
	if q.Result == nil || q.Proofs == nil {
		return nil, false
	}
	if refined, ok := refineObservationByIndexReadback(q); ok {
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

func refineObservationByIndexReadback(q IndexReadObservationQuery) (typ.Type, bool) {
	if q.Index.TablePath.IsEmpty() {
		return nil, false
	}
	if q.Index.KeyPath.IsEmpty() && !IndexWriteReadCanUseKeyValueOnly(q.Index.KeyType) {
		return nil, false
	}
	facts := q.Proofs.IndexReadPointFacts(q.Point, q.View)
	if admitted, ok := facts.DynamicIndexReadback(q.Index.DynamicReadbackQuery()); ok {
		readback := product.ProjectValueOrUnknown(admitted)
		if indexReadbackIsInformative(readback, true) {
			return refineReadbackPresence(q, readback), true
		}
	}
	return nil, false
}

func refineReadbackPresence(q IndexReadObservationQuery, readback typ.Type) typ.Type {
	if q.Proofs == nil || q.Index.TablePath.IsEmpty() || q.Index.KeyPath.IsEmpty() ||
		!q.Proofs.HasKeyOf(q.Point, q.Index.TablePath, q.Index.KeyPath) {
		return readback
	}
	if refined, ok := removeNil(readback); ok {
		return refined
	}
	return readback
}

func indexReadbackIsInformative(t typ.Type, ok bool) bool {
	return ok && !typ.IsAbsentOrUnknown(t) && !typ.IsAny(t)
}

func refineObservationByKeyPresence(q IndexReadObservationQuery) (typ.Type, bool) {
	if q.Index.TablePath.IsEmpty() || q.Index.KeyPath.IsEmpty() ||
		!q.Proofs.HasKeyOf(q.Point, q.Index.TablePath, q.Index.KeyPath) {
		return nil, false
	}
	return removeNil(q.Result)
}

func refineObservationTupleIndexByNumericBounds(q IndexReadObservationQuery) (typ.Type, bool) {
	if !q.Index.HasIndexVar || q.Index.IndexVarPath.Symbol == 0 {
		return nil, false
	}
	arity, ok := narrow.TupleArity(q.Index.Container)
	if !ok {
		return nil, false
	}
	lower, upper, ok := q.Proofs.NumericBoundsAt(q.Point, q.Index.IndexVarPath.Symbol)
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

func refineObservationSequenceIndexByLengthRelation(q IndexReadObservationQuery) (typ.Type, bool) {
	if !q.Index.HasIndexVar || q.Index.IndexVarPath.Symbol == 0 || q.Index.TablePath.IsEmpty() {
		return nil, false
	}
	lower, _, ok := q.Proofs.NumericBoundsAt(q.Point, q.Index.IndexVarPath.Symbol)
	if !ok || lower+q.Index.IndexVarOffset < 1 {
		return nil, false
	}
	arrPath, lenOffset, ok := q.Proofs.ArrayLenRefPathAt(q.Point, q.Index.IndexVarPath.Symbol)
	if !ok {
		return nil, false
	}
	if !arrPath.Equal(q.Index.TablePath) && arrPath.Key() != q.Index.TablePath.Key() {
		return nil, false
	}
	if lenOffset > -q.Index.IndexVarOffset {
		return nil, false
	}
	return refineSequenceIndex(q.Index.Container, q.Result, lower+q.Index.IndexVarOffset)
}

func refineObservationSequenceIndexByLengthExpr(q IndexReadObservationQuery) (typ.Type, bool) {
	if !q.Index.HasLength || q.Index.TablePath.IsEmpty() || !q.Index.LengthPath.Equal(q.Index.TablePath) {
		return nil, false
	}
	if arity, ok := narrow.TupleArity(q.Index.Container); ok {
		refined := narrow.RefineLengthIndex(q.Index.Container, q.Result, arity, q.Index.LengthOffset)
		if refined != nil {
			return refined, true
		}
	}
	lower, _, ok := q.Proofs.LengthBoundsAt(q.Point, q.Index.TablePath)
	if !ok {
		return nil, false
	}
	refined := narrow.RefineLengthIndex(q.Index.Container, q.Result, lower, q.Index.LengthOffset)
	return refined, refined != nil
}

func refineObservationSequenceIndexByLiteralLength(q IndexReadObservationQuery) (typ.Type, bool) {
	if !q.Index.HasLiteralIndex || q.Index.LiteralIndex < 1 || q.Index.TablePath.IsEmpty() {
		return nil, false
	}
	lower, _, ok := q.Proofs.LengthBoundsAt(q.Point, q.Index.TablePath)
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
