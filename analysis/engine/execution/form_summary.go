// form_summary.go owns the S form: the Summary and Complete read axis. A
// summary row delivers the whole declared cell vector of one partition row,
// and a complete row delivers it against a closed denominator. Both are read
// under one mandatory contract, validated once when the cursor opens, so no
// per-cell branch decides what a cell means.

package execution

import (
	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	ruleplan "github.com/wippyai/go-lua/analysis/schema/rule/plan"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// SummaryContract is the read contract one S-form row is delivered under. It
// is mandatory: a summary or complete read carries an owner-declared cell
// order, an absence policy, an opaque policy, a width, and the closed
// denominator its vector is sealed against. The contract is a value copied
// from the sealed plan; a row cannot be opened without one.
type SummaryContract struct {
	Form        ruleprogram.ReadForm
	Contract    ruleplan.ReadContract
	Denominator ruleplan.DenominatorAddr
}

// Available reports whether this is a complete S-form contract. Both summary
// and complete reads seal a denominator - the closed set their vector is a
// vector of - so an absent denominator is not a sparse read here, it is an
// unsealed one.
func (contract SummaryContract) Available() bool {
	if contract.Form != ruleprogram.Summary && contract.Form != ruleprogram.Complete {
		return false
	}
	if !contract.Contract.Order.Available() || !contract.Contract.Sparse.Available() ||
		!contract.Contract.OnOpaque.Available() || !contract.Contract.Multiplicity.Available() {
		return false
	}
	return contract.Denominator.Present
}

// Closed reports whether this contract seals the whole denominator vector.
// A complete read is closed by construction: its normal form has no selection
// predicate, so every declared coordinate is a cell of the delivered vector.
func (contract SummaryContract) Closed() bool {
	return contract.Available() && contract.Form == ruleprogram.Complete
}

// SummaryVector is one partition row's whole cell vector. It is a view over
// the Binding-owned observation storage: constructing one allocates nothing,
// and At is a direct index that neither compacts nor reorders.
//
// Absence is preserved per cell. A coordinate whose value was deleted stays a
// cell of the vector and reports present false, so the reader keeps the
// correlation between a cell's position and the coordinate the owner declared
// at that position. Compacting the absent cells away would silently renumber
// every later cell.
type SummaryVector[V any] struct {
	view factbinding.Observation[V]
	// members is the second backing: the cells of a nested member set, read
	// one ordinal at a time at each member's own exact coordinate rather than
	// delivered by a Factor cursor. See NewMemberVector for why that is a
	// backing of this view and not a view of its own.
	members []MemberCell[V]
	width   uint16
	open    bool
}

// MemberCell is one cell of a member-set vector: the fact read at that
// member's own coordinate, and whether that coordinate holds one. Absence is
// carried per cell for the same reason a Factor-backed vector carries it - the
// position of a cell is the ordinal its owner declared it at, and compacting
// an absent cell away would renumber every later one.
type MemberCell[V any] struct {
	Value   V
	Present bool
	// Region is the support this cell was observed over. A vector's cells are
	// read one coordinate at a time and each answers over what its own read
	// proved, so the conclusion folded from them holds over the conjunction of
	// their supports and not over the window the invocation opened.
	Region support.Mask
}

// NewMemberVector views one caller-owned member-set cell slice as the vector
// its reader is declared to receive.
//
// A nested member set is a closed denominator the owner itself publishes -
// its MemberCount and MemberAt ARE the denominator - so a read that spans it
// is a whole-vector read, and the declaration that says so is a Summary read.
// What differs is only where the cells come from: a Factor-backed summary read
// delivers a view over Binding-owned observation storage, while a member set
// is read one ordinal at a time through each member's own exact coordinate.
//
// The reader must not be able to tell. A many-valued input is ONE vector
// argument under the reducer call shape, so a second vector type would split
// every fold that consumes one into two spellings of the same parameter. The
// view therefore carries a second backing rather than growing a sibling.
//
// The slice is caller-owned and lives for the invocation, which is what keeps
// a warm member-set read allocation-free: a family sizes it once at its sealed
// member width and refills it per invocation.
func NewMemberVector[V any](cells []MemberCell[V]) (SummaryVector[V], bool) {
	if cells == nil || len(cells) > int(^uint16(0)) {
		return SummaryVector[V]{}, false
	}
	return SummaryVector[V]{members: cells, width: uint16(len(cells)), open: true}, true
}

// Valid reports whether the vector still belongs to its live read cursor, or,
// for a member set, to the caller-owned cells it was opened over.
func (vector SummaryVector[V]) Valid() bool {
	if !vector.open {
		return false
	}
	if vector.members != nil {
		return len(vector.members) == int(vector.width)
	}
	return vector.view.Valid()
}

// Count is the declared cell width of this vector, absent cells included.
func (vector SummaryVector[V]) Count() int {
	if !vector.open {
		return 0
	}
	return int(vector.width)
}

// At returns one cell in sealed declaration order. The three results are the
// typed value, whether that coordinate holds one, and whether the index names
// a cell at all: an absent cell is an available cell with no value, while an
// out-of-range index is not a cell.
func (vector SummaryVector[V]) At(index int) (V, bool, bool) {
	var zero V
	if !vector.open || index < 0 || index >= int(vector.width) {
		return zero, false, false
	}
	if vector.members != nil {
		if len(vector.members) != int(vector.width) {
			return zero, false, false
		}
		cell := vector.members[index]
		return cell.Value, cell.Present, true
	}
	entry, ok := vector.view.At(index)
	if !ok {
		return zero, false, false
	}
	value, present := entry.Read()
	return value, present, true
}

// SummaryRow is static typed row data for the S form: one sealed summary read,
// the contract it is delivered under, and the exact write its fold publishes
// through. It holds no live Run, cursor, or domain value.
type SummaryRow[K scalar.Key, V any] struct {
	read     SummaryRead[K, V]
	write    ExactWrite[K, V]
	contract SummaryContract
}

// NewSummaryRow seals one S-form row. The contract is not optional and not
// defaulted: a row whose plan did not carry a complete summary contract is
// refused here rather than delivered under an invented one.
func NewSummaryRow[K scalar.Key, V any](binding *factbinding.Binding[K, V], unit carrier.Unit, input uint16, target carrier.Target, output uint16, contract SummaryContract) (SummaryRow[K, V], bool) {
	if !contract.Available() {
		return SummaryRow[K, V]{}, false
	}
	read, readOK := NewSummaryRead(binding, unit, input)
	write, writeOK := NewExactWrite(binding, target, output)
	if !readOK || !writeOK {
		return SummaryRow[K, V]{}, false
	}
	return SummaryRow[K, V]{read: read, write: write, contract: contract}, true
}

// Valid proves that the row still names a live declared summary unit and a
// live declared write target.
func (row SummaryRow[K, V]) Valid() bool {
	return row.contract.Available() && row.read.Valid() && row.write.Valid()
}

// Contract returns the sealed contract this row is delivered under.
func (row SummaryRow[K, V]) Contract() SummaryContract { return row.contract }

// Write returns the row's sealed exact publication axis, so a fold stages
// through the same descriptor the row was sealed with.
func (row SummaryRow[K, V]) Write() ExactWrite[K, V] { return row.write }

// Deliver advances this row's cursor by one partition row and returns that
// row's whole cell vector. The status is the cursor disposition: Available
// carries a vector, Exhausted ends the partition, and Refuse is a contract or
// lifecycle failure. Nothing is allocated on a warm delivery.
func (row SummaryRow[K, V]) Deliver(ticket Ticket, scratch *Scratch[K, V]) (SummaryVector[V], ReadStatus) {
	if !row.Valid() || scratch == nil {
		return SummaryVector[V]{}, ReadRefuse
	}
	status := row.read.Read(ticket, scratch)
	if status != ReadAvailable {
		return SummaryVector[V]{}, status
	}
	view, viewOK := scratch.Observation()
	if !viewOK {
		return SummaryVector[V]{}, ReadRefuse
	}
	count := view.Count()
	if count < 0 || count > int(^uint16(0)) {
		return SummaryVector[V]{}, ReadRefuse
	}
	// A single-width contract is a vector of one cell, not a scalar read: the
	// declared width is checked once here so no cell-level branch has to.
	if row.contract.Contract.Multiplicity != ruleprogram.MultiplicityMany && count > 1 {
		return SummaryVector[V]{}, ReadRefuse
	}
	return SummaryVector[V]{view: view, width: uint16(count), open: true}, ReadAvailable
}

// Region returns the authenticated support row the current delivery belongs
// to. It is the region a fold stages its published fact at.
func (row SummaryRow[K, V]) Region(scratch *Scratch[K, V]) (support.Mask, bool) {
	if !row.Valid() || scratch == nil {
		return support.Mask{}, false
	}
	return scratch.Region()
}

// Close closes this row's read cursor and leaves the Ticket open so the
// write axis can still stage and seal against the same invocation.
func (row SummaryRow[K, V]) Close(ticket Ticket, scratch *Scratch[K, V]) bool {
	if !row.Valid() {
		return false
	}
	return row.read.Close(ticket, scratch)
}

// summaryFormContract projects one join's sealed summary contract. It is the
// one place the plan's three separate read rows - form, scalar contract, and
// denominator address - are folded into the value a row is opened under.
func summaryFormContract(rule generated.CompiledRule, join int) (SummaryContract, bool) {
	form, formOK := rule.ReadFormAt(join)
	contract, contractOK := rule.ReadContractAt(join)
	denominator, denominatorOK := rule.ReadDenominatorAt(join)
	if !formOK || !contractOK || !denominatorOK {
		return SummaryContract{}, false
	}
	// Which addressing a form requires is the declaration's own law, asked
	// here rather than restated: a complete read is closed and names nothing,
	// while a summary read is correlated by an owner-issued predicate, by the
	// ordinal of the member set it spans, or by the position it holds in the
	// key vector its candidate published. A descriptor that
	// disagrees with its own form is refused rather than delivered as the
	// other one.
	predicate, predicatePresent, predicateOK := rule.ReadPredicateAt(join)
	parent, parentPresent, parentOK := rule.ReadParentAt(join)
	keyVector, keyVectorPresent, keyVectorOK := rule.ReadKeyVectorAt(join)
	if !predicateOK || !parentOK || !keyVectorOK ||
		!generated.ReadFormAddressShape(form, predicate, predicatePresent, parent, parentPresent, keyVector, keyVectorPresent) {
		return SummaryContract{}, false
	}
	sealed := SummaryContract{Form: form, Contract: contract, Denominator: denominator}
	if !sealed.Available() {
		return SummaryContract{}, false
	}
	return sealed, true
}
