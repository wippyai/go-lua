package operation

import (
	"iter"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// OperationCount includes the synthesized opaque operation.
func (core Core) OperationCount() int { return core.geometry.operations.Count() }

// SourceCount is the number of authored operation rows. The opaque row has no
// source coordinate and is intentionally omitted from this projection.
func (core Core) SourceCount() int { return core.geometry.sourceN }

// BoundCount is the canonical root/provider prefix. Canonical ordering emits
// every binding-owned operation before produced-only children and the opaque
// row, so this count is stable without a Target-side counter.
func (core Core) BoundCount() int {
	return core.geometry.boundN
}

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
