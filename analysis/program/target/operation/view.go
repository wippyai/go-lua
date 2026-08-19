package operation

import (
	"errors"
	"iter"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// OperationCount includes the synthesized opaque operation.
func (core Core) OperationCount() int { return core.geometry.operations.Count() }

// SourceCount is the number of authored operation rows. The opaque row has no
// source coordinate and is intentionally omitted from this projection.
func (core Core) SourceCount() int { return core.geometry.sourceN }

// SourceCount and SourceOperation are also available on the first immutable
// geometry value so Target can map drafts before exact-key anchor finalization.
func (geometry Geometry) SourceCount() int { return geometry.sourceN }

func (geometry Geometry) SourceOperation(source int) (vocabulary.Operation, bool) {
	row, ok := geometry.sources.At(source)
	if !ok || row.operation == 0 {
		return 0, false
	}
	return row.operation, true
}

// OperationAt returns one canonical one-based operation handle by zero-based
// owner index.
func (core Core) OperationAt(index int) (vocabulary.Operation, bool) {
	if _, ok := core.geometry.operations.At(index); !ok {
		return 0, false
	}
	return vocabulary.Operation(index + 1), true
}

// Operations iterates canonical operation handles without exposing final
// storage or a copied []Operation ABI.
func (core Core) Operations(yield func(vocabulary.Operation) bool) bool {
	if yield == nil {
		return false
	}
	for index := 0; index < core.OperationCount(); index++ {
		if !yield(vocabulary.Operation(index + 1)) {
			return false
		}
	}
	return true
}

// SourceOperation resolves a zero-based root source coordinate to the owner-
// issued operation handle.
func (core Core) SourceOperation(source int) (vocabulary.Operation, bool) {
	return core.geometry.SourceOperation(source)
}

// Opaque returns the owner-issued synthesized operation handle.
func (core Core) Opaque() (vocabulary.Operation, bool) {
	if core.OperationCount() == 0 {
		return 0, false
	}
	return vocabulary.Operation(core.OperationCount()), true
}

func (core Core) operation(op vocabulary.Operation) (operationRow, bool) {
	if op == 0 {
		return operationRow{}, false
	}
	return core.geometry.operations.At(int(op) - 1)
}

// BindingCount and BindingAt expose copied neutral binding values. The final
// operation owner retains only rows and segment pools.
func (core Core) BindingCount(op vocabulary.Operation) int {
	row, ok := core.operation(op)
	if !ok {
		return 0
	}
	return core.geometry.bindings.Count(row.bindings)
}

func (core Core) BindingAt(op vocabulary.Operation, index int) (vocabulary.BindingSpec, bool) {
	row, ok := core.operation(op)
	if !ok {
		return vocabulary.BindingSpec{}, false
	}
	binding, ok := core.geometry.bindings.At(row.bindings, index)
	if !ok {
		return vocabulary.BindingSpec{}, false
	}
	owner := make([]string, core.geometry.segments.Count(binding.owner))
	for index := range owner {
		value, valueOK := core.geometry.segments.At(binding.owner, index)
		if !valueOK {
			return vocabulary.BindingSpec{}, false
		}
		owner[index] = value
	}
	member := make([]string, core.geometry.segments.Count(binding.member))
	for index := range member {
		value, valueOK := core.geometry.segments.At(binding.member, index)
		if !valueOK {
			return vocabulary.BindingSpec{}, false
		}
		member[index] = value
	}
	return vocabulary.BindingSpec{Namespace: binding.namespace, Owner: owner, Member: member}, true
}

func (core Core) bindingSpecs(op vocabulary.Operation) ([]vocabulary.BindingSpec, error) {
	count := core.BindingCount(op)
	bindings := make([]vocabulary.BindingSpec, count)
	for index := range bindings {
		binding, ok := core.BindingAt(op, index)
		if !ok {
			return nil, errors.New("target/operation: malformed binding projection")
		}
		bindings[index] = binding
	}
	return bindings, nil
}

// InputFormalCount and ValuesVarCount are the narrow value-geometry columns
// consumed by Protocol.
func (core Core) InputFormalCount(op vocabulary.Operation) int {
	row, ok := core.operation(op)
	if !ok {
		return 0
	}
	return int(row.input)
}

func (core Core) ValuesVarCount(op vocabulary.Operation) uint32 {
	row, ok := core.operation(op)
	if !ok {
		return 0
	}
	return row.valuesVar
}

func (core Core) OutcomeCount(op vocabulary.Operation) int {
	row, ok := core.operation(op)
	if !ok {
		return 0
	}
	return core.geometry.outcomes.Count(row.outcomes)
}

func (core Core) OutcomeValueSlots(op vocabulary.Operation, index int) (uint32, bool) {
	row, ok := core.operation(op)
	if !ok {
		return 0, false
	}
	outcome, ok := core.geometry.outcomes.At(row.outcomes, index)
	if !ok {
		return 0, false
	}
	return outcome.slots, true
}

// CallbackCount, CallbackAt, CallbackOwner, CallbackSource, and
// CallbackLifecycle expose owner-issued callback coordinates. Lifecycle is
// retained here as part of the callback geometry, not recomputed by Target.
func (core Core) CallbackCount(op vocabulary.Operation) int {
	row, ok := core.operation(op)
	if !ok {
		return 0
	}
	return core.geometry.callbacks.Count(row.callbacks)
}

func (core Core) CallbackAt(op vocabulary.Operation, index int) (vocabulary.CallbackID, bool) {
	row, ok := core.operation(op)
	if !ok {
		return 0, false
	}
	callback, ok := core.geometry.callbacks.At(row.callbacks, index)
	if !ok {
		return 0, false
	}
	return callback.id, true
}

func (core Core) CallbackOwner(id vocabulary.CallbackID) (vocabulary.Operation, bool) {
	if id == 0 {
		return 0, false
	}
	for operation := 1; operation <= core.OperationCount(); operation++ {
		op := vocabulary.Operation(operation)
		for index := 0; index < core.CallbackCount(op); index++ {
			callback, ok := core.CallbackAt(op, index)
			if ok && callback == id {
				return op, true
			}
		}
	}
	return 0, false
}

func (core Core) CallbackSource(id vocabulary.CallbackID) (vocabulary.InputSource, bool) {
	row, ok := core.callback(id)
	if !ok {
		return vocabulary.InputSource{}, false
	}
	if opaque, opaqueOK := core.Opaque(); opaqueOK && row.owner == opaque {
		return vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs}, true
	}
	return vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: uint32(row.source)}, true
}

func (core Core) CallbackLifecycle(id vocabulary.CallbackID) (vocabulary.CallbackLifecycle, bool) {
	row, ok := core.callback(id)
	if !ok {
		return 0, false
	}
	return row.lifecycle, true
}

func (core Core) callback(id vocabulary.CallbackID) (callbackRow, bool) {
	if id == 0 {
		return callbackRow{}, false
	}
	for operation := 1; operation <= core.OperationCount(); operation++ {
		op := vocabulary.Operation(operation)
		row, rowOK := core.operation(op)
		if !rowOK {
			return callbackRow{}, false
		}
		for _, callback := range core.geometry.callbacks.All(row.callbacks) {
			if callback.id == id {
				return callback, true
			}
		}
	}
	return callbackRow{}, false
}

// Anchor is the operation semantic anchor issued by Core.
func (core Core) Anchor(op vocabulary.Operation) (identity.ContentID, bool) {
	if op == 0 {
		return identity.ContentID{}, false
	}
	row, ok := core.anchors.At(int(op) - 1)
	if !ok || !row.id.Available() {
		return identity.ContentID{}, false
	}
	return row.id, true
}

// Visit emits every source operation and its immutable geometry. The callback
// receives a Core-owned coordinate and can stop without exposing storage.
func (core Core) Visit(yield func(vocabulary.Operation) bool) bool {
	return core.Operations(yield)
}

// Geometry exposes only the immutable operation count needed by diagnostics;
// callers cannot recover mutable rows from it.
func (geometry Geometry) OperationCount() int { return geometry.operations.Count() }

// Ensure iter.Seq remains part of the package's intended iteration vocabulary
// without returning a slice-backed final ABI.
func (core Core) Seq() iter.Seq[vocabulary.Operation] {
	return func(yield func(vocabulary.Operation) bool) {
		for index := 0; index < core.OperationCount(); index++ {
			if !yield(vocabulary.Operation(index + 1)) {
				return
			}
		}
	}
}
