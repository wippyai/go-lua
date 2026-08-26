package input

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	bindingpkg "github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Execute redeems one sealed relation-scan binding for the ordered cofiber
// partitions present in the Reader. The Reader is required to be bound to the
// exact mounted layout; no logical Access or scope is reconstructed at
// runtime. Every accepted row is lowered once by tuple.Input, preserving the
// owner-issued RowID, exact scope, lineage, and delivered cells.
//
// The returned slice is ordered transport of sealed partitions, not a second
// relation representation. Rows are grouped only by their exact authenticated
// cofiber; no callback stream, flattening, or regrouping is exposed.
func Execute(binding arrangement.InputBinding, mounted witness.Mounted, reader read.Reader) ([]tuple.Batch, bool) {
	if !binding.Available() || !mounted.Available() || !reader.Available() {
		return nil, false
	}
	rangeAuthority, rangeOK := binding.Range()
	if !rangeOK || !rangeAuthority.Available() {
		return nil, false
	}
	scan := binding.Scan()
	values := binding.Values()
	if !scan.ValidFor(mounted.Fence()) || !values.ValidFor(mounted.Fence()) || !reader.Layout().Equal(values) || reader.Layout().Access().Relation() != binding.Relation() || !rangeAuthority.Layout().Equal(scan) {
		return nil, false
	}
	type partition struct {
		scope  witness.Scope
		values []tuple.Tuple
	}
	partitions := make([]partition, 0)
	refused := false
	completed, valid := reader.Scan(func(row read.Row) bool {
		if row == nil || !row.Available() || !row.Scope().ValidFor(mounted.RuntimeFence()) {
			refused = true
			return false
		}
		value, ok := tuple.Input(mounted, reader, row)
		if !ok || !value.ValidFor(mounted) || !value.Scope().Same(row.Scope()) {
			refused = true
			return false
		}
		partitionIndex := -1
		for index := range partitions {
			if partitions[index].scope.Same(row.Scope()) {
				partitionIndex = index
				break
			}
		}
		if partitionIndex < 0 {
			partitions = append(partitions, partition{scope: row.Scope(), values: []tuple.Tuple{value}})
		} else {
			partitions[partitionIndex].values = append(partitions[partitionIndex].values, value)
		}
		return true
	})
	if refused || !valid || !completed {
		return nil, false
	}
	batches := make([]tuple.Batch, len(partitions))
	for index, value := range partitions {
		// The binding was issued by arrangement.Node, so redeem its private
		// range witness directly. Input never chooses a runtime Access.
		batch, ok := tuple.NewRangeBatch(mounted, rangeAuthority, value.scope, value.values, bindingpkg.DenominatorWitness{})
		if !ok {
			return nil, false
		}
		batches[index] = batch
	}
	return batches, true
}
