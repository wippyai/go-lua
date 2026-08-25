package binding

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Fence identifies the solve-local runtime that authenticated a token.  It is
// deliberately independent of a signature owner and of any invocation scope:
// one operation may join cells issued by several logical owners and scopes.
type Fence struct {
	schema     model.SchemaID
	mount      identity.MountID
	generation identity.Generation
}

// NewFence adopts one mounted schema revision as the runtime token authority.
func NewFence(schema model.SchemaID, mount identity.MountID, generation identity.Generation) (Fence, bool) {
	if !schema.Available() || !mount.Available() || !generation.Available() {
		return Fence{}, false
	}
	return Fence{schema: schema, mount: mount, generation: generation}, true
}

func (fence Fence) Available() bool {
	return fence.schema.Available() && fence.mount.Available() && fence.generation.Available()
}

func (fence Fence) Schema() model.SchemaID          { return fence.schema }
func (fence Fence) Mount() identity.MountID         { return fence.mount }
func (fence Fence) Generation() identity.Generation { return fence.generation }
func (fence Fence) Same(other Fence) bool {
	return fence.Available() && other.Available() && fence == other
}
func (fence Fence) validFor(other Fence) bool {
	return fence.Available() && other.Available() && fence == other
}

// Issuer is the solve-local authority for runtime-fenced opaque tokens.
// Opaque handles are carried but never interpreted here.
type Issuer struct{ fence Fence }

func NewIssuer(fence Fence) (Issuer, bool) {
	if !fence.Available() {
		return Issuer{}, false
	}
	return Issuer{fence: fence}, true
}

func (issuer Issuer) Available() bool { return issuer.fence.Available() }
func (issuer Issuer) Fence() Fence    { return issuer.fence }

// ScopeToken is an already-conjoined invocation formula authenticated by the
// mounted runtime.  The logical scope declarations that contributed to the
// formula are intentionally absent: a conjunction may span several declared
// ScopeIDs and therefore has no single nominal scope identity here.  Mount
// resolution owns the formula/proof construction; binding only carries its
// opaque canonical identity and runtime fence.
type ScopeToken struct {
	fence  Fence
	opaque identity.ContentID
}

// IssueScope authenticates one canonical formula/proof identity for this
// mounted runtime.  The identity is opaque to binding; scope algebra and
// entailment remain mount responsibilities.
func (issuer Issuer) IssueScope(opaque identity.ContentID) (ScopeToken, bool) {
	if !issuer.Available() || !opaque.Available() {
		return ScopeToken{}, false
	}
	return ScopeToken{fence: issuer.fence, opaque: opaque}, true
}

func (scope ScopeToken) Available() bool {
	return scope.fence.Available() && scope.opaque.Available()
}

func (scope ScopeToken) ValidFor(fence Fence) bool {
	return scope.Available() && scope.fence.validFor(fence)
}
func (scope ScopeToken) Same(other ScopeToken) bool {
	return scope.Available() && other.Available() && scope == other
}
func (issuer Issuer) AuthenticateScope(scope ScopeToken) bool {
	return scope.ValidFor(issuer.fence)
}

// MembershipView is an immutable logical view of the rows in one denominator.
// Construction copies the caller's row vector once; worker calls only borrow
// this stable storage and perform logical RowID membership checks.
type MembershipView struct {
	relation model.RelationID
	rows     []model.RowID
}

func NewMembershipView(relation model.RelationID, rows []model.RowID) (MembershipView, bool) {
	if !relation.Available() || rows == nil {
		return MembershipView{}, false
	}
	copyOf := make([]model.RowID, len(rows))
	for index, row := range rows {
		if !row.Available() || row.Relation() != relation {
			return MembershipView{}, false
		}
		for _, prior := range copyOf[:index] {
			if prior == row {
				return MembershipView{}, false
			}
		}
		copyOf[index] = row
	}
	return MembershipView{relation: relation, rows: copyOf}, true
}

func (view MembershipView) Available() bool {
	if !view.relation.Available() || view.rows == nil {
		return false
	}
	for _, row := range view.rows {
		if !row.Available() || row.Relation() != view.relation {
			return false
		}
	}
	return true
}

func (view MembershipView) Relation() model.RelationID { return view.relation }
func (view MembershipView) Len() int                   { return len(view.rows) }

func (view MembershipView) At(index int) (model.RowID, bool) {
	if !view.Available() || index < 0 || index >= len(view.rows) {
		return model.RowID{}, false
	}
	return view.rows[index], true
}

func (view MembershipView) Index(row model.RowID) (int, bool) {
	if !view.Available() || !row.Available() {
		return 0, false
	}
	for index, candidate := range view.rows {
		if candidate == row {
			return index, true
		}
	}
	return 0, false
}

func (view MembershipView) Contains(row model.RowID) bool {
	if !view.Available() || !row.Available() {
		return false
	}
	for _, candidate := range view.rows {
		if candidate == row {
			return true
		}
	}
	return false
}

func (view MembershipView) Same(other MembershipView) bool {
	if !view.Available() || !other.Available() || view.relation != other.relation || len(view.rows) != len(other.rows) {
		return false
	}
	for index, row := range view.rows {
		if row != other.rows[index] {
			return false
		}
	}
	return true
}

// DenominatorWitness authenticates both the logical denominator and the
// mounted membership view used to issue rows. A bare RowID never establishes
// membership.
type DenominatorWitness struct {
	fence      Fence
	relation   model.RelationID
	key        model.KeyID
	membership MembershipView
	opaque     identity.ContentID
}

func (issuer Issuer) IssueDenominator(ref model.DenominatorRef, membership MembershipView, opaque identity.ContentID) (DenominatorWitness, bool) {
	if !issuer.Available() || !ref.Available() || !membership.Available() || membership.Relation() != ref.Relation() || !opaque.Available() {
		return DenominatorWitness{}, false
	}
	return DenominatorWitness{fence: issuer.fence, relation: ref.Relation(), key: ref.Key(), membership: membership, opaque: opaque}, true
}

func (witness DenominatorWitness) Available() bool {
	return witness.fence.Available() && witness.relation.Available() && witness.key.Available() && witness.key.Relation() == witness.relation && witness.membership.Available() && witness.membership.Relation() == witness.relation && witness.opaque.Available()
}

func (witness DenominatorWitness) Relation() model.RelationID { return witness.relation }
func (witness DenominatorWitness) Key() model.KeyID           { return witness.key }
func (witness DenominatorWitness) ValidFor(fence Fence) bool {
	return witness.Available() && witness.fence.validFor(fence)
}
func (witness DenominatorWitness) Matches(ref model.DenominatorRef) bool {
	return witness.Available() && ref.Available() && witness.relation == ref.Relation() && witness.key == ref.Key()
}
func (witness DenominatorWitness) Contains(row model.RowID) bool {
	return witness.Available() && witness.membership.Contains(row)
}

// Index resolves one authenticated logical row to the stable position in the
// witness-owned membership view. The view remains the sole row-order source;
// callers cannot replace or reorder it through this projection.
func (witness DenominatorWitness) Index(row model.RowID) (int, bool) {
	if !witness.Available() {
		return 0, false
	}
	return witness.membership.Index(row)
}

// Evidence returns the owner-issued identity that authenticated this
// denominator membership snapshot.
func (witness DenominatorWitness) Evidence() (identity.ContentID, bool) {
	if !witness.Available() {
		return identity.ContentID{}, false
	}
	return witness.opaque, true
}

func (witness DenominatorWitness) Same(other DenominatorWitness) bool {
	return witness.Available() && other.Available() && witness.fence == other.fence && witness.relation == other.relation && witness.key == other.key && witness.opaque == other.opaque && witness.membership.Same(other.membership)
}
func (issuer Issuer) AuthenticateDenominator(witness DenominatorWitness) bool {
	return witness.ValidFor(issuer.fence)
}

// CellToken is a mounted, scope-qualified address for one logical row/column.
// Its witness is tied to the cell's own denominator, so inputs from different
// relations may coexist in one frame.
type CellToken struct {
	fence    Fence
	witness  DenominatorWitness
	scope    ScopeToken
	relation model.RelationID
	column   model.ColumnID
	row      model.RowID
}

func (issuer Issuer) IssueCell(witness DenominatorWitness, scope ScopeToken, column model.ColumnID, row model.RowID) (CellToken, bool) {
	if !issuer.Available() || !witness.ValidFor(issuer.fence) || !scope.ValidFor(issuer.fence) || !column.Available() || !row.Available() || column.Relation() != witness.relation || row.Relation() != witness.relation || !witness.Contains(row) {
		return CellToken{}, false
	}
	return CellToken{fence: issuer.fence, witness: witness, scope: scope, relation: witness.relation, column: column, row: row}, true
}

func (cell CellToken) Available() bool {
	return cell.fence.Available() && cell.witness.ValidFor(cell.fence) && cell.scope.ValidFor(cell.fence) && cell.relation.Available() && cell.column.Available() && cell.column.Relation() == cell.relation && cell.row.Available() && cell.row.Relation() == cell.relation && cell.witness.Contains(cell.row)
}

func (cell CellToken) Fence() Fence                { return cell.fence }
func (cell CellToken) Witness() DenominatorWitness { return cell.witness }
func (cell CellToken) Scope() ScopeToken           { return cell.scope }
func (cell CellToken) Relation() model.RelationID  { return cell.relation }
func (cell CellToken) Column() model.ColumnID      { return cell.column }
func (cell CellToken) Row() model.RowID            { return cell.row }
func (cell CellToken) ValidFor(fence Fence) bool {
	return cell.Available() && cell.fence.validFor(fence)
}
func (cell CellToken) Same(other CellToken) bool {
	return cell.Available() && other.Available() && cell.fence == other.fence && cell.relation == other.relation && cell.column == other.column && cell.row == other.row && cell.witness.Same(other.witness) && cell.scope.Same(other.scope)
}
func (issuer Issuer) AuthenticateCell(cell CellToken) bool {
	return cell.ValidFor(issuer.fence)
}

// ValueToken binds an encoded opaque value to its declared logical type and
// mounted runtime. Type owners are intentionally independent of operation
// owners: cross-owner joins are valid when the signature declares them.
type ValueToken struct {
	fence  Fence
	typeID model.TypeID
	opaque identity.ContentID
}

func (issuer Issuer) IssueValue(typeID model.TypeID, opaque identity.ContentID) (ValueToken, bool) {
	if !issuer.Available() || !typeID.Available() || !opaque.Available() {
		return ValueToken{}, false
	}
	return ValueToken{fence: issuer.fence, typeID: typeID, opaque: opaque}, true
}

func (value ValueToken) Available() bool {
	return value.fence.Available() && value.typeID.Available() && value.opaque.Available()
}

func (value ValueToken) Fence() Fence               { return value.fence }
func (value ValueToken) Type() model.TypeID         { return value.typeID }
func (value ValueToken) Opaque() identity.ContentID { return value.opaque }
func (value ValueToken) ValidFor(fence Fence) bool {
	return value.Available() && value.fence.validFor(fence)
}
func (value ValueToken) Same(other ValueToken) bool {
	return value.Available() && other.Available() && value.fence == other.fence && value.typeID == other.typeID && value.opaque == other.opaque
}
func (issuer Issuer) AuthenticateValue(value ValueToken) bool { return value.ValidFor(issuer.fence) }

// Cell is the typed semantic payload at one authenticated address. Presence
// and value are independent: absent or unproven cells carry no value token.
type Cell struct {
	address  CellToken
	typeID   model.TypeID
	value    ValueToken
	presence model.Presence
}

func NewCell(address CellToken, typeID model.TypeID, value ValueToken, presence model.Presence) (Cell, bool) {
	if !address.Available() || !typeID.Available() || !presence.Available() || presence.Is(model.Refused) || !valueMatches(value, presence, address.Fence()) {
		return Cell{}, false
	}
	if value.Available() && value.Type() != typeID {
		return Cell{}, false
	}
	return Cell{address: address, typeID: typeID, value: value, presence: presence}, true
}

func (cell Cell) Available() bool {
	return cell.address.Available() && cell.typeID.Available() && cell.presence.Available() && !cell.presence.Is(model.Refused) && valueMatches(cell.value, cell.presence, cell.address.Fence()) && (!cell.value.Available() || cell.value.Type() == cell.typeID)
}

func valueMatches(value ValueToken, presence model.Presence, fence Fence) bool {
	if !presence.Available() || presence.Is(model.Refused) {
		return false
	}
	if presence.Is(model.Present) || presence.Is(model.AuthenticatedOpaque) {
		return value.ValidFor(fence)
	}
	return !value.Available()
}

func (cell Cell) Address() CellToken       { return cell.address }
func (cell Cell) Type() model.TypeID       { return cell.typeID }
func (cell Cell) Value() ValueToken        { return cell.value }
func (cell Cell) Presence() model.Presence { return cell.presence }

type Span struct {
	cells  []Cell
	start  uint32
	length uint32
}

var emptyCells = []Cell{}

func NewSpan(cells []Cell, start, length uint32) (Span, bool) {
	if cells == nil || uint64(start)+uint64(length) > uint64(len(cells)) {
		return Span{}, false
	}
	for _, cell := range cells {
		if !cell.Available() {
			return Span{}, false
		}
	}
	return Span{cells: cells, start: start, length: length}, true
}

func (span Span) Available() bool {
	if span.cells == nil || uint64(span.start)+uint64(span.length) > uint64(len(span.cells)) {
		return false
	}
	for _, cell := range span.cells {
		if !cell.Available() {
			return false
		}
	}
	return true
}

func (span Span) Len() int { return int(span.length) }
func (span Span) At(index int) (Cell, bool) {
	if !span.Available() || index < 0 || index >= int(span.length) {
		return Cell{}, false
	}
	return span.cells[int(span.start)+index], true
}

type slotKind uint8

const (
	scalarSlot slotKind = iota + 1
	spanSlot
)

// Slot is a borrowed scalar or span view. It holds no ownership of the cell
// vector, so constructing a frame does not copy or allocate invocation data.
type Slot struct {
	kind         slotKind
	single       Cell
	cells        []Cell
	span         Span
	rangeWitness DenominatorWitness
}

func NewScalarSlot(cell Cell) (Slot, bool) {
	if !cell.Available() {
		return Slot{}, false
	}
	return Slot{kind: scalarSlot, single: cell}, true
}

func NewSpanSlot(cells []Cell) (Slot, bool) {
	if cells == nil || len(cells) == 0 {
		return Slot{}, false
	}
	span, ok := NewSpan(cells, 0, uint32(len(cells)))
	if !ok {
		return Slot{}, false
	}
	return Slot{kind: spanSlot, cells: cells, span: span, rangeWitness: cells[0].Address().Witness()}, true
}

// NewEmptySpanSlot carries an authenticated empty range. It is the only way
// to represent an empty complete delivery: no cell exists from which a range
// witness could otherwise be recovered.
func NewEmptySpanSlot(witness DenominatorWitness) (Slot, bool) {
	if !witness.Available() {
		return Slot{}, false
	}
	span, ok := NewSpan(emptyCells, 0, 0)
	if !ok {
		return Slot{}, false
	}
	return Slot{kind: spanSlot, cells: emptyCells, span: span, rangeWitness: witness}, true
}

func (slot Slot) Available() bool {
	switch slot.kind {
	case scalarSlot:
		return slot.single.Available()
	case spanSlot:
		return slot.cells != nil && slot.rangeWitness.Available() && slot.span.Available()
	default:
		return false
	}
}

func (slot Slot) Kind() uint8 { return uint8(slot.kind) }
func (slot Slot) IsScalar() bool {
	return slot.kind == scalarSlot && slot.Available()
}
func (slot Slot) IsSpan() bool {
	return slot.kind == spanSlot && slot.Available()
}
func (slot Slot) Len() int {
	switch slot.kind {
	case scalarSlot:
		return 1
	case spanSlot:
		return slot.span.Len()
	default:
		return 0
	}
}

func (slot Slot) At(index int) (Cell, bool) {
	switch slot.kind {
	case scalarSlot:
		if !slot.Available() || index != 0 {
			return Cell{}, false
		}
		return slot.single, true
	case spanSlot:
		return slot.span.At(index)
	default:
		return Cell{}, false
	}
}

func (slot Slot) Span() (Span, bool) {
	if slot.kind != spanSlot || !slot.Available() {
		return Span{}, false
	}
	return slot.span, true
}

func (slot Slot) Witness() DenominatorWitness {
	if slot.kind != spanSlot {
		return DenominatorWitness{}
	}
	return slot.rangeWitness
}
