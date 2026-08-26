package testfixture

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// LeftInputBatch redeems the complete row vector and range authority issued
// by the mounted left Input node.  The Input declaration owns both layouts:
// its Scan layout authenticates the range, while its Values layout is the
// complete authored source vector delivered to operators.  Tests must consume
// that sealed pair rather than reconstructing a vector from relation columns;
// the latter can select a different physical coordinate or fail to prove the
// current source ABI.
func (fixture Fixture) LeftInputBatch(t TB, root database.Version) (tuple.Batch, bool) {
	t.Helper()
	node, nodeOK := fixture.LeftInputNode()
	if !nodeOK || !node.Available() {
		return tuple.Batch{}, false
	}
	input, inputOK := node.Input()
	if !inputOK || !input.Available() {
		return tuple.Batch{}, false
	}
	rangeAuthority, rangeOK := input.Range()
	if !rangeOK || !rangeAuthority.Available() {
		return tuple.Batch{}, false
	}
	reader, readerOK := read.Bind(root, input.Values(), fixture.Geometry(), fixture.Scratch())
	if !readerOK || !reader.Available() {
		return tuple.Batch{}, false
	}
	values := make([]tuple.Tuple, 0, len(fixture.RowsLeft()))
	var scope witness.Scope
	completed, valid := reader.Scan(func(row read.Row) bool {
		value, valueOK := tuple.Input(fixture.Mounted(), reader, row)
		if !valueOK {
			return false
		}
		if len(values) == 0 {
			scope = value.Scope()
		} else if !scope.Same(value.Scope()) {
			return false
		}
		values = append(values, value)
		return true
	})
	if !completed || !valid || len(values) == 0 || !scope.ValidFor(fixture.Mounted().RuntimeFence()) {
		return tuple.Batch{}, false
	}
	batch, batchOK := tuple.NewRangeBatch(fixture.Mounted(), rangeAuthority, scope, values, binding.DenominatorWitness{})
	if !batchOK || !batch.ValidFor(fixture.Mounted()) {
		return tuple.Batch{}, false
	}
	return batch, true
}
