package input

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	bindingpkg "github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// ExecuteExtent redeems one exact source-row extent of an Input producer.
//
// The source witness is the membership authority. If rows is non-nil, it
// must be the witness's ordered member vector; a nil rows argument asks this
// function to enumerate the witness directly without making a second row
// vector. Every member is redeemed with Reader.LookupRowID. The reader's
// common-cofiber rows are grouped by their exact normalized scope in callback
// order, so no physical partition or authored row order is reconstructed.
//
// emptyScope is used only for an empty source witness. It is still required
// to be an already-mounted scope: an empty extent carries one authenticated
// Input range rather than disappearing or acquiring a synthetic scope.
func ExecuteExtent(
	binding arrangement.InputBinding,
	mounted witness.Mounted,
	reader read.Reader,
	source bindingpkg.DenominatorWitness,
	rows []model.RowID,
	emptyScope witness.Scope,
) ([]tuple.Batch, bool) {
	if !binding.Available() || !mounted.Available() || !reader.Available() {
		return nil, false
	}
	rangeAuthority, rangeOK := binding.Range()
	if !rangeOK || !rangeAuthority.Available() {
		return nil, false
	}
	scan := binding.Scan()
	values := binding.Values()
	if !scan.ValidFor(mounted.Fence()) || !values.ValidFor(mounted.Fence()) ||
		!reader.Layout().Equal(values) ||
		reader.Layout().Access().Relation() != binding.Relation() ||
		rangeAuthority.Kind() != algebra.KindInput ||
		!rangeAuthority.Layout().Equal(scan) ||
		!source.ValidFor(mounted.RuntimeFence()) ||
		source.Relation() != binding.Relation() ||
		!emptyScope.ValidFor(mounted.RuntimeFence()) {
		return nil, false
	}
	if _, scopeOK := mounted.ScopeToken(emptyScope); !scopeOK {
		return nil, false
	}

	// Validate the complete ordered source extent before doing any state
	// lookup. This rejects duplicates, foreign members, and an omitted member
	// without allowing a partial result to become an accepted extent.
	count := source.Len()
	seen := make(map[model.RowID]struct{}, count)
	for index := 0; index < count; index++ {
		member, memberOK := source.At(index)
		if !memberOK || !member.Available() || member.Relation() != source.Relation() || !source.Contains(member) {
			return nil, false
		}
		if rows != nil {
			if index >= len(rows) || rows[index] != member {
				return nil, false
			}
		}
		if _, duplicate := seen[member]; duplicate {
			return nil, false
		}
		seen[member] = struct{}{}
	}
	if rows != nil && len(rows) != count {
		return nil, false
	}

	if count == 0 {
		empty, emptyOK := tuple.NewRangeBatch(mounted, rangeAuthority, emptyScope, []tuple.Tuple{}, bindingpkg.DenominatorWitness{})
		if !emptyOK || !empty.ValidFor(mounted) {
			return nil, false
		}
		return []tuple.Batch{empty}, true
	}

	type partition struct {
		scope  witness.Scope
		values []tuple.Tuple
	}
	partitions := make([]partition, 0)
	seenRows := make(map[model.RowID][]witness.Scope, count)
	for index := 0; index < count; index++ {
		rowID, rowOK := source.At(index)
		if !rowOK {
			return nil, false
		}
		if rows != nil {
			rowID = rows[index]
		}
		found := 0
		malformed := false
		completed, valid := reader.LookupRowID(rowID, func(row read.Row) bool {
			if row == nil || !row.Available() || !reader.Owns(row) ||
				row.ID() != rowID || row.ID().Relation() != binding.Relation() ||
				!source.Contains(row.ID()) || !row.Scope().ValidFor(mounted.RuntimeFence()) {
				malformed = true
				return false
			}
			for _, prior := range seenRows[rowID] {
				if prior.Same(row.Scope()) {
					// A source RowID may legitimately have more than one
					// normalized cofiber, but the same exact cofiber cannot be
					// emitted twice by one lookup.
					malformed = true
					return false
				}
			}
			value, valueOK := tuple.Input(mounted, reader, row)
			if !valueOK || !value.ValidFor(mounted) || !value.Scope().Same(row.Scope()) {
				malformed = true
				return false
			}
			seenRows[rowID] = append(seenRows[rowID], row.Scope())
			found++
			// Reader callbacks are already the authenticated cofiber
			// transport order. Coalesce only adjacent equal scopes: A/B/A
			// is three extents and must not be regrouped into A:[first,last].
			if len(partitions) == 0 || !partitions[len(partitions)-1].scope.Same(row.Scope()) {
				partitions = append(partitions, partition{scope: row.Scope(), values: []tuple.Tuple{value}})
			} else {
				last := len(partitions) - 1
				partitions[last].values = append(partitions[last].values, value)
			}
			return true
		})
		if malformed || !completed || !valid || found == 0 {
			// A valid directory member with no posting is not an empty
			// extent here: the exact source witness named a row that this
			// reader was required to redeem.
			return nil, false
		}
	}

	result := make([]tuple.Batch, len(partitions))
	for index, value := range partitions {
		if value.values == nil {
			return nil, false
		}
		batch, batchOK := tuple.NewRangeBatch(mounted, rangeAuthority, value.scope, value.values, bindingpkg.DenominatorWitness{})
		if !batchOK || !batch.ValidFor(mounted) {
			return nil, false
		}
		result[index] = batch
	}
	if result == nil {
		return nil, false
	}
	return result, true
}

// ExecuteExtentFromWitness is the no-copy convenience form of ExecuteExtent.
// It uses DenominatorWitness.At directly for the exact ordered source rows.
func ExecuteExtentFromWitness(
	binding arrangement.InputBinding,
	mounted witness.Mounted,
	reader read.Reader,
	source bindingpkg.DenominatorWitness,
	emptyScope witness.Scope,
) ([]tuple.Batch, bool) {
	return ExecuteExtent(binding, mounted, reader, source, nil, emptyScope)
}
