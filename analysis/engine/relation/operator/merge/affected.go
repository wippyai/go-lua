package merge

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// RecomputeAffected reduces exactly the successor (scope, key) ranges named
// by changed. alternatives is authored child order; each child is already a
// materialized successor vector selected from its sealed physical access.
//
// This is intentionally a narrow tuple boundary. It does not accept a
// database, Reader, layout resolver, callback, or relation scan. A caller
// that cannot redeem every authored alternative from seal-issued evidence
// must refuse before calling this function. Empty alternative branches are
// valid and simply contribute no tuple to an affected key.
//
// The output contains at most one Merge range batch per affected (scope, key)
// and calls tuple.Merge exactly once for each range that still has at least
// one successor alternative. A key deleted from every alternative therefore
// produces no output batch.
func RecomputeAffected(binding arrangement.MergeBinding, mounted witness.Mounted, changed []tuple.Batch, alternatives [][]tuple.Batch) ([]tuple.Batch, bool) {
	rangeAuthority, keyColumns, relation, ok := mergeAuthority(binding, mounted)
	if !ok || alternatives == nil || len(alternatives) == 0 {
		return nil, false
	}

	// Validate all successor batches before selecting affected keys. This keeps
	// malformed or foreign alternatives from being silently ignored merely
	// because they do not happen to contain a changed key.
	for _, branch := range alternatives {
		for _, batch := range branch {
			if !batch.ValidFor(mounted) {
				return nil, false
			}
			for index := 0; index < batch.Len(); index++ {
				value, valueOK := batch.At(index)
				if !valueOK || !validValue(value, mounted, relation, keyColumns) {
					return nil, false
				}
			}
		}
	}

	affected, affectedOK := affectedKeys(mounted, changed, relation, keyColumns)
	if !affectedOK {
		return nil, false
	}
	result := make([]tuple.Batch, 0, len(affected))
	for _, key := range affected {
		members := make([]tuple.Tuple, 0, len(alternatives))
		for _, branch := range alternatives {
			branchMembers, branchOK := branchMembersFor(mounted, branch, key, relation, keyColumns)
			if !branchOK {
				return nil, false
			}
			members = append(members, branchMembers...)
		}
		if len(members) == 0 {
			// This is the successor deletion case. The affected range is
			// intentionally absent from the output, not represented by a
			// fabricated empty tuple.
			continue
		}
		merged, mergeOK := tuple.Merge(mounted, members, keyColumns)
		if !mergeOK || !merged.ValidFor(mounted) || !merged.Scope().Same(key.scope) {
			return nil, false
		}
		keys, keysOK := keyValues(merged, mounted, keyColumns)
		if !keysOK {
			return nil, false
		}
		batch, batchOK := tuple.NewKeyRangeBatch(mounted, rangeAuthority, key.scope, keys, []tuple.Tuple{merged})
		if !batchOK || !batch.ValidFor(mounted) {
			return nil, false
		}
		result = append(result, batch)
	}
	return result, true
}

type affectedKey struct {
	scope  witness.Scope
	values []binding.ValueToken
}

func mergeAuthority(binding arrangement.MergeBinding, mounted witness.Mounted) (arrangement.RangeBinding, []model.ColumnID, model.RelationID, bool) {
	if !binding.Available() || !mounted.Available() {
		return arrangement.RangeBinding{}, nil, model.RelationID{}, false
	}
	rangeAuthority, rangeOK := binding.Range()
	if !rangeOK || !rangeAuthority.Available() || rangeAuthority.Kind() != algebra.KindMerge || !rangeAuthority.ValidFor(mounted.Fence()) {
		return arrangement.RangeBinding{}, nil, model.RelationID{}, false
	}
	layout := binding.Key()
	if !layout.Available() || !layout.ValidFor(mounted.Fence()) || !layout.Access().Key().Available() {
		return arrangement.RangeBinding{}, nil, model.RelationID{}, false
	}
	keyColumns := layout.KeyColumns()
	if len(keyColumns) == 0 {
		return arrangement.RangeBinding{}, nil, model.RelationID{}, false
	}
	return rangeAuthority, keyColumns, layout.Access().Relation(), true
}

func affectedKeys(mounted witness.Mounted, changed []tuple.Batch, relation model.RelationID, columns []model.ColumnID) ([]affectedKey, bool) {
	result := make([]affectedKey, 0)
	for _, batch := range changed {
		if !batch.ValidFor(mounted) {
			return nil, false
		}
		for index := 0; index < batch.Len(); index++ {
			value, valueOK := batch.At(index)
			if !valueOK || !validValue(value, mounted, relation, columns) {
				return nil, false
			}
			keys, keysOK := keyValues(value, mounted, columns)
			if !keysOK || !appendAffected(mounted, &result, value.Scope(), keys, columns) {
				return nil, false
			}
		}
		// A keyed Merge/Group range may carry its affected key even when the
		// successor tuple was deleted. RangeKeys are already sealed values, so
		// redeem those values directly without scanning a branch.
		if batch.Len() == 0 {
			keys := batch.RangeKeys()
			if len(keys) != 0 && !validKeyValues(mounted, keys, columns) {
				return nil, false
			}
			if len(keys) != 0 && !appendAffected(mounted, &result, batch.Scope(), keys, columns) {
				return nil, false
			}
		}
	}
	return result, true
}

func appendAffected(mounted witness.Mounted, values *[]affectedKey, scope witness.Scope, keys []binding.ValueToken, columns []model.ColumnID) bool {
	if !scope.ValidFor(mounted.RuntimeFence()) || !validKeyValues(mounted, keys, columns) {
		return false
	}
	for _, existing := range *values {
		if existing.scope.Same(scope) && sameKeyValues(mounted, existing.values, keys) {
			return true
		}
	}
	*values = append(*values, affectedKey{scope: scope, values: append([]binding.ValueToken(nil), keys...)})
	return true
}

func branchMembersFor(mounted witness.Mounted, branch []tuple.Batch, key affectedKey, relation model.RelationID, columns []model.ColumnID) ([]tuple.Tuple, bool) {
	result := make([]tuple.Tuple, 0, 1)
	for _, batch := range branch {
		if !batch.ValidFor(mounted) {
			return nil, false
		}
		for index := 0; index < batch.Len(); index++ {
			value, valueOK := batch.At(index)
			if !valueOK || !validValue(value, mounted, relation, columns) {
				return nil, false
			}
			if value.Scope().Same(key.scope) {
				candidate, candidateOK := keyValues(value, mounted, columns)
				if !candidateOK {
					return nil, false
				}
				if sameKeyValues(mounted, candidate, key.values) {
					result = append(result, value)
				}
			}
		}
	}
	return result, true
}

func sameKeyValues(mounted witness.Mounted, left, right []binding.ValueToken) bool {
	if len(left) == 0 || len(left) != len(right) {
		return false
	}
	for index := range left {
		if !tuple.SemanticEqual(mounted, left[index].Type(), left[index], right[index]) {
			return false
		}
	}
	return true
}

func validKeyValues(mounted witness.Mounted, values []binding.ValueToken, columns []model.ColumnID) bool {
	if !mounted.Available() || len(values) == 0 || len(values) != len(columns) {
		return false
	}
	types := make(map[model.ColumnID]model.TypeID, len(mounted.Columns()))
	for _, schema := range mounted.Columns() {
		if !schema.Available() || !schema.Type().Available() {
			return false
		}
		types[schema.ID()] = schema.Type()
	}
	for index, column := range columns {
		typeID, ok := types[column]
		if !ok || !values[index].Available() || !values[index].ValidFor(mounted.RuntimeFence()) || values[index].Type() != typeID {
			return false
		}
	}
	return true
}
