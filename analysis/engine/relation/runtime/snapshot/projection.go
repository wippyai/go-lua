package snapshot

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/relation/runtime/terminal"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	canonical "github.com/wippyai/go-lua/analysis/snapshot"
)

const (
	columnDirectoryTag = "analysis/engine/relation/runtime/snapshot/column/v1"
	denominatorTag     = "analysis/engine/relation/runtime/snapshot/denominator/v1"
)

// RowKey is the stable logical address of one projected cell.  Relation and
// Row are owner-issued identities; Scope is the authenticated normalized
// decision fiber.  The physical geometry key never crosses this boundary.
//
// Scope is part of the key because one logical row may carry distinct cells
// on disjoint runtime fibers.  It is not a second scope authority: the
// mounted witness and geometry remain the only issuers and normalizers.
type RowKey struct {
	Relation model.RelationID
	Row      model.RowID
	Scope    witness.Scope
}

// Available reports whether the key carries one relation-consistent logical
// row and an authenticated scope token.
func (key RowKey) Available() bool {
	return key.Relation.Available() && key.Row.Available() && key.Row.Relation() == key.Relation && key.Scope.Available()
}

// ValidFor reports whether the key's scope belongs to fence.  Identity
// availability alone is not enough to redeem a key from another runtime.
func (key RowKey) ValidFor(fence binding.Fence) bool {
	return key.Available() && key.Scope.ValidFor(fence)
}

// Cell is the generic semantic payload stored at a projected logical key.
// Value remains an opaque authenticated ValueToken; no legacy result carrier
// or domain value is reconstructed here.  Presence and lineage are retained
// as their separate semantic/proof algebras.
type Cell struct {
	Column   model.ColumnID
	Type     model.TypeID
	Presence model.Presence
	Value    binding.ValueToken
	Lineage  model.LineageRef
}

// Available reports whether the cell's independent components are complete.
// A value is required exactly for Present and AuthenticatedOpaque cells;
// ProvenAbsent, UnprovenMissing, and Refused carry no value token.
func (cell Cell) Available() bool {
	if !cell.Column.Available() || !cell.Type.Available() || !cell.Presence.Available() || !cell.Lineage.Available() {
		return false
	}
	if cell.Presence.Is(model.Present) || cell.Presence.Is(model.AuthenticatedOpaque) {
		return cell.Value.Available() && cell.Value.Type() == cell.Type
	}
	return !cell.Value.Available()
}

// ValidFor reports whether any carried value token is authenticated by fence.
// The token's type is checked by Available, while its runtime authority is
// checked here.
func (cell Cell) ValidFor(fence binding.Fence) bool {
	if !cell.Available() {
		return false
	}
	return !cell.Value.Available() || cell.Value.ValidFor(fence)
}

// Column declares one immutable canonical snapshot axis.  Axis is retained
// as an opaque declaration rather than a second column store.  PublicationID
// and DenominatorID are stable ContentID directory identities derived from
// the owner-issued schema/column IDs; they are not row or fact authorities.
type Column struct {
	ID            model.ColumnID
	Type          model.TypeID
	PublicationID identity.ContentID
	DenominatorID identity.ContentID
	axis          canonical.Axis[RowKey, Cell]
}

// Available reports whether the declaration names one typed canonical axis.
func (column Column) Available() bool {
	return column.ID.Available() && column.Type.Available() && column.PublicationID.Available() && column.DenominatorID.Available() && column.axis.Available() && column.ID.Relation().Available()
}

// Axis returns the typed canonical snapshot address.  The returned value is
// only a declaration; it retains no mutable builder or column storage.
func (column Column) Axis() canonical.Axis[RowKey, Cell] { return column.axis }

// Projection is the immutable declared projection of one terminal relation
// root.  It retains the canonical analysis/snapshot.Snapshot by value and a
// stable column-to-axis declaration vector; it does not retain a database
// root, evaluator result, domain payload, or alternate row store.
type Projection struct {
	published canonical.Snapshot
	columns   []Column
	fence     binding.Fence
	sealed    bool
}

// Available reports whether the canonical publication and its declarations
// are complete.  The constructor establishes this proof once; the retained
// values are immutable thereafter.
func (projection Projection) Available() bool {
	if !projection.sealed || !projection.published.Published() || !projection.fence.Available() || projection.columns == nil || len(projection.columns) == 0 {
		return false
	}
	for index, column := range projection.columns {
		if !column.Available() || column.axis.SchemaID != projection.published.Schema() {
			return false
		}
		for _, prior := range projection.columns[:index] {
			if prior.ID == column.ID {
				return false
			}
		}
	}
	return true
}

// Snapshot returns the one canonical immutable publication.  A zero value is
// returned when projection is unavailable.
func (projection Projection) Snapshot() canonical.Snapshot {
	if !projection.Available() {
		return canonical.Snapshot{}
	}
	return projection.published
}

// Schema returns the logical schema identity of the canonical publication.
func (projection Projection) Schema() identity.ContentID {
	if !projection.Available() {
		return identity.ContentID{}
	}
	return projection.published.Schema()
}

// Store returns the process-local store identity of the canonical
// publication.  It is inherited from the mounted root and is never derived
// from content.
func (projection Projection) Store() identity.StoreID {
	if !projection.Available() {
		return 0
	}
	return projection.published.Store()
}

// Generation returns the aggregate root revision published by this
// projection.
func (projection Projection) Generation() identity.Generation {
	if !projection.Available() {
		return 0
	}
	return projection.published.Generation()
}

// Columns returns the canonical declaration order defensively.
func (projection Projection) Columns() []Column {
	if !projection.Available() {
		return nil
	}
	return append([]Column(nil), projection.columns...)
}

// Column resolves a stable logical column identity without exposing a second
// mutable directory.  Mounted column order is canonical and the projection
// vector is immutable, so a linear lookup is sufficient at this seam.
func (projection Projection) Column(id model.ColumnID) (Column, bool) {
	if !projection.Available() || !id.Available() {
		return Column{}, false
	}
	for _, column := range projection.columns {
		if column.ID == id {
			return column, true
		}
	}
	return Column{}, false
}

// Read resolves one typed cell through the canonical snapshot.  Unknown
// columns and malformed keys are rejected as ReadInvalid; no default fact is
// manufactured.
func (projection Projection) Read(id model.ColumnID, key RowKey) (Cell, canonical.ReadStatus) {
	var zero Cell
	column, ok := projection.Column(id)
	if !ok || !key.ValidFor(projection.fence) {
		return zero, canonical.ReadInvalid
	}
	return canonical.Read(&projection.published, column.axis, key)
}

// Keys enumerates the immutable denominator members attached to one
// projected column.  The membership is borrowed from the canonical snapshot;
// this package does not maintain a parallel row directory.
func (projection Projection) Keys(id model.ColumnID) []RowKey {
	column, ok := projection.Column(id)
	if !ok {
		return nil
	}
	count, ok := canonical.MemberCountAtAxis(&projection.published, column.axis)
	if !ok || count == 0 {
		return []RowKey{}
	}
	keys := make([]RowKey, 0, count)
	for index := 0; index < count; index++ {
		key, ok := canonical.MemberAtAxis(&projection.published, column.axis, index)
		if !ok {
			return nil
		}
		keys = append(keys, key)
	}
	return keys
}

// Publish projects the terminal replacement runtime result through the
// existing immutable snapshot builder.  The terminal relation root remains
// the sole fact authority: only its public store scan is read, and no root or
// engine result payload is retained by the returned Projection.
//
// Any malformed, foreign, duplicate, or unnormalizable fact refuses the
// whole publication.  There is intentionally one runtime path and no partial
// publication path.
func Publish(result terminal.Result, view geometry.Geometry) (Projection, bool) {
	if !result.Available() || !view.Available() {
		return Projection{}, false
	}
	root := result.Root()
	mounted := root.Mounted()
	if !mounted.Available() || !view.ValidFor(mounted) {
		return Projection{}, false
	}

	addressFence := mounted.Fence()
	runtimeFence := root.Fence()
	schema := addressFence.SchemaID().Content()
	storeID := addressFence.StoreID()
	generation := identity.Generation(root.Revision())
	if !schema.Available() || !storeID.Available() || !generation.Available() || !runtimeFence.Available() {
		return Projection{}, false
	}

	columns := mounted.Columns()
	if len(columns) == 0 {
		return Projection{}, false
	}
	universe, ok := view.Universe()
	if !ok || !universe.Valid() {
		return Projection{}, false
	}
	lineage, ok := mounted.Lineage()
	if !ok || lineage == nil || !lineage.Fence().Same(runtimeFence) {
		return Projection{}, false
	}
	scratch := store.NewReadScratch(view.Manager())
	if scratch == nil || !scratch.Available() {
		return Projection{}, false
	}

	builder := canonical.NewBuilder(schema, storeID, generation)
	declared := make([]Column, 0, len(columns))
	for slot, declaration := range columns {
		if uint64(slot) > uint64(^uint32(0)) {
			return Projection{}, false
		}
		id := declaration.ID()
		typeID := declaration.Type()
		if !id.Available() || !typeID.Available() || declaration.Relation() != id.Relation() {
			return Projection{}, false
		}
		for _, prior := range declared {
			if prior.ID == id {
				return Projection{}, false
			}
		}
		publicationID, ok := deriveColumnID(columnDirectoryTag, schema, id)
		if !ok {
			return Projection{}, false
		}
		denominatorID, ok := deriveColumnID(denominatorTag, schema, id)
		if !ok {
			return Projection{}, false
		}
		axis := canonical.Axis[RowKey, Cell]{SchemaID: schema, Slot: uint32(slot)}
		rows, members, ok := scanColumn(root.Store(), mounted, view, runtimeFence, lineage, id, typeID, universe, scratch)
		if !ok {
			return Projection{}, false
		}
		content := canonical.Content[RowKey, Cell]{Rows: rows, Denominator: denominatorID, Members: members}
		if err := canonical.PutColumn(&builder, axis, content); err != nil {
			return Projection{}, false
		}
		if err := builder.Publish(publicationID, uint32(slot)); err != nil {
			return Projection{}, false
		}
		declared = append(declared, Column{ID: id, Type: typeID, PublicationID: publicationID, DenominatorID: denominatorID, axis: axis})
	}

	sealed, err := builder.Seal()
	if err != nil || !sealed.Published() || len(declared) != sealed.Columns() {
		return Projection{}, false
	}
	projection := Projection{published: sealed, columns: declared, fence: runtimeFence, sealed: true}
	if !projection.Available() {
		return Projection{}, false
	}
	return projection, true
}

func scanColumn(
	root store.Version,
	mounted witness.Mounted,
	view geometry.Geometry,
	fence binding.Fence,
	lineageAuthority interface{ Validate(model.LineageRef) bool },
	id model.ColumnID,
	typeID model.TypeID,
	universe support.Mask,
	scratch *store.ReadScratch,
) (map[RowKey]Cell, []RowKey, bool) {
	if !root.Available() || !mounted.Available() || !view.Available() || !fence.Available() || !id.Available() || !typeID.Available() || !universe.Valid() || scratch == nil || !scratch.Available() || lineageAuthority == nil {
		return nil, nil, false
	}
	rows := make(map[RowKey]Cell)
	members := make([]RowKey, 0)
	completed, valid := root.Scan(id, universe, scratch, func(part store.ReadPart) bool {
		if part.Column() != id || part.Type() != typeID || !part.Region().Valid() || part.Region().Manager() != view.Manager() {
			return false
		}
		presence := part.Presence()
		if !presence.Available() {
			return false
		}
		value := part.Value()
		if value.Available() && (!value.ValidFor(fence) || value.Type() != typeID) {
			return false
		}
		if (presence.Is(model.Present) || presence.Is(model.AuthenticatedOpaque)) != value.Available() {
			return false
		}
		lineage := part.Lineage()
		if !lineage.Available() || !lineageAuthority.Validate(lineage) {
			return false
		}
		scope, scopeOK := view.Normalize(part.Region())
		if !scopeOK || !scope.ValidFor(fence) {
			return false
		}
		row, rowOK := mounted.RowAt(id.Relation(), int(part.Key()))
		if !rowOK || !row.Available() || row.Relation() != id.Relation() {
			return false
		}
		key := RowKey{Relation: id.Relation(), Row: row, Scope: scope}
		if !key.ValidFor(fence) {
			return false
		}
		cell := Cell{Column: id, Type: typeID, Presence: presence, Value: value, Lineage: lineage}
		if !cell.ValidFor(fence) {
			return false
		}
		if _, duplicate := rows[key]; duplicate {
			return false
		}
		rows[key] = cell
		members = append(members, key)
		return true
	})
	if !completed || !valid {
		return nil, nil, false
	}
	return rows, members, true
}

func deriveColumnID(tag string, schema identity.ContentID, column model.ColumnID) (identity.ContentID, bool) {
	if tag == "" || !schema.Available() || !column.Available() || !column.Relation().Available() {
		return identity.ContentID{}, false
	}
	relation := column.Relation()
	schemaBytes := schema
	relationOwner := relation.Owner().Content()
	relationContent := relation.Content()
	columnOwner := column.Owner().Content()
	columnContent := column.Content()
	return identity.DeriveContentID(tag,
		schemaBytes[:],
		relationOwner[:],
		relationContent[:],
		columnOwner[:],
		columnContent[:],
	)
}
