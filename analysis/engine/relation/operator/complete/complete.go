package complete

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Execute closes one authenticated cofiber over the sealed Complete binding.
// The binding's Range is the sole output authority. Existing tuples preserve
// their issued lineage; missing denominator members receive schema-declared
// ProvenAbsent cells and the mounted row lineage issued for that member.
// Execute closes one Complete range using the exact denominator witness
// supplied by the caller. Correlated replay passes a q-specific posting from
// its PartitionDirectory; ordinary evaluation resolves the mounted global
// witness once and passes it here. Complete never resolves a witness by
// denominator reference internally.
func Execute(completeBinding arrangement.CompleteBinding, mounted witness.Mounted, input tuple.Batch, witnessValue binding.DenominatorWitness) (tuple.Batch, bool) {
	if !completeBinding.Available() || !mounted.Available() || !input.ValidFor(mounted) {
		return tuple.Batch{}, false
	}
	rangeAuthority, rangeOK := completeBinding.Range()
	if !rangeOK || !rangeAuthority.Available() || rangeAuthority.Kind() != algebra.KindComplete || rangeAuthority.Denominator() != completeBinding.Denominator() || rangeAuthority.Layout().Digest() != completeBinding.Key().Digest() {
		return tuple.Batch{}, false
	}
	layout := completeBinding.Key()
	denominator := completeBinding.Denominator()
	if !layout.Available() || !layout.ValidFor(mounted.Fence()) || !denominator.Available() || layout.Access().Relation() != denominator.Relation() || layout.Access().Key() != denominator.Key() || len(layout.KeyColumns()) == 0 {
		return tuple.Batch{}, false
	}
	if !witnessValue.Available() || !witnessValue.ValidFor(mounted.RuntimeFence()) || !witnessValue.Matches(denominator) {
		return tuple.Batch{}, false
	}
	columns := completeBinding.Columns()
	if len(columns) == 0 {
		return tuple.Batch{}, false
	}
	fiber := input.Scope()
	seen := make(map[model.RowID]tuple.Tuple, input.Len())
	for index := 0; index < input.Len(); index++ {
		value, valueOK := input.At(index)
		if !valueOK || !value.ValidFor(mounted) || !value.Scope().Same(fiber) {
			return tuple.Batch{}, false
		}
		row, rowOK := value.SourceFor(denominator.Relation())
		if !rowOK || !witnessValue.Contains(row) {
			return tuple.Batch{}, false
		}
		if _, duplicate := seen[row]; duplicate {
			return tuple.Batch{}, false
		}
		seen[row] = value
	}
	outputs := make([]tuple.Tuple, 0, witnessValue.Len())
	for index := 0; index < witnessValue.Len(); index++ {
		row, rowOK := witnessValue.At(index)
		if !rowOK {
			return tuple.Batch{}, false
		}
		value, present := seen[row]
		if !present {
			value, present = tuple.NewAbsent(mounted, witnessValue, fiber, row, columns)
		} else {
			value, present = tuple.ExtendAbsent(mounted, witnessValue, value, denominator.Relation(), columns)
		}
		if !present || !value.ValidFor(mounted) || !value.Scope().Same(fiber) {
			return tuple.Batch{}, false
		}
		outputs = append(outputs, value)
	}
	// Keep an empty output vector non-nil: the range constructor authenticates
	// empty Complete results under the binding rather than using a second Empty
	// representation. Correlation coordinates belong to the evaluation replay
	// witness, not to tuple state; replay retains the anchor coordinate while
	// this operator returns the ordinary sealed Complete range.
	return tuple.NewRangeBatch(mounted, rangeAuthority, fiber, outputs, witnessValue)
}
