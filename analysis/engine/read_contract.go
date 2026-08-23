// read_contract.go owns the declaration half of the read-boundary contract: the
// one declaration a Rule makes about how the engine must deliver a read, so a
// Fold never sees an unordered, absent, or foreign operand and never restates
// engine policy. The materialization half is execution.ReadCellPolicy, which
// sits below every consumer of a delivered cell.

package engine

import "github.com/wippyai/go-lua/analysis/engine/execution"

// ReadOrder declares the member order a selected read's Selection is
// materialized in. It is one declaration per read; a Fold holds no positional
// assumption of its own.
type ReadOrder uint8

const (
	// ReadOrderCanonical delivers members in the engine's canonical route
	// order: resolved exact Unit, then numeric tag. This is the order every
	// selected read has always had.
	ReadOrderCanonical ReadOrder = iota
	// ReadOrderByTag delivers members in ascending tag order, so a member's
	// ordinal is the rank of its own tag. Duplicate tags have no total order
	// under this declaration and refuse the read.
	ReadOrderByTag
)

func (order ReadOrder) valid() bool {
	return order == ReadOrderCanonical || order == ReadOrderByTag
}

// ReadSparse declares how an unwritten Factor coordinate reaches the Fold.
type ReadSparse uint8

const (
	// ReadSparseExplicit preserves the stored/absent distinction. It is the
	// opt-in for a rule that genuinely reads evidence provenance and must tell
	// a written Bottom from an unwritten coordinate; those rules read
	// OrderedCells.At.
	ReadSparseExplicit ReadSparse = iota
	// ReadSparseFactorDefault delivers an unwritten coordinate as the bound
	// Factor's declared default, present. A Fold under this declaration reads
	// OrderedCells.Value and has no absent branch to get wrong.
	ReadSparseFactorDefault
)

func (sparse ReadSparse) valid() bool {
	return sparse == ReadSparseExplicit || sparse == ReadSparseFactorDefault
}

// ReadOpaque declares the disposition of a selected read whose locator reports
// that some alternative of its dispatch set is opaque.
type ReadOpaque uint8

const (
	// ReadOpaqueRefuse refuses the read when its locator reports an opaque
	// alternative. A rule whose judgment has no sound over-approximation
	// declares this explicitly.
	ReadOpaqueRefuse ReadOpaque = iota
	// ReadOpaqueWiden substitutes the Factor's Top for every member of the
	// read when any alternative is opaque, so a Fold never branches on
	// opacity and never mistakes it for malformed evidence.
	ReadOpaqueWiden
)

func (opaque ReadOpaque) valid() bool {
	return opaque == ReadOpaqueRefuse || opaque == ReadOpaqueWiden
}

// ReadContract is the whole read-boundary declaration. Its zero value is the
// engine's original delivery: canonical order, explicit sparsity, and refusal
// on an opaque alternative.
type ReadContract struct {
	Order    ReadOrder
	Sparse   ReadSparse
	OnOpaque ReadOpaque
}

func (contract ReadContract) valid() bool {
	return contract.Order.valid() && contract.Sparse.valid() && contract.OnOpaque.valid()
}

// exactValid additionally refuses the clauses that have no meaning for a read
// with no locator: an exact read observes one coordinate in one order and has
// no alternative set that could be opaque.
func (contract ReadContract) exactValid() bool {
	return contract.valid() && contract.Order == ReadOrderCanonical && contract.OnOpaque == ReadOpaqueRefuse
}

// readForeignOwnerRefusal names the engine refusal a read takes when the
// Factor presented at binding is not the Factor its sealed row owns. The
// authentication happens once, here, so a Fold never re-proves value ownership
// with a self-equality check.
const readForeignOwnerRefusal = "read/foreign-owner"

// readContractRefusal names the engine refusal a read takes when the contract
// it declares is not one this read kind can carry.
const readContractRefusal = "read/contract"

// summaryCellFrom delivers one observed coordinate through the shared
// execution-owned materialization and wraps it in the engine's ordered cell.
func summaryCellFrom[V any](policy execution.ReadCellPolicy[V], value V, present bool) summaryCell[V] {
	delivered, deliveredPresent := policy.Cell(value, present)
	return summaryCell[V]{value: delivered, present: deliveredPresent}
}
