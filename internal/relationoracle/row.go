package relationoracle

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// ValueToken is the only semantic payload crossing the oracle boundary. The
// token is opaque here and carries its nominal TypeID plus a canonical content
// identity. A caller-owned AlgebraRegistry gives that type identity its
// equality and Join semantics.
type ValueToken struct {
	typeID  model.TypeID
	content identity.ContentID
}

// NewValueToken adopts an authenticated opaque value identity.
func NewValueToken(typeID model.TypeID, content identity.ContentID) (ValueToken, bool) {
	if !typeID.Available() || !content.Available() {
		return ValueToken{}, false
	}
	return ValueToken{typeID: typeID, content: content}, true
}

func (value ValueToken) Available() bool {
	return value.typeID.Available() && value.content.Available()
}
func (value ValueToken) Type() model.TypeID          { return value.typeID }
func (value ValueToken) Content() identity.ContentID { return value.content }
func (value ValueToken) Equal(other ValueToken) bool {
	return value.Available() && other.Available() && value.typeID == other.typeID && value.content == other.content
}

// Algebra compares and combines opaque values of one TypeID. Join must return
// a token of the same TypeID as its inputs. No domain implementation crosses
// this package boundary.
type Algebra interface {
	Equal(left ValueToken, right ValueToken) bool
	Join(left ValueToken, right ValueToken) ValueToken
}

// AlgebraEntry associates exactly one logical TypeID with its test algebra.
type AlgebraEntry struct {
	typeID  model.TypeID
	algebra Algebra
}

func NewAlgebraEntry(typeID model.TypeID, algebra Algebra) (AlgebraEntry, bool) {
	if !typeID.Available() || algebra == nil {
		return AlgebraEntry{}, false
	}
	return AlgebraEntry{typeID: typeID, algebra: algebra}, true
}

func (entry AlgebraEntry) Available() bool    { return entry.typeID.Available() && entry.algebra != nil }
func (entry AlgebraEntry) Type() model.TypeID { return entry.typeID }
func (entry AlgebraEntry) Algebra() Algebra   { return entry.algebra }

// AlgebraRegistry is an immutable TypeID-keyed lookup used by all semantic
// relational operators. Its storage is canonicalized by TypeID identity.
type AlgebraRegistry struct {
	entries []AlgebraEntry
	valid   bool
}

func NewAlgebraRegistry(entries []AlgebraEntry) (AlgebraRegistry, bool) {
	copyOf := append([]AlgebraEntry(nil), entries...)
	for _, entry := range copyOf {
		if !entry.Available() {
			return AlgebraRegistry{}, false
		}
	}
	sort.Slice(copyOf, func(left, right int) bool { return typeIDLess(copyOf[left].typeID, copyOf[right].typeID) })
	for index := 1; index < len(copyOf); index++ {
		if copyOf[index-1].typeID == copyOf[index].typeID {
			return AlgebraRegistry{}, false
		}
	}
	if copyOf == nil {
		copyOf = make([]AlgebraEntry, 0)
	}
	return AlgebraRegistry{entries: copyOf, valid: true}, true
}

func (registry AlgebraRegistry) Available() bool { return registry.valid && registry.entries != nil }
func (registry AlgebraRegistry) Entries() []AlgebraEntry {
	return append([]AlgebraEntry(nil), registry.entries...)
}

func (registry AlgebraRegistry) Lookup(typeID model.TypeID) (Algebra, bool) {
	if !registry.Available() || !typeID.Available() {
		return nil, false
	}
	index := sort.Search(len(registry.entries), func(index int) bool { return !typeIDLess(registry.entries[index].typeID, typeID) })
	if index >= len(registry.entries) || registry.entries[index].typeID != typeID {
		return nil, false
	}
	return registry.entries[index].algebra, true
}

// IdentityAlgebra is a deterministic opaque test algebra. Equality compares
// canonical value identities; Join derives a commutative identity and retains
// the operand TypeID. Domain tests that need richer semantics provide their
// own Algebra implementation and still use ValueToken storage.
type IdentityAlgebra struct{}

func (IdentityAlgebra) Equal(left ValueToken, right ValueToken) bool { return left.Equal(right) }
func (IdentityAlgebra) Join(left ValueToken, right ValueToken) ValueToken {
	if !left.Available() || !right.Available() || left.typeID != right.typeID {
		return ValueToken{}
	}
	if left.Equal(right) {
		return left
	}
	first, second := left.content, right.content
	if bytes.Compare(first[:], second[:]) > 0 {
		first, second = second, first
	}
	typeContent := left.typeID.Content()
	content, ok := identity.DeriveContentID("internal/relationoracle/value-join/v1", typeContent[:], first[:], second[:])
	if !ok {
		return ValueToken{}
	}
	return ValueToken{typeID: left.typeID, content: content}
}

// Cell is one immutable logical column value. Presence is separate from the
// value: Present with an opaque token is real, while UnprovenMissing and
// ProvenAbsent carry no value token. Refused is not a Cell presence; refusal
// belongs to semantic/outcome.Result at Apply.
type Cell struct {
	column   model.ColumnID
	typeID   model.TypeID
	value    ValueToken
	presence model.Presence
}

// NewCell validates the TypeID/value fence and rejects Refused as a cell
// status. Missing and absent cells retain their declared TypeID so
// heterogeneous relations remain representable after Complete.
func NewCell(column model.ColumnID, typeID model.TypeID, value ValueToken, presence model.Presence) (Cell, bool) {
	if !column.Available() || !typeID.Available() || !presence.Available() || presence.Is(model.Refused) {
		return Cell{}, false
	}
	if value.Available() != (presence.Is(model.Present) || presence.Is(model.AuthenticatedOpaque)) {
		return Cell{}, false
	}
	if value.Available() && value.typeID != typeID {
		return Cell{}, false
	}
	return Cell{column: column, typeID: typeID, value: value, presence: presence}, true
}

// PresentCell creates a present cell with an opaque typed value.
func PresentCell(column model.ColumnID, typeID model.TypeID, value ValueToken) (Cell, bool) {
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		return Cell{}, false
	}
	return NewCell(column, typeID, value, presence)
}

func MissingCell(column model.ColumnID, typeID model.TypeID) (Cell, bool) {
	presence, ok := model.NewPresence(model.UnprovenMissing)
	if !ok {
		return Cell{}, false
	}
	return NewCell(column, typeID, ValueToken{}, presence)
}

func AbsentCell(column model.ColumnID, typeID model.TypeID) (Cell, bool) {
	presence, ok := model.NewPresence(model.ProvenAbsent)
	if !ok {
		return Cell{}, false
	}
	return NewCell(column, typeID, ValueToken{}, presence)
}

func OpaqueCell(column model.ColumnID, typeID model.TypeID, value ValueToken) (Cell, bool) {
	presence, ok := model.NewPresence(model.AuthenticatedOpaque)
	if !ok {
		return Cell{}, false
	}
	return NewCell(column, typeID, value, presence)
}

func (cell Cell) Available() bool {
	return cell.column.Available() && cell.typeID.Available() && cell.presence.Available() && !cell.presence.Is(model.Refused)
}
func (cell Cell) Column() model.ColumnID   { return cell.column }
func (cell Cell) Type() model.TypeID       { return cell.typeID }
func (cell Cell) Presence() model.Presence { return cell.presence }

// Value returns an authenticated token only for present/opaque cells.
func (cell Cell) Value() (ValueToken, bool) {
	if !cell.Available() || (!cell.presence.Is(model.Present) && !cell.presence.Is(model.AuthenticatedOpaque)) {
		return ValueToken{}, false
	}
	return cell.value, true
}

// Row is an immutable logical row keyed by one model.RowID. Cells are sorted
// by nominal ColumnID only; local ordinals never enter logical identity.
type Row struct {
	id    model.RowID
	scope Scope
	cells []Cell
	valid bool
}

func NewRow(id model.RowID, scope Scope, cells []Cell) (Row, bool) {
	if !id.Available() || !scope.Available() {
		return Row{}, false
	}
	copyOf := append([]Cell(nil), cells...)
	for _, cell := range copyOf {
		if !cell.Available() || cell.column.Relation() != id.Relation() {
			return Row{}, false
		}
	}
	sort.Slice(copyOf, func(left, right int) bool { return columnLess(copyOf[left].column, copyOf[right].column) })
	for index := 1; index < len(copyOf); index++ {
		if copyOf[index-1].column == copyOf[index].column {
			return Row{}, false
		}
	}
	return Row{id: id, scope: scope, cells: copyOf, valid: true}, true
}

func (row Row) Available() bool { return row.valid && row.id.Available() && row.scope.Available() }
func (row Row) ID() model.RowID { return row.id }
func (row Row) Scope() Scope    { return row.scope }
func (row Row) Cells() []Cell   { return append([]Cell(nil), row.cells...) }

func (row Row) Cell(column model.ColumnID) (Cell, bool) {
	if !row.Available() || !column.Available() {
		return Cell{}, false
	}
	index := sort.Search(len(row.cells), func(index int) bool { return !columnLess(row.cells[index].column, column) })
	if index >= len(row.cells) || row.cells[index].column != column {
		return Cell{}, false
	}
	return row.cells[index], true
}

// Relation is an immutable logical relation keyed only by model logical IDs.
type Relation struct {
	id    model.RelationID
	rows  []Row
	valid bool
}

func NewRelation(id model.RelationID, rows []Row) (Relation, bool) {
	if !id.Available() {
		return Relation{}, false
	}
	copyOf := append([]Row(nil), rows...)
	for _, row := range copyOf {
		if !row.Available() || row.id.Relation() != id {
			return Relation{}, false
		}
	}
	sort.Slice(copyOf, func(left, right int) bool { return rowLess(copyOf[left], copyOf[right]) })
	for index := 1; index < len(copyOf); index++ {
		if copyOf[index-1].id == copyOf[index].id {
			return Relation{}, false
		}
	}
	if copyOf == nil {
		copyOf = make([]Row, 0)
	}
	return Relation{id: id, rows: copyOf, valid: true}, true
}

func EmptyRelation(id model.RelationID) (Relation, bool) { return NewRelation(id, nil) }
func (relation Relation) Available() bool {
	return relation.valid && relation.id.Available() && relation.rows != nil
}
func (relation Relation) ID() model.RelationID { return relation.id }
func (relation Relation) Rows() []Row          { return append([]Row(nil), relation.rows...) }
func (relation Relation) Scan() []Row          { return relation.Rows() }
func Input(relation Relation) []Row            { return relation.Scan() }

func (relation Relation) Row(id model.RowID) (Row, bool) {
	if !relation.Available() || !id.Available() || id.Relation() != relation.id {
		return Row{}, false
	}
	index := sort.Search(len(relation.rows), func(index int) bool { return !rowIDLess(relation.rows[index].id, id) })
	if index >= len(relation.rows) || relation.rows[index].id != id {
		return Row{}, false
	}
	return relation.rows[index], true
}

func columnLess(left, right model.ColumnID) bool {
	leftOwner, rightOwner := left.Owner().Content(), right.Owner().Content()
	if comparison := bytes.Compare(leftOwner[:], rightOwner[:]); comparison != 0 {
		return comparison < 0
	}
	leftContent, rightContent := left.Content(), right.Content()
	return bytes.Compare(leftContent[:], rightContent[:]) < 0
}

func typeIDLess(left, right model.TypeID) bool {
	leftOwner, rightOwner := left.Owner().Content(), right.Owner().Content()
	if comparison := bytes.Compare(leftOwner[:], rightOwner[:]); comparison != 0 {
		return comparison < 0
	}
	leftContent, rightContent := left.Content(), right.Content()
	return bytes.Compare(leftContent[:], rightContent[:]) < 0
}

func rowIDLess(left, right model.RowID) bool {
	leftRelation, rightRelation := left.Relation(), right.Relation()
	leftOwner, rightOwner := leftRelation.Owner().Content(), rightRelation.Owner().Content()
	if comparison := bytes.Compare(leftOwner[:], rightOwner[:]); comparison != 0 {
		return comparison < 0
	}
	leftRelationContent, rightRelationContent := leftRelation.Content(), rightRelation.Content()
	if comparison := bytes.Compare(leftRelationContent[:], rightRelationContent[:]); comparison != 0 {
		return comparison < 0
	}
	leftContent, rightContent := left.Content(), right.Content()
	return bytes.Compare(leftContent[:], rightContent[:]) < 0
}

func rowLess(left, right Row) bool { return rowIDLess(left.id, right.id) }

func deriveRowID(relation model.RelationID, tag string, rows ...model.RowID) (model.RowID, bool) {
	if !relation.Available() || tag == "" {
		return model.RowID{}, false
	}
	parts := make([][]byte, 0, 1+2*len(rows))
	relationContent := relation.Content()
	parts = append(parts, relationContent[:])
	for _, row := range rows {
		if !row.Available() {
			return model.RowID{}, false
		}
		ownerContent := row.Owner().Content()
		rowContent := row.Content()
		parts = append(parts, ownerContent[:], rowContent[:])
	}
	content, ok := identity.DeriveContentID(tag, parts...)
	if !ok {
		return model.RowID{}, false
	}
	return model.IssueRowID(relation, content)
}
