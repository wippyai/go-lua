package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// IndexReadRefinementQuery carries lowered in-bounds read evidence.
type IndexReadRefinementQuery struct {
	Container         product.AbstractValue
	Read              product.AbstractValue
	ContainerRef      ContainerRef
	LiteralIndex      int64
	ReadbackTarget    constraint.Path
	ReadbackKeyPath   constraint.Path
	ReadbackKeyValue  product.AbstractValue
	PresentTablePath  constraint.Path
	PresentKeyPath    constraint.Path
	LengthIndexRef    ContainerRef
	LengthIndexOffset int64
	IndexSymbol       cfg.SymbolID
}

// RefineIndexRead composes lowered point-state proofs for one indexed read:
// admitted dynamic-write readback, proven key presence, then in-bounds sequence
// narrowing. Callers lower syntax to paths, refs, and key values before entering
// this law.
func (f PointFacts) RefineIndexRead(q IndexReadRefinementQuery) ProductValue {
	if admitted, ok := f.DynamicIndexReadback(DynamicIndexReadbackQuery{
		Target:           q.ReadbackTarget,
		KeyPath:          q.ReadbackKeyPath,
		KeyValue:         q.ReadbackKeyValue,
		FollowKeyAliases: true,
	}); ok && !admitted.IsZero() {
		return ProductValue{Value: admitted, State: StateResolved}
	}

	var result typ.Type
	if !q.Read.IsZero() {
		result = q.Read.ProjectValue()
	}
	resolved := func(refined typ.Type) ProductValue {
		if refined == nil {
			return ProductValue{State: StateUnknown}
		}
		return ProductValue{Value: product.FromType(refined), State: StateResolved}
	}
	if present := f.ReadPresentKeyValue(PresentKeyReadQuery{
		TablePath: q.PresentTablePath,
		KeyPath:   q.PresentKeyPath,
		Result:    result,
	}); present.State == StateResolved {
		return present
	}
	if q.Container.IsZero() || result == nil {
		return ProductValue{State: StateUnknown}
	}
	container := q.Container.ProjectValue()
	if container == nil {
		return ProductValue{State: StateUnknown}
	}

	if q.LiteralIndex >= 1 {
		if arity, ok := narrow.TupleArity(container); ok && arity >= q.LiteralIndex {
			return resolved(narrow.RefineSequenceIndex(container, result, q.LiteralIndex))
		}
		if lower, _, ok := NumericLenBoundsForContainer(f.state.Num, q.ContainerRef); q.ContainerRef.IsValid() && ok && lower >= q.LiteralIndex {
			return resolved(narrow.RefineSequenceIndex(container, result, q.LiteralIndex))
		}
		return ProductValue{State: StateUnknown}
	}

	if q.LengthIndexRef.IsValid() && q.LengthIndexRef.Equal(q.ContainerRef) {
		if arity, ok := narrow.TupleArity(container); ok {
			return resolved(narrow.RefineLengthIndex(container, result, arity, q.LengthIndexOffset))
		}
		if lower, _, ok := NumericLenBoundsForContainer(f.state.Num, q.ContainerRef); ok {
			return resolved(narrow.RefineLengthIndex(container, result, lower, q.LengthIndexOffset))
		}
		return ProductValue{State: StateUnknown}
	}

	idxKey, ok := NumericVarKeyOfSymbol(q.IndexSymbol)
	if !ok {
		return ProductValue{State: StateUnknown}
	}

	if arity, ok := narrow.TupleArity(container); ok {
		if lower, upper, ok := numeric.BoundsForWithTheory(f.state.Num, idxKey); ok && lower >= 1 && upper <= arity {
			return resolved(narrow.RefineSequenceIndex(container, result, lower))
		}
	}

	if ref, offset, ok := NumericLenRefWithOffsetForVar(f.state.Num, q.IndexSymbol); q.ContainerRef.IsValid() && ok && ref.Equal(q.ContainerRef) {
		if lower, _, ok := numeric.BoundsForWithTheory(f.state.Num, idxKey); ok && lower+offset >= 1 && offset <= 0 {
			return resolved(narrow.RefineSequenceIndex(container, result, lower+offset))
		}
	}
	return ProductValue{State: StateUnknown}
}
