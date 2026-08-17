package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// BodyRow is one immutable BodyPath and its contiguous ordered Outcome range.
// The physical range is private artifact storage, never a semantic identity.
type BodyRow struct {
	id           identity.ContentID
	context      identity.ContentID
	entry        identity.ContentID
	function     identity.ContentID
	formal       identity.ContentID
	callable     bool
	entryPoints  []identity.ContentID
	roots        []RootRow
	outcomeStart uint32
	outcomeEnd   uint32
	sealed       bool
}

// RootRow is one Body-owned sealed executable root descriptor. The semantic
// ID is issued by Program while Flow's semantic-path proof is live; no raw
// Source/Flow Term, authored Root, or Span is retained in the artifact.
type RootRow struct {
	id     identity.ContentID
	family keyspace.Family
}

func (row RootRow) Available() bool {
	return row.id.Available() && row.family != keyspace.FamilyInvalid
}
func (row RootRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row RootRow) Family() keyspace.Family {
	if !row.Available() {
		return keyspace.FamilyInvalid
	}
	return row.family
}

func (row BodyRow) Available() bool {
	return row.sealed && row.id.Available() && row.context.Available() && row.entry.Available() && row.outcomeEnd >= row.outcomeStart &&
		(!row.callable || row.function.Available() && row.formal.Available()) &&
		(row.callable || !row.function.Available() && !row.formal.Available())
}

// EntryID is the exact Program-local semantic identity of this Body's entry
// Site. EntryPointAt exposes its local WTO materializations separately; Link
// and Runtime must not reconstruct this boundary from a Body path.
func (row BodyRow) EntryID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.entry
}

func (row BodyRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

// ContextID is the exact Program Body boundary identity captured during
// compilation.  It is distinct from ID, which is the lexical Body path.
func (row BodyRow) ContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.context
}

// Callable reports whether this Body has the exact transformer Function
// proof required by a closure allocation target.
func (row BodyRow) Callable() bool { return row.Available() && row.callable }

// FunctionContextID and CallFormalID expose the parent-issued IDs needed to
// construct Call target rows.  Non-callable bodies fail closed.
func (row BodyRow) FunctionContextID() (identity.ContentID, bool) {
	return row.function, row.Callable() && row.function.Available()
}
func (row BodyRow) CallFormalID() (identity.ContentID, bool) {
	return row.formal, row.Callable() && row.formal.Available()
}

func (row BodyRow) OutcomeCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.outcomeEnd - row.outcomeStart)
}

// EntryPointCount and EntryPointAt expose the exact existing LocalWTO point
// memberships for this Body's entry Site. They are retained from the sealed
// Program attachment row and are never derived from a Body path.
func (row BodyRow) EntryPointCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.entryPoints)
}
func (row BodyRow) EntryPointAt(index int) (identity.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.entryPoints) {
		return identity.ContentID{}, false
	}
	return row.entryPoints[index], row.entryPoints[index].Available()
}

// RootCount and RootAt expose the exact dense executable-root denominator in
// source order. These are artifact rows, never a runtime Program/Flow query.
func (row BodyRow) RootCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.roots)
}
func (row BodyRow) RootAt(index int) (RootRow, bool) {
	if !row.Available() || index < 0 || index >= len(row.roots) {
		return RootRow{}, false
	}
	root := row.roots[index]
	return root, root.Available()
}
