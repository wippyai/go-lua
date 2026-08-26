package targetfixture

import (
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
)

// Population declares the complete mounted row membership for one logical
// denominator. Families choose the relation/key and typed RowIDs themselves.
type Population struct {
	Denominator model.DenominatorRef
	Rows        []model.RowID
}

// Scope binds one declared logical scope to a test-only neutral region label.
// Labels are geometry only: they are never interpreted as domain semantics.
type Scope struct {
	ID     model.ScopeID
	Region string
}

// Cell is one typed initial publication candidate. Its token must be encoded
// by the owning family codec under the supplied runtime issuer.
type Cell struct {
	Denominator model.DenominatorRef
	Row         model.RowID
	Column      model.ColumnID
	Value       binding.ValueToken
	Presence    model.Presence
}

// Present constructs a normal typed initial value cell.
func Present(denominator model.DenominatorRef, row model.RowID, column model.ColumnID, value binding.ValueToken) (Cell, bool) {
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		return Cell{}, false
	}
	return Cell{Denominator: denominator, Row: row, Column: column, Value: value, Presence: presence}, true
}

// Opaque constructs an authenticated opaque initial value cell. The caller
// still supplies the exact typed token; this helper only states the declared
// presence contract and never treats opacity as a domain value.
func Opaque(denominator model.DenominatorRef, row model.RowID, column model.ColumnID, value binding.ValueToken) (Cell, bool) {
	presence, ok := model.NewPresence(model.AuthenticatedOpaque)
	if !ok {
		return Cell{}, false
	}
	return Cell{Denominator: denominator, Row: row, Column: column, Value: value, Presence: presence}, true
}

// Initial declares a zero-input seed operation. The family supplies the
// sealed signature and the typed row values; the kit issues cells, applies,
// and publishes them through the target runtime's sole state path.
type Initial struct {
	Operation signature.Signature
	Scope     model.ScopeID
	Cells     func(binding.Issuer) ([]Cell, bool)
}

// Authorities is the owner-supplied typed algebra/equality registry. It is
// invoked only after the target mount has issued its runtime fence.
type Authorities func(binding.Issuer) (Registry, bool)

// Registry is a small heterogeneous registry assembled from family-owned
// typed codecs. The kit never infers equality, algebra, or domain values.
type Registry struct {
	Algebras   []binding.ValueAlgebra
	Equalities []binding.ValueEquality
}

// Spec is the complete family-owned declaration handoff to the generic target
// mount. Declaration must contain relations, columns, keys, scopes, typed
// capabilities, and family rules; Build adds only the Initial declarations
// described above before compiling it.
type Spec struct {
	Identity    Identity
	Declaration relcompile.Declaration
	Bindings    []binding.Factory
	Populations []Population
	Scopes      []Scope
	Initials    []Initial
	Authorities Authorities
	// ResolveExpand supplies the owner-issued C→P/key vectors for any logical
	// Expand in Declaration. It is cold mount input only: Build passes it to
	// the inventory so witness.Specialize freezes the vectors into its
	// immutable evidence catalog; runtime never retains or calls it.
	ResolveExpand func(model.ExpandContract) ([]expand.Vector, bool)
	// PartitionInventory is the owner-issued correlated-Apply source.  It is
	// optional for declarations without CorrelationPartitions; when a checked
	// declaration contains one, Build forwards this exact source to the mount
	// inventory and refuses if it is absent.  The fixture does not derive
	// postings from Populations or retain a second row map.
	PartitionInventory witness.PartitionInventory
	MountByte          byte
}
