// Package query owns the composed immutable query surface of one authored
// Static publication. The enclosing static owner supplies already-sealed
// canonical tables and a lifecycle cell; this package never reaches back into
// the owner's Component or construction callbacks.
package query

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

// Proof is the immutable local Static containment relation retained after the
// root owner has validated the combined forest. NewProof transfers ownership
// of the supplied slices; callers must not mutate them after the transfer.
// The slices are never returned, so consumers can only observe the typed
// Parent and FieldOwner queries below.
type Proof struct {
	parents     [keyspace.FamilyCount][]keyspace.Term
	fieldOwners []keyspace.Term
	available   bool
}

// NewProof transfers the validated local containment rows into the query
// vertical. It intentionally does not build an index or copy the canonical
// relation image.
func NewProof(parents [keyspace.FamilyCount][]keyspace.Term, fieldOwners []keyspace.Term) *Proof {
	return &Proof{parents: parents, fieldOwners: fieldOwners, available: true}
}

func (proof *Proof) availableProof() bool { return proof != nil && proof.available }

// Snapshot is the immutable canonical input consumed by Static queries. It is
// a shallow value of the sealed child tables; those tables own their immutable
// row storage. No root Component or query-derived index is retained here.
type Snapshot struct {
	contentID    identity.ContentID
	census       [keyspace.FamilyCount]uint32
	types        statictypes.Table
	references   staticrefs.Table
	declarations staticdecl.Table
	signatures   staticsig.Table
	contracts    staticcontracts.Table
	operators    staticoperators.Table
	operands     staticoperands.Table
	publications staticpubs.Table
	proof        *Proof
}

// NewSnapshot accepts the sealed canonical values from Static's composition
// owner. The values are copied as immutable table headers only; row storage,
// order, and identity remain the owners' canonical representations.
func NewSnapshot(
	contentID identity.ContentID,
	census [keyspace.FamilyCount]uint32,
	types statictypes.Table,
	references staticrefs.Table,
	declarations staticdecl.Table,
	signatures staticsig.Table,
	contracts staticcontracts.Table,
	operators staticoperators.Table,
	operands staticoperands.Table,
	publications staticpubs.Table,
	proof *Proof,
) Snapshot {
	return Snapshot{
		contentID:    contentID,
		census:       census,
		types:        types,
		references:   references,
		declarations: declarations,
		signatures:   signatures,
		contracts:    contracts,
		operators:    operators,
		operands:     operands,
		publications: publications,
		proof:        proof,
	}
}

// ContentID returns the authored identity carried by this canonical snapshot.
func (snapshot Snapshot) ContentID() identity.ContentID { return snapshot.contentID }

// Census returns the sealed family cardinality column. It is the only
// cardinality authority used by the composed Static queries.
func (snapshot Snapshot) Census() [keyspace.FamilyCount]uint32 { return snapshot.census }

// Tables exposes the sealed child tables to Static's composition-owned codec
// and denominator code. The returned values are immutable table headers.
func (snapshot Snapshot) Tables() (
	statictypes.Table,
	staticrefs.Table,
	staticdecl.Table,
	staticsig.Table,
	staticcontracts.Table,
	staticoperators.Table,
	staticoperands.Table,
	staticpubs.Table,
) {
	return snapshot.types, snapshot.references, snapshot.declarations,
		snapshot.signatures, snapshot.contracts, snapshot.operators,
		snapshot.operands, snapshot.publications
}

// View returns a permanently available query view over the snapshot.
func (snapshot Snapshot) View() View { return View{snapshot: snapshot} }

// View is the immutable composed Static query surface. A non-nil live cell
// binds it to an active construction transaction; a nil cell denotes a
// published snapshot that remains available forever.
type View struct {
	snapshot Snapshot
	live     *uint32
}

// NewView creates a view over immutable canonical values. The owner supplies
// live as nil for a published snapshot or as its one-shot lifecycle cell for a
// construction view.
func NewView(snapshot Snapshot, live *uint32) View {
	return View{snapshot: snapshot, live: live}
}

func (view View) available() bool {
	return view.snapshot.contentID.Available() &&
		(view.live == nil || atomic.LoadUint32(view.live) != 0)
}

// Available reports whether this view can currently observe its snapshot.
func (view View) Available() bool { return view.available() }

// Snapshot returns the immutable canonical values when this view is live.
// The second result is false for a zero or expired construction view.
func (view View) Snapshot() (Snapshot, bool) {
	if !view.available() {
		return Snapshot{}, false
	}
	return view.snapshot, true
}
