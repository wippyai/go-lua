package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
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
	LengthIndexRef    ContainerRef
	LengthIndexOffset int64
	IndexSymbol       cfg.SymbolID
}

// RefineIndexRead removes nil when lowered evidence proves an in-bounds read.
func (f PointFacts) RefineIndexRead(q IndexReadRefinementQuery) ProductValue {
	container := q.Container.ProjectValue()
	result := q.Read.ProjectValue()
	if container == nil || result == nil {
		return ProductValue{State: StateUnknown}
	}
	resolved := func(refined typ.Type) ProductValue {
		if refined == nil {
			return ProductValue{State: StateUnknown}
		}
		return ProductValue{Value: product.FromType(refined), State: StateResolved}
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
