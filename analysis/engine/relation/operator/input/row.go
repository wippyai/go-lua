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

// ExecuteRow redeems one exact source RowID at one exact authenticated
// cofiber scope. The source witness proves membership; the supplied scope
// names which one of the reader's common-cofiber rows is the population
// extent. Other cofibers for the same RowID are valid reader results and are
// skipped, while a second match for the requested scope is malformed.
//
// No relation scan, scope inference, singleton witness, semantic cell check,
// or fallback is performed here. The resulting tuple is lowered by the
// ordinary Input boundary and retains the sealed Input range authority.
func ExecuteRow(
	binding arrangement.InputBinding,
	mounted witness.Mounted,
	reader read.Reader,
	source bindingpkg.DenominatorWitness,
	rowID model.RowID,
	scope witness.Scope,
) (tuple.Batch, bool) {
	if !binding.Available() || !mounted.Available() || !reader.Available() ||
		!rowID.Available() || !source.ValidFor(mounted.RuntimeFence()) ||
		!source.Contains(rowID) || source.Relation() != binding.Relation() ||
		rowID.Relation() != binding.Relation() || !scope.ValidFor(mounted.RuntimeFence()) {
		return tuple.Batch{}, false
	}
	if _, scopeOK := mounted.ScopeToken(scope); !scopeOK {
		return tuple.Batch{}, false
	}
	rangeAuthority, rangeOK := binding.Range()
	if !rangeOK || !rangeAuthority.Available() || rangeAuthority.Kind() != algebra.KindInput {
		return tuple.Batch{}, false
	}
	scan := binding.Scan()
	values := binding.Values()
	if !scan.ValidFor(mounted.Fence()) || !values.ValidFor(mounted.Fence()) ||
		!reader.Layout().Equal(values) ||
		reader.Layout().Access().Relation() != binding.Relation() ||
		!rangeAuthority.Layout().Equal(scan) {
		return tuple.Batch{}, false
	}

	var result tuple.Tuple
	found := false
	malformed := false
	completed, valid := reader.LookupRowID(rowID, func(candidate read.Row) bool {
		if candidate == nil || !candidate.Available() || !reader.Owns(candidate) ||
			candidate.ID() != rowID || candidate.ID().Relation() != binding.Relation() ||
			!source.Contains(candidate.ID()) || !candidate.Scope().ValidFor(mounted.RuntimeFence()) {
			malformed = true
			return false
		}
		if !candidate.Scope().Same(scope) {
			return true
		}
		if found {
			malformed = true
			return false
		}
		value, valueOK := tuple.Input(mounted, reader, candidate)
		if !valueOK || !value.ValidFor(mounted) || !value.Scope().Same(scope) {
			malformed = true
			return false
		}
		result = value
		found = true
		return true
	})
	if malformed || !completed || !valid || !found {
		return tuple.Batch{}, false
	}
	batch, batchOK := tuple.NewRangeBatch(mounted, rangeAuthority, scope, []tuple.Tuple{result}, bindingpkg.DenominatorWitness{})
	if !batchOK || !batch.ValidFor(mounted) {
		return tuple.Batch{}, false
	}
	return batch, true
}
