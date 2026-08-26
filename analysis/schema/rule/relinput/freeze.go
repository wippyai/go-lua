package relinput

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Ordinal is the position of one row inside its frozen column. Each column is
// a dense sequence emitted in one order, and that order is the contract: rule
// rows are emitted by rule ordinal, port rows by the span order the rule rows
// name, and region rows in first-named order.
type Ordinal uint32

// freezeDomain fences the frozen bundle's identities against every other
// column universe. A frozen bundle is addressed by the rule catalog it was
// sealed for and the authority that issued its scopes, so a bundle sealed by
// another owner over the same catalog occupies a different schema.
const freezeDomain = "analysis/relation-input/v1"

const (
	ruleSlot   uint32 = 0
	portSlot   uint32 = 1
	regionSlot uint32 = 2
)

// SchemaID is the identity a bundle's frozen columns are sealed under. It is
// derived from the rule catalog and the issuing owner, so a reader that names
// either differently recovers no column rather than a foreign one.
func SchemaID(catalog identity.ContentID, owner model.OwnerID) (identity.ContentID, bool) {
	if !catalog.Available() || !owner.Available() {
		return identity.ContentID{}, false
	}
	content := owner.Content()
	return identity.DeriveContentID(freezeDomain, catalog[:], content[:])
}

func denominatorID(schemaID identity.ContentID, column string) (identity.ContentID, bool) {
	if !schemaID.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID(freezeDomain+"/"+column, schemaID[:])
}

func ruleAxis(schemaID identity.ContentID) snapshot.Axis[Ordinal, Row] {
	return snapshot.Axis[Ordinal, Row]{SchemaID: schemaID, Slot: ruleSlot}
}

func portAxis(schemaID identity.ContentID) snapshot.Axis[Ordinal, PortRow] {
	return snapshot.Axis[Ordinal, PortRow]{SchemaID: schemaID, Slot: portSlot}
}

func regionAxis(schemaID identity.ContentID) snapshot.Axis[Ordinal, RegionRow] {
	return snapshot.Axis[Ordinal, RegionRow]{SchemaID: schemaID, Slot: regionSlot}
}

// sealable is what every frozen row answers: whether it names a statement. A
// column never seals a row that states nothing, so a reader never recovers
// one.
type sealable interface {
	Available() bool
}

func putColumn[V sealable](builder *snapshot.FrozenBuilder, axis snapshot.Axis[Ordinal, V], rows []V, column string) bool {
	denominator, derived := denominatorID(axis.SchemaID, column)
	if !derived {
		return false
	}
	for _, row := range rows {
		if !row.Available() {
			return false
		}
	}
	if rows == nil {
		rows = []V{}
	}
	content := snapshot.Content[Ordinal, V]{Sequence: rows, Denominator: denominator}
	return snapshot.PutFrozenColumn(builder, axis, content) == nil
}

// Freeze seals the bundle's three columns into one immutable publication. The
// bundle keeps its own storage: freezing copies the rows and returns a
// publication that can be read without the sealing composition.
func (bundle *Bundle) Freeze(store identity.StoreID) (snapshot.Frozen, bool) {
	if !bundle.Available() || !store.Available() {
		return snapshot.Frozen{}, false
	}
	schemaID, derived := SchemaID(bundle.catalog, bundle.owner)
	if !derived {
		return snapshot.Frozen{}, false
	}
	builder := snapshot.NewFrozen(schemaID, store)
	if !putColumn(&builder, ruleAxis(schemaID), bundle.rows, "rule") ||
		!putColumn(&builder, portAxis(schemaID), bundle.ports, "port") ||
		!putColumn(&builder, regionAxis(schemaID), bundle.regions, "region") {
		return snapshot.Frozen{}, false
	}
	frozen, err := builder.Seal()
	if err != nil {
		return snapshot.Frozen{}, false
	}
	return frozen, true
}

// View reads one frozen bundle. It borrows the sealed publication and holds
// no copied column: the answers a View gives are the answers the owner sealed.
type View struct {
	frozen   *snapshot.Frozen
	schemaID identity.ContentID
	catalog  identity.ContentID
	owner    model.OwnerID
}

// Open binds a frozen publication to the rule catalog and issuing owner it
// was sealed for. A reader that names either differently opens nothing.
func Open(frozen *snapshot.Frozen, catalog identity.ContentID, owner model.OwnerID) (View, bool) {
	schemaID, derived := SchemaID(catalog, owner)
	if frozen == nil || !derived {
		return View{}, false
	}
	view := View{frozen: frozen, schemaID: schemaID, catalog: catalog, owner: owner}
	if _, published := view.count(); !published {
		return View{}, false
	}
	return view, true
}

// Available reports whether the view names a sealed bundle publication.
func (view View) Available() bool {
	return view.frozen != nil && view.schemaID.Available() && view.catalog.Available() && view.owner.Available()
}

// Catalog is the rule-catalog digest this view's bundle was sealed for.
func (view View) Catalog() identity.ContentID {
	if !view.Available() {
		return identity.ContentID{}
	}
	return view.catalog
}

// Owner is the authority that issued every scope the view answers with.
func (view View) Owner() model.OwnerID {
	if !view.Available() {
		return model.OwnerID{}
	}
	return view.owner
}

func (view View) columnCount(column string) (int, bool) {
	if !view.Available() {
		return 0, false
	}
	denominator, derived := denominatorID(view.schemaID, column)
	if !derived {
		return 0, false
	}
	return view.frozen.Denominators().Size(denominator)
}

func columnAt[V any](view View, axis snapshot.Axis[Ordinal, V], index int) (V, bool) {
	var absent V
	if !view.Available() || index < 0 {
		return absent, false
	}
	row, status := snapshot.ReadFrozen(view.frozen, axis, Ordinal(index))
	if status != snapshot.ReadHit {
		return absent, false
	}
	return row, true
}

func (view View) count() (int, bool) {
	return view.columnCount("rule")
}

// Count is the number of rule ordinals the frozen table covers.
func (view View) Count() int {
	count, published := view.count()
	if !published {
		return 0
	}
	return count
}

// At returns one rule ordinal's frozen row.
func (view View) At(ordinal int) (Row, bool) {
	return columnAt(view, ruleAxis(view.schemaID), ordinal)
}

// CandidateScope returns the decision scope one rule ordinal's candidate rows
// are decided at.
func (view View) CandidateScope(ordinal int) (model.ScopeID, bool) {
	row, held := view.At(ordinal)
	if !held || !row.Present() {
		return model.ScopeID{}, false
	}
	return row.Candidate(), true
}

// PortCount is the declared input width of one frozen rule ordinal.
func (view View) PortCount(ordinal int) (int, bool) {
	row, held := view.At(ordinal)
	if !held {
		return 0, false
	}
	return row.PortCount(), true
}

// PortScope returns the decision scope one declared input port observes, in
// the rule's own port order.
func (view View) PortScope(ordinal, port int) (model.ScopeID, bool) {
	row, held := view.At(ordinal)
	if !held || !row.Present() || port < 0 || port >= int(row.portCount) {
		return model.ScopeID{}, false
	}
	frozen, read := columnAt(view, portAxis(view.schemaID), int(row.portOffset)+port)
	if !read {
		return model.ScopeID{}, false
	}
	return frozen.Scope(), true
}

// RegionCount is the number of distinct scopes the frozen table carries
// evidence for.
func (view View) RegionCount() int {
	count, published := view.columnCount("region")
	if !published {
		return 0
	}
	return count
}

// RegionAt returns one frozen scope-evidence row in first-named order.
func (view View) RegionAt(index int) (RegionRow, bool) {
	return columnAt(view, regionAxis(view.schemaID), index)
}

// ScopeRegion returns the owner-issued region evidence one named scope stands
// on. A scope the bundle never named has no evidence.
func (view View) ScopeRegion(scope model.ScopeID) (identity.ContentID, bool) {
	if !view.Available() || !scope.Available() {
		return identity.ContentID{}, false
	}
	for index := 0; index < view.RegionCount(); index++ {
		region, held := view.RegionAt(index)
		if !held {
			return identity.ContentID{}, false
		}
		if region.Scope() == scope {
			return region.Evidence(), true
		}
	}
	return identity.ContentID{}, false
}
