// Package group implements the sealed relational Group operator. Group is a
// batch transformation: cofiber normalization happens before this package,
// and tuple.Batch is the only transient row container.
package group

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Execute groups one immutable input batch by the exact key vector in the
// sealed GroupBinding redeemed from the producer Node. The Node owns the
// opaque output-range authority. Each returned batch is one ordered key group.
// Group boundaries are semantic output extents, not a callback stream or a
// second row representation; every batch retains the same exact input
// cofiber.
//
// Empty input is a valid empty result. Foreign/stale batches, malformed key
// cells, or a cardinality violation refuse. The
// operator never chooses a key/layout at runtime and never fabricates a key
// or scope.
func Execute(binding arrangement.GroupBinding, mounted witness.Mounted, input tuple.Batch) ([]tuple.Batch, bool) {
	if !binding.Available() || !mounted.Available() || !input.ValidFor(mounted) {
		return nil, false
	}
	rangeAuthority, rangeOK := binding.Range()
	if !rangeOK || !rangeAuthority.Available() || rangeAuthority.Kind() != algebra.KindGroup {
		return nil, false
	}
	layout := binding.Key()
	if !layout.Available() || !layout.ValidFor(mounted.Fence()) || !layout.Access().Key().Available() || len(layout.KeyColumns()) == 0 {
		return nil, false
	}
	bound := cardinalityBound(binding.Cardinality())
	if bound == 0 {
		return nil, false
	}
	keyColumns := layout.KeyColumns()

	type keyGroup struct {
		key    tuple.Tuple
		values []tuple.Tuple
	}
	groups := make([]keyGroup, 0)
	for index := 0; index < input.Len(); index++ {
		value, ok := input.At(index)
		if !ok || !value.ValidFor(mounted) || !value.Scope().Same(input.Scope()) || !validKey(value, mounted, keyColumns) {
			return nil, false
		}
		matched := -1
		for position := range groups {
			if tuple.SameKey(mounted, groups[position].key, value, keyColumns) {
				matched = position
				break
			}
		}
		if matched < 0 {
			groups = append(groups, keyGroup{key: value, values: []tuple.Tuple{value}})
			continue
		}
		if uint64(len(groups[matched].values)) >= uint64(bound) {
			return nil, false
		}
		groups[matched].values = append(groups[matched].values, value)
	}

	result := make([]tuple.Batch, len(groups))
	for index, value := range groups {
		keys, keysOK := keyValues(value.key, mounted, keyColumns)
		if !keysOK {
			return nil, false
		}
		batch, ok := tuple.NewKeyRangeBatch(mounted, rangeAuthority, input.Scope(), keys, value.values)
		if !ok {
			return nil, false
		}
		result[index] = batch
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

func cardinalityBound(cardinality model.Cardinality) uint32 {
	if !cardinality.Available() {
		return 0
	}
	switch cardinality.Kind() {
	case model.ExactlyOne, model.Optional:
		return 1
	case model.BoundedMany:
		bound, _ := cardinality.Bound()
		return bound
	default:
		// CompleteDenominator is a signature/output contract whose row
		// extent comes from a mounted witness; Group has no such witness.
		return 0
	}
}

func validKey(value tuple.Tuple, mounted witness.Mounted, columns []model.ColumnID) bool {
	if !value.ValidFor(mounted) || len(columns) == 0 {
		return false
	}
	for _, column := range columns {
		cell, ok := value.CellFor(column)
		if !ok || !cell.Column().Available() || cell.Column() != column || !cell.Type().Available() || !cell.Presence().Available() || !keyPresence(cell.Presence()) || !cell.Value().Available() || !cell.Value().ValidFor(mounted.RuntimeFence()) || cell.Value().Type() != cell.Type() {
			return false
		}
	}
	return true
}

func keyPresence(presence model.Presence) bool {
	return presence.Is(model.Present) || presence.Is(model.AuthenticatedOpaque)
}
