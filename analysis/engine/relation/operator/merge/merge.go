// Package merge implements the sealed relational Merge operator. Alternatives
// are reduced by the tuple authority; this package only chooses key/identity
// groups and preserves their ordered scope partitions.
package merge

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Execute merges alternative tuple batches under the exact producer Node,
// which redeems its sealed MergeBinding. Every input batch is one exact cofiber;
// contradictory cofibers are not conjoined or widened, but remain separate
// output batches. Within each cofiber, equal declared keys must carry the same
// owner-issued RowID and are reduced by the tuple package's canonical
// algebra/lineage authority.
//
// Output batches and key ranges retain the sealed input order: one batch is
// emitted for each distinct (scope, key) range, even when adjacent ranges
// share a scope. No callback stream, local row/cell representation, runtime
// layout choice, fallback scan, or identity derivation is allowed.
func Execute(binding arrangement.MergeBinding, mounted witness.Mounted, inputs []tuple.Batch) ([]tuple.Batch, bool) {
	if !binding.Available() || !mounted.Available() || len(inputs) == 0 {
		return nil, false
	}
	rangeAuthority, rangeOK := binding.Range()
	if !rangeOK || !rangeAuthority.Available() || rangeAuthority.Kind() != algebra.KindMerge {
		return nil, false
	}
	layout := binding.Key()
	if !layout.Available() || !layout.ValidFor(mounted.Fence()) || !layout.Access().Key().Available() || len(layout.KeyColumns()) == 0 {
		return nil, false
	}
	keyColumns := layout.KeyColumns()
	type keyGroup struct {
		key    tuple.Tuple
		row    model.RowID
		values []tuple.Tuple
	}
	type scopeGroup struct {
		scope  witness.Scope
		groups []keyGroup
	}
	scopes := make([]scopeGroup, 0)
	for _, input := range inputs {
		if !input.ValidFor(mounted) || !input.Scope().ValidFor(mounted.RuntimeFence()) {
			return nil, false
		}
		scopeIndex := -1
		for index := range scopes {
			if scopes[index].scope.Same(input.Scope()) {
				scopeIndex = index
				break
			}
		}
		if scopeIndex < 0 {
			scopes = append(scopes, scopeGroup{scope: input.Scope()})
			scopeIndex = len(scopes) - 1
		}
		for index := 0; index < input.Len(); index++ {
			value, ok := input.At(index)
			if !ok || !validValue(value, mounted, layout.Access().Relation(), keyColumns) {
				return nil, false
			}
			row, ok := value.SourceFor(layout.Access().Relation())
			if !ok {
				return nil, false
			}
			matched := -1
			for position := range scopes[scopeIndex].groups {
				candidate := &scopes[scopeIndex].groups[position]
				if tuple.SameKey(mounted, candidate.key, value, keyColumns) {
					matched = position
					if candidate.row != row {
						return nil, false
					}
					break
				}
			}
			if matched < 0 {
				scopes[scopeIndex].groups = append(scopes[scopeIndex].groups, keyGroup{key: value, row: row, values: []tuple.Tuple{value}})
			} else {
				scopes[scopeIndex].groups[matched].values = append(scopes[scopeIndex].groups[matched].values, value)
			}
		}
	}

	// Keep one output batch per sealed key range. A single Batch per scope
	// would erase the Group/Apply range boundary and force a downstream
	// consumer to rediscover the very key partition the seal already proved.
	result := make([]tuple.Batch, 0)
	for _, scope := range scopes {
		for _, group := range scope.groups {
			merged, ok := tuple.Merge(mounted, group.values, keyColumns)
			if !ok || !merged.ValidFor(mounted) || !merged.Scope().Same(scope.scope) {
				return nil, false
			}
			keys, keysOK := keyValues(group.key, mounted, keyColumns)
			if !keysOK {
				return nil, false
			}
			batch, ok := tuple.NewKeyRangeBatch(mounted, rangeAuthority, scope.scope, keys, []tuple.Tuple{merged})
			if !ok {
				return nil, false
			}
			result = append(result, batch)
		}
	}
	return result, true
}

func keyValues(value tuple.Tuple, mounted witness.Mounted, columns []model.ColumnID) ([]binding.ValueToken, bool) {
	if !value.ValidFor(mounted) || len(columns) == 0 {
		return nil, false
	}
	result := make([]binding.ValueToken, len(columns))
	for index, column := range columns {
		cell, ok := value.CellFor(column)
		if !ok || !cell.Value().Available() || !cell.Value().ValidFor(mounted.RuntimeFence()) {
			return nil, false
		}
		result[index] = cell.Value()
	}
	return result, true
}

func validValue(value tuple.Tuple, mounted witness.Mounted, relation model.RelationID, keyColumns []model.ColumnID) bool {
	if !value.ValidFor(mounted) || !relation.Available() || value.SourceLen() == 0 || len(keyColumns) == 0 {
		return false
	}
	row, ok := value.SourceFor(relation)
	if !ok || !row.Available() || row.Relation() != relation {
		return false
	}
	for _, cell := range value.Cells() {
		if !cell.Column().Available() || !cell.Type().Available() || !cell.Presence().Available() || cell.Presence().Is(model.Refused) {
			return false
		}
		if cell.Value().Available() && (!cell.Value().ValidFor(mounted.RuntimeFence()) || cell.Value().Type() != cell.Type()) {
			return false
		}
	}
	for _, column := range keyColumns {
		cell, cellOK := value.CellFor(column)
		if !cellOK || !cell.Value().Available() || !cell.Value().ValidFor(mounted.RuntimeFence()) || cell.Value().Type() != cell.Type() || !keyPresence(cell.Presence()) {
			return false
		}
	}
	return true
}

func keyPresence(presence model.Presence) bool {
	return presence.Is(model.Present) || presence.Is(model.AuthenticatedOpaque)
}
