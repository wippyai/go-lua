package query

import (
	projection "github.com/wippyai/go-lua/analysis/engine/relation/runtime/snapshot"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	canonical "github.com/wippyai/go-lua/analysis/snapshot"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// FactColumn seals the logical Placement column identity together with the
// owner-issued Fact codec. The identity is checked again against the
// projection at read time, so a codec from another mounted axis cannot redeem
// a token merely because the Go payload type is the same.
type FactColumn struct {
	id     model.ColumnID
	typeID model.TypeID
	codec  *relbindgen.Column[placementdomain.Fact]
}

// NewFactColumn adopts one owner-issued Placement Fact column and its codec.
// The type captured here is immutable and must agree with the codec's own
// type; no default Fact is manufactured for an unavailable declaration.
func NewFactColumn(id model.ColumnID, codec *relbindgen.Column[placementdomain.Fact]) (FactColumn, bool) {
	if !id.Available() || codec == nil || !codec.Available() {
		return FactColumn{}, false
	}
	typeID := codec.Type()
	if !typeID.Available() {
		return FactColumn{}, false
	}
	return FactColumn{id: id, typeID: typeID, codec: codec}, true
}

// Available reports whether the column carries one immutable identity and a
// live owner codec.
func (column FactColumn) Available() bool {
	return column.id.Available() && column.typeID.Available() && column.codec != nil && column.codec.Available() && column.codec.Type() == column.typeID
}

// ID returns the sealed logical Placement Fact column identity.
func (column FactColumn) ID() model.ColumnID { return column.id }

// Type returns the owner-issued semantic type carried by the codec.
func (column FactColumn) Type() model.TypeID { return column.typeID }

// FactRow is one immutable Placement row redeemed from a canonical snapshot.
// Key retains the authenticated relation row and runtime scope. Presence and
// Lineage remain separate from the semantic Fact: ProvenAbsent, UnprovenMissing
// and Refused rows carry no Fact, while Present and AuthenticatedOpaque rows
// carry the decoded owner value.
type FactRow struct {
	key      projection.RowKey
	fact     placementdomain.Fact
	presence model.Presence
	lineage  model.LineageRef
	hasFact  bool
}

// Available reports whether the row's metadata is complete and its semantic
// payload agrees with its presence. A non-present row is still a valid typed
// row when its absence/refusal status and lineage are authenticated.
func (row FactRow) Available() bool {
	if !row.key.Available() || !row.presence.Available() {
		return false
	}
	// A canonical denominator-only proven absence has no cell and therefore
	// no lineage sidecar. Explicit absent/refused cells still carry lineage.
	if !row.lineage.Available() && !row.presence.Is(model.ProvenAbsent) {
		return false
	}
	if row.presence.Is(model.Present) || row.presence.Is(model.AuthenticatedOpaque) {
		return row.hasFact && row.fact.Valid()
	}
	return !row.hasFact
}

// Key returns the immutable logical address, including its authenticated
// scope token.
func (row FactRow) Key() projection.RowKey { return row.key }

// Presence returns the exact semantic presence state; no absent or refused
// state is turned into a domain lattice value.
func (row FactRow) Presence() model.Presence { return row.presence }

// Lineage returns the proof-sidecar identity attached to this row.
func (row FactRow) Lineage() model.LineageRef { return row.lineage }

// HasLineage reports whether this row carries an explicit proof-sidecar
// identity. A denominator-only ProvenAbsent row is valid without one because
// the canonical denominator itself is its absence proof.
func (row FactRow) HasLineage() bool { return row.Available() && row.lineage.Available() }

// Fact returns the redeemed Placement Fact only for Present or
// AuthenticatedOpaque rows. It deliberately returns no Bottom/Unknown
// substitute for absence, refusal, or a failed decode.
func (row FactRow) Fact() (placementdomain.Fact, bool) {
	if !row.Available() || !row.hasFact {
		return placementdomain.Fact{}, false
	}
	return row.fact, true
}

// Rows is an immutable canonical-order collection of typed Placement rows.
// Its backing slice is never exposed; At returns a value copy.
type Rows struct{ rows []FactRow }

// Available reports whether the collection was sealed. A sealed empty set is
// valid and remains distinct from the zero value.
func (rows Rows) Available() bool { return rows.rows != nil }

// Len returns the number of rows in the sealed collection.
func (rows Rows) Len() int {
	if !rows.Available() {
		return 0
	}
	return len(rows.rows)
}

// At returns one row in the projection's stable denominator order.
func (rows Rows) At(index int) (FactRow, bool) {
	if !rows.Available() || index < 0 || index >= len(rows.rows) {
		return FactRow{}, false
	}
	return rows.rows[index], true
}

func validFactColumn(published projection.Projection, column FactColumn) bool {
	if !published.Available() || !column.Available() {
		return false
	}
	declared, ok := published.Column(column.id)
	return ok && declared.Available() && declared.ID == column.id && declared.Type == column.typeID
}

// ReadOne redeems one explicitly named logical row. It is the focused
// nearest-negative seam for callers that need to distinguish a proven
// denominator absence, an uncovered miss, and an invalid/foreign address.
// The final bool reports whether a typed row was produced; status remains the
// canonical snapshot outcome even when that bool is false.
func ReadOne(published projection.Projection, column FactColumn, key projection.RowKey) (FactRow, canonical.ReadStatus, bool) {
	if !validFactColumn(published, column) {
		return FactRow{}, canonical.ReadInvalid, false
	}
	cell, status := published.Read(column.id, key)
	switch status {
	case canonical.ReadProvenAbsent:
		presence, ok := model.NewPresence(model.ProvenAbsent)
		if !ok {
			return FactRow{}, canonical.ReadInvalid, false
		}
		row := FactRow{key: key, presence: presence}
		return row, status, row.Available()
	case canonical.ReadHit:
		if !cell.Available() || cell.Column != column.id || cell.Type != column.typeID {
			return FactRow{}, canonical.ReadInvalid, false
		}
		row := FactRow{key: key, presence: cell.Presence, lineage: cell.Lineage}
		if cell.Presence.Is(model.Present) || cell.Presence.Is(model.AuthenticatedOpaque) {
			fact, decoded := column.codec.Decode(cell.Value)
			if !decoded || !fact.Valid() {
				return FactRow{}, canonical.ReadInvalid, false
			}
			row.fact, row.hasFact = fact, true
		} else if cell.Value.Available() {
			return FactRow{}, canonical.ReadInvalid, false
		}
		return row, status, row.Available()
	default:
		return FactRow{}, status, false
	}
}

// Read redeems the Placement Fact column from one canonical projection. It
// enumerates the projection's immutable RowKey members in publisher order,
// reads each cell through the generic snapshot seam, and decodes only cells
// whose sealed logical type and column identity agree with the owner codec.
// Any malformed row, wrong type, foreign scope, or failed decode refuses the
// whole read; no partial typed collection is returned.
func Read(published projection.Projection, column FactColumn) (Rows, bool) {
	if !published.Available() || !column.Available() {
		return Rows{}, false
	}
	if !validFactColumn(published, column) {
		return Rows{}, false
	}
	keys := published.Keys(column.id)
	if keys == nil {
		return Rows{}, false
	}
	rows := make([]FactRow, 0, len(keys))
	for _, key := range keys {
		row, _, ok := ReadOne(published, column, key)
		if !ok {
			return Rows{}, false
		}
		rows = append(rows, row)
	}
	sealed := append([]FactRow(nil), rows...)
	if sealed == nil {
		sealed = []FactRow{}
	}
	return Rows{rows: sealed}, true
}
