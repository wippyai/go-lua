package factapply

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

// ApplyResolvedPathStore applies a fully term-resolved N4 program. It performs
// only owner-local path-address binding; no Facts/source/call callback exists.
func (a *PathSemanticAuthority) ApplyResolvedPathStore(ctx context.Context, reg *axis.Registry, transaction ResolvedPathStoreTransaction, input state.State) (state.State, error) {
	return a.ApplyResolvedPathStoreOnto(ctx, reg, transaction, input, input)
}

// ResolveObjectMemberValue applies the sole semantic overlay law for a table
// constructor member. Both concrete State mutation and the guarded
// coordinate evaluator call this boundary; expected-type evidence therefore
// cannot acquire a second symbolic spelling.
func (a *PathSemanticAuthority) ResolveObjectMemberValue(reg *axis.Registry, value, expected product.Value) (product.Value, error) {
	if reg == nil || !a.Valid() || !product.BelongsToRegistry(reg, value) || !product.BelongsToRegistry(reg, expected) {
		return product.Value{}, fmt.Errorf("factapply: invalid object-member overlay authority")
	}
	return overlayExpectedObjectEntryValue(reg, a.typeValues, value, expected), nil
}

// ApplyResolvedObjectMaterialization installs the heap graph produced by a Lua
// table constructor before its value crosses a call boundary. Path assignment
// uses the same heap kernel; storage is not a precondition for allocation.
func (a *PathSemanticAuthority) ApplyResolvedObjectMaterialization(ctx context.Context, reg *axis.Registry, object ResolvedPathStoreObject, input state.State) (state.State, error) {
	if ctx == nil || reg == nil || !a.Valid() || len(object.Heaps) == 0 || len(object.Entries) != 0 || object.ListFloor != 0 {
		return state.State{}, fmt.Errorf("factapply: invalid object-materialization authority")
	}
	object = cloneResolvedPathStoreObject(object)
	for heapIndex := range object.Heaps {
		heap := &object.Heaps[heapIndex]
		if !product.BelongsToRegistry(reg, heap.Root) {
			return state.State{}, fmt.Errorf("factapply: object materialization has a foreign root")
		}
		for memberIndex := range heap.Members {
			member := &heap.Members[memberIndex]
			if len(member.Suffix) == 0 || !product.BelongsToRegistry(reg, member.Value) || member.HasExpected && !product.BelongsToRegistry(reg, member.Expected) {
				return state.State{}, fmt.Errorf("factapply: object materialization has a malformed member")
			}
			if member.HasExpected {
				resolved, err := a.ResolveObjectMemberValue(reg, member.Value, member.Expected)
				if err != nil {
					return state.State{}, err
				}
				member.Value = resolved
			}
		}
	}
	return applyResolvedObjectHeapsChecked(reg, a.resolver.KeySpace(), input, object.Heaps)
}

// ApplyResolvedPathStoreOnto executes one complete callback-free N4 program.
// Input is the immutable point-entry snapshot used for source propagation and
// cancellation rollback; Output is the evolving N0..N3 state onto which the
// ordered assignment, object, presence, and independent static steps compose.
func (a *PathSemanticAuthority) ApplyResolvedPathStoreOnto(ctx context.Context, reg *axis.Registry, transaction ResolvedPathStoreTransaction, input, output state.State) (state.State, error) {
	if ctx == nil || reg == nil || !a.Valid() || !transaction.Valid(reg) {
		return state.State{}, fmt.Errorf("factapply: invalid resolved path-store authority")
	}
	transaction = transaction.Clone()
	bindSource := func(write *ResolvedPathStoreWrite) {
		if write == nil || !write.HasSourcePath || write.HasSourceState {
			return
		}
		write.SourceStateKey, write.HasSourceState = visibility.AddressAt(a.resolver, transaction.Point, write.SourcePath).RootOrVisibleStateKey()
	}
	var overlayErr error
	overlay := func(write *ResolvedPathStoreWrite) {
		if overlayErr == nil && write != nil && write.HasExpected {
			write.Value, overlayErr = a.ResolveObjectMemberValue(reg, write.Value, write.Expected)
		}
	}
	if transaction.HasAssignment {
		bindSource(&transaction.Assignment)
		overlay(&transaction.Assignment)
	}
	if transaction.HasStatic {
		bindSource(&transaction.Static)
		overlay(&transaction.Static)
	}
	for index := range transaction.Object.Entries {
		bindSource(&transaction.Object.Entries[index])
		overlay(&transaction.Object.Entries[index])
	}
	for heapIndex := range transaction.Object.Heaps {
		for memberIndex := range transaction.Object.Heaps[heapIndex].Members {
			member := &transaction.Object.Heaps[heapIndex].Members[memberIndex]
			if overlayErr == nil && member.HasExpected {
				member.Value, overlayErr = a.ResolveObjectMemberValue(reg, member.Value, member.Expected)
			}
		}
	}
	if overlayErr != nil {
		return state.State{}, overlayErr
	}
	result := ApplyResolvedPathStore(ResolvedPathStoreRequest{
		Context:  transfer.NodeContext{Context: ctx, Session: cancellation.FromContext(ctx), Registry: reg, Point: transaction.Point},
		Resolver: a.resolver, Input: input, Output: output, Transaction: transaction,
	})
	if result.Canceled {
		if err := ctx.Err(); err != nil {
			return input, err
		}
		return input, context.Canceled
	}
	return result.Output, nil
}
