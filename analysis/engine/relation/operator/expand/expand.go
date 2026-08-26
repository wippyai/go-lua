package expand

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	arrangementexpand "github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

type partition struct {
	scope  witness.Scope
	values []tuple.Tuple
}

// Execute redeems one owner-frozen C-to-R expansion over an ordered source
// range. For each C row, Evidence supplies the already-issued R key tokens in
// owner order; Reader.Lookup performs the exact physical key lookup and
// returns complete R rows. The output is the C tuple combined with each
// matching R tuple, partitioned only when the mounted cofiber scopes differ.
// Partitions are coalesced only when scopes are contiguous in encounter
// order; a later match with an earlier scope never moves ahead of an owner
// vector element.
//
// A missing candidate vector is malformed evidence and refuses. An
// authenticated empty vector, a key with no R row, or a contradictory scope
// is a valid no-selection and produces the source range's authenticated empty
// extent when the whole invocation has no matches. No Reader.Scan, owner
// callback, ordinal conversion, RowID fabrication, or fallback is present.
func Execute(evidence arrangementexpand.Evidence, mounted witness.Mounted, view geometry.Geometry, source tuple.Batch, reader read.Reader) ([]tuple.Batch, bool) {
	if !validIngress(evidence, mounted, view, source, reader) {
		return nil, false
	}
	contract := evidence.Contract()
	selected, selectedOK := mounted.Scope(contract.Scope())
	if !selectedOK || !selected.ValidFor(mounted.RuntimeFence()) {
		return nil, false
	}
	keyColumns := reader.Layout().KeyColumns()
	if len(keyColumns) != 1 || keyColumns[0] != contract.Key() {
		return nil, false
	}
	if reader.Layout().Access().Relation() != contract.Reader() || len(reader.Layout().Columns()) == 0 {
		return nil, false
	}

	// Keep one result partition per exact output scope. The source range proof
	// is retained for every partition; this is transport over one C range, not
	// a new relation or a second state store.
	partitions := make([]partition, 0)

	for sourceIndex := 0; sourceIndex < source.Len(); sourceIndex++ {
		candidate, candidateOK := source.At(sourceIndex)
		if !candidateOK || !candidate.ValidFor(mounted) {
			return nil, false
		}
		row, rowOK := candidate.SourceFor(contract.Candidate())
		if !rowOK || !row.Available() {
			return nil, false
		}
		// The frozen directory is keyed by the complete nominal C RowID. A
		// content digest alone is not a C→P correspondence: equal content in
		// another relation or occurrence must refuse rather than alias.
		vector, vectorOK := evidence.VectorAt(row)
		if !vectorOK || !vector.Available() {
			return nil, false
		}

		for keyIndex := 0; keyIndex < vector.KeyCount(); keyIndex++ {
			key, keyOK := vector.KeyAt(keyIndex)
			if !keyOK || !key.Available() || key.Type() != evidence.KeyType() || !key.ValidFor(mounted.RuntimeFence()) {
				return nil, false
			}
			lookup, lookupOK := reader.TupleFrom([]binding.ValueToken{key})
			if !lookupOK || !lookup.Available() {
				return nil, false
			}
			completed, valid := reader.Lookup(lookup, func(target read.Row) bool {
				if target == nil || !target.Available() || target.ID().Relation() != contract.Reader() {
					return false
				}
				// Expand's scope is the right-side port scope.  The compiler
				// deliberately omits a second Select node, so the physical
				// reader must enforce the same exact entailment law here.
				// An out-of-scope target is valid no-selection, never evidence
				// failure and never a default row.
				if !targetInPort(view, target, selected) {
					return true
				}
				// Expand preserves the candidate extent.  A target that covers
				// only part of that extent would require splitting C, which is
				// not this operator's contract; treat it as no-selection rather
				// than publishing a widened or fabricated scope.
				if !view.Entails(candidate.Scope(), target.Scope()) {
					return true
				}
				right, rightOK := tuple.Input(mounted, reader, target)
				if !rightOK || !right.ValidFor(mounted) || !right.Scope().Same(target.Scope()) {
					return false
				}
				combined, combineOK := tuple.Append(mounted, view, candidate, right)
				if !combineOK || !combined.ValidFor(mounted) || !combined.Scope().Same(candidate.Scope()) {
					return false
				}
				// The owner vector order is semantic.  Do not group by scope
				// globally: K1(A), K2(B), K3(A) must remain A, B, A.
				partitions = appendPartition(partitions, combined.Scope(), combined)
				return true
			})
			if !completed || !valid {
				return nil, false
			}
		}
	}

	if len(partitions) == 0 {
		empty, emptyOK := tuple.PreserveRange(mounted, source, source.Scope(), []tuple.Tuple{})
		if !emptyOK {
			return nil, false
		}
		return []tuple.Batch{empty}, true
	}
	result := make([]tuple.Batch, len(partitions))
	for index, partition := range partitions {
		batch, batchOK := tuple.PreserveRange(mounted, source, partition.scope, partition.values)
		if !batchOK {
			return nil, false
		}
		result[index] = batch
	}
	return result, true
}

// targetInPort is the one physical enforcement of Expand's right-side port
// scope. A target outside the sealed scope is a valid no-selection, not a
// malformed row and not a reason to widen the candidate scope.
func targetInPort(view geometry.Geometry, target read.Row, selected witness.Scope) bool {
	return target != nil && target.Available() && selected.Available() && view.Entails(target.Scope(), selected)
}

// appendPartition preserves encounter order while coalescing only adjacent
// equal scopes. It intentionally does not use a scope-keyed map: A/B/A is
// three ordered extents, not A:[first,last],B:[middle].
func appendPartition(partitions []partition, scope witness.Scope, value tuple.Tuple) []partition {
	if len(partitions) == 0 || !partitions[len(partitions)-1].scope.Same(scope) {
		return append(partitions, partition{scope: scope, values: []tuple.Tuple{value}})
	}
	last := len(partitions) - 1
	partitions[last].values = append(partitions[last].values, value)
	return partitions
}

func validIngress(evidence arrangementexpand.Evidence, mounted witness.Mounted, view geometry.Geometry, source tuple.Batch, reader read.Reader) bool {
	if !evidence.Available() || !mounted.Available() || !view.ValidFor(mounted) || !source.ValidFor(mounted) || !reader.Available() {
		return false
	}
	if !evidence.Fence().Same(mounted.RuntimeFence()) || !reader.Layout().ValidFor(mounted.Fence()) {
		return false
	}
	contract := evidence.Contract()
	if !contract.Available() || !evidence.KeyType().Available() || !sourceCandidate(source, contract.Candidate()) {
		return false
	}
	// The mounted owner vector already carries its authored order and sparse
	// extent; runtime only redeems that sealed evidence and its port scope.
	if !contract.Scope().Available() {
		return false
	}
	selected, selectedOK := mounted.Scope(contract.Scope())
	if !selectedOK || !selected.ValidFor(mounted.RuntimeFence()) {
		return false
	}
	if reader.Layout().Access().Relation() != contract.Reader() || reader.Layout().KeyWidth() != 1 || reader.Layout().KeyColumns()[0] != contract.Key() {
		return false
	}
	typeID, typeOK := reader.Type(contract.Key())
	return typeOK && typeID == evidence.KeyType()
}

func sourceCandidate(source tuple.Batch, candidate model.RelationID) bool {
	if !candidate.Available() || !source.Available() || source.Len() == 0 {
		return source.Available() && source.Len() == 0
	}
	for index := 0; index < source.Len(); index++ {
		value, ok := source.At(index)
		if !ok || !value.Available() {
			return false
		}
		if _, ok := value.SourceFor(candidate); !ok {
			return false
		}
	}
	return true
}
