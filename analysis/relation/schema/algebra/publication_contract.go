package algebra

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// SlotSource is the sealed physical address of one semantic input inside an
// Apply child tuple. It is deliberately positional: Child selects one Apply
// child occurrence and Cell selects the typed cell occurrence in that tuple.
//
// RelationID and ColumnID are not an address here.  A Join may legally retain
// two reads of one relation and two occurrences of one nominal ColumnID.  A
// name lookup would either choose one arbitrarily or refuse a valid plan. The
// selected Cell itself retains its sealed source-row ordinal. The checker
// proves both ordinals against the expression layout; the evaluator redeems
// them with Tuple.At then Tuple.SourceAt.
type SlotSource struct {
	child uint32
	cell  uint32
}

// NewSlotSource constructs one positional semantic-slot address.  Bounds and
// exact type membership are intentionally checked only after the enclosing
// Apply expression and its signature are available.
func NewSlotSource(child, cell uint32) SlotSource {
	return SlotSource{child: child, cell: cell}
}

// Child is the ordered Apply child occurrence supplying this semantic slot.
func (source SlotSource) Child() uint32 { return source.child }

// Cell is the ordered typed cell occurrence inside that child's sealed tuple
// layout.
func (source SlotSource) Cell() uint32 { return source.cell }

// ApplyContract declares one authenticated semantic operation boundary. The
// operation identity is owned by semantic/signature. slotSource maps each
// sealed signature input slot to an exact tuple position in an ordered Apply
// child. Several slots may name one child: that child is selected once and all
// of its slots are read from its same correlated tuple/range. Distinct child
// ordinals retain the physical Cartesian product semantics for genuinely
// independent reads.
type ApplyContract struct {
	operation   signature.Identity
	slotSource  []SlotSource
	correlation ApplyCorrelation
	output      OutputAddress
}

// NewApplyContract constructs an ordinary operation boundary without
// semantic validation. slotSource is an ordered vector with one entry per
// operation input. output is mandatory sealed destination geometry; there is
// no output-less form because a mounted Apply must have one authenticated
// destination plan. The checker and mount seal child, cell, totality, range,
// and type relationship against the operation signature.
func NewApplyContract(operation signature.Identity, slotSource []SlotSource, output OutputAddress) ApplyContract {
	return ApplyContract{operation: operation, slotSource: cloneSlotSources(slotSource), output: output}
}

// NewCorrelatedApplyContract constructs the explicit heterogeneous Apply
// boundary. Correlation and output are required to be complete sealed
// declarations; malformed declarations are refused here rather than becoming
// a second runtime interpretation of ordinary Apply.
func NewCorrelatedApplyContract(operation signature.Identity, slotSource []SlotSource, correlation ApplyCorrelation, output OutputAddress) (ApplyContract, bool) {
	if !correlation.Available() || !output.Available() {
		return ApplyContract{}, false
	}
	return ApplyContract{operation: operation, slotSource: cloneSlotSources(slotSource), correlation: correlation, output: output}, true
}

// Operation returns the stable operation identity.
func (contract ApplyContract) Operation() signature.Identity { return contract.operation }

// SlotSource returns the sealed positional source for each operation input
// slot.
func (contract ApplyContract) SlotSource() []SlotSource { return cloneSlotSources(contract.slotSource) }

// Correlation returns the explicit heterogeneous Apply correlation declaration,
// or its unavailable zero value when this Apply is uncorrelated.
func (contract ApplyContract) Correlation() ApplyCorrelation { return contract.correlation }

// Output returns the sealed destination geometry of this Apply plan.
func (contract ApplyContract) Output() OutputAddress { return contract.output }

func (contract ApplyContract) digestBytes() []byte {
	parts := appendOperation(nil, contract.operation)
	parts = appendLength(parts, len(contract.slotSource))
	for _, source := range contract.slotSource {
		parts = appendUint32(parts, source.child)
		parts = appendUint32(parts, source.cell)
	}
	parts = appendBytes(parts, contract.output.digestBytes())
	// Correlation presence is part of the Apply identity even when a caller
	// attempts to carry an unavailable declaration.  Omitting that marker for
	// malformed data would alias a hostile correlated shape with the ordinary
	// Apply form and would let a later checker/mount boundary lose the fact that
	// correlation was authored.  A valid declaration contributes its complete
	// digest bytes; an unavailable declaration contributes an explicit empty
	// payload and remains unavailable to semantic admission.
	if contract.correlation.Specified() {
		parts = append(parts, 1)
		if contract.correlation.Available() {
			return appendBytes(parts, contract.correlation.digestBytes())
		}
		return appendBytes(parts, nil)
	}
	return append(parts, 0)
}

// PublishContract names one logical destination relation and key. Invocation
// cardinality and atomic commit policy belong to the semantic and transaction
// layers, not to this expression contract.
type PublishContract struct {
	destination model.RelationID
	key         model.KeyID
	columns     []model.ColumnID
}

// NewPublishContract constructs a publication declaration without checking
// destination ownership or key authority.
func NewPublishContract(destination model.RelationID, key model.KeyID, columns ...model.ColumnID) PublishContract {
	return PublishContract{destination: destination, key: key, columns: cloneColumns(columns)}
}

// Destination returns the logical destination relation.
func (contract PublishContract) Destination() model.RelationID { return contract.destination }

// Key returns the destination key used by the declaration.
func (contract PublishContract) Key() model.KeyID { return contract.key }

// Columns is the exact ordered writable semantic layout committed by this
// publication.  A nil vector remains the compatibility form for historic
// full-relation contracts; newly lowered rules always provide an explicit
// non-empty vector.  The checker resolves that compatibility form before
// physical mount, so runtime never discovers a column by name.
func (contract PublishContract) Columns() []model.ColumnID { return cloneColumns(contract.columns) }

func (contract PublishContract) digestBytes() []byte {
	parts := appendRelation(nil, contract.destination)
	parts = appendKey(parts, contract.key)
	return appendColumns(parts, contract.columns)
}
