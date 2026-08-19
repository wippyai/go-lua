package cold

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Ordinal is the position of one row inside its cold family. A cold family is
// a dense sequence the compiler emitted in one order, and that order is part
// of what the artifact's content identity commits to, so the coordinate is
// the position and not a derived identity.
type Ordinal uint32

// The dense slot each cold family occupies in a compiled program's
// publication. Slots are append-only: a family added later takes the next
// slot, and no family ever moves, because a slot is half of the address every
// consumer holds.
const slotCallTarget uint32 = 0

// CallTarget is the exact closure-allocation-to-callable-body proof: which
// allocation is called, which body it enters, and the context and formal that
// body was compiled with.
//
// Every field is an artifact-local identity. None of them is mount-qualified,
// which is why one compiled program's call targets are shared unchanged by
// every Link that mounts it, and why a consumer that needs a mount-qualified
// address derives it at the read site from the module key it already holds.
type CallTarget struct {
	Allocation identity.ContentID
	Body       identity.ContentID
	Context    identity.ContentID
	Function   identity.ContentID
	Formal     identity.ContentID
}

// Available reports whether row names a proof. A target that is missing any
// of its five identities proves nothing, so it is never a row a consumer can
// read as one.
func (row CallTarget) Available() bool {
	return row.Allocation.Available() && row.Body.Available() && row.Context.Available() &&
		row.Function.Available() && row.Formal.Available()
}

// CallTargets is the address of the call-target column of the cold catalog.
func CallTargets(catalog identity.ContentID) snapshot.Axis[Ordinal, CallTarget] {
	return snapshot.Axis[Ordinal, CallTarget]{SchemaID: catalog, Slot: slotCallTarget}
}

// CallTargetDenominator is the identity of the call-target family's key
// universe within one cold catalog. The universe is the family's own ordinal
// range, so its identity is derived from the catalog and the family alone.
func CallTargetDenominator(catalog identity.ContentID) (identity.ContentID, bool) {
	if !catalog.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID(catalogDomain+"/call-target", catalog[:])
}

// CallTargetContent seals a compiled family into the column's payload. The
// rows are the emitted sequence and the denominator's membership is that
// sequence's ordinal range, so the column is total over exactly what it
// publishes and an ordinal past the end is a proven absence rather than a
// missing row.
//
// A family with an unavailable row seals nothing: a compiled program either
// proved every target it emitted or it did not compile.
func CallTargetContent(rows []CallTarget, catalog identity.ContentID) (snapshot.Content[Ordinal, CallTarget], bool) {
	denominator, derived := CallTargetDenominator(catalog)
	if !derived {
		return snapshot.Content[Ordinal, CallTarget]{}, false
	}
	sealed := make(map[Ordinal]CallTarget, len(rows))
	members := make([]Ordinal, 0, len(rows))
	for index, row := range rows {
		if !row.Available() {
			return snapshot.Content[Ordinal, CallTarget]{}, false
		}
		ordinal := Ordinal(index)
		sealed[ordinal] = row
		members = append(members, ordinal)
	}
	return snapshot.Content[Ordinal, CallTarget]{
		Rows: sealed, Denominator: denominator, Members: members,
	}, true
}

// CallTargetCount is the sealed width of the call-target family: the
// cardinality of the key universe the column is total over. A catalog the
// publication does not hold reports nothing rather than an empty family.
func CallTargetCount(frozen *snapshot.Frozen, catalog identity.ContentID) (int, bool) {
	denominator, derived := CallTargetDenominator(catalog)
	if !derived || frozen == nil {
		return 0, false
	}
	return frozen.Denominators().Size(denominator)
}

// CallTargetAt returns one proof by its position in the emitted sequence. An
// ordinal outside the sealed family, and a publication that holds no
// call-target column at all, both report nothing.
func CallTargetAt(frozen *snapshot.Frozen, catalog identity.ContentID, index int) (CallTarget, bool) {
	if index < 0 {
		return CallTarget{}, false
	}
	row, status := snapshot.ReadFrozen(frozen, CallTargets(catalog), Ordinal(index))
	return row, status == snapshot.ReadHit
}
