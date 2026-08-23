package static

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// SummaryResultFamily is Static's canonical summary query family.  The
// coordinate plane is the Value domain's existing denominator; Static only
// publishes the TypeFact it solved at each supplied coordinate.
const SummaryResultFamily schema.Key = "static-type-summary"

// SummaryResultStates is the row-state vocabulary for a written TypeFact
// coordinate. An unwritten coordinate is represented by the plane's absent
// row state, never by a Static lattice value.
const SummaryResultStates = structure.CategoryPublicationRowClass

// SummaryResultColumns are the only Static-owned wire columns. A TypeFact's
// canonical ClassSet identity is the detached value; the TypeFact handle,
// Class structure, and Runtime graph never cross this boundary.
const (
	SummaryColumnClass     = iota
	TypeSummaryColumnClass = SummaryColumnClass
)

func SummaryResultColumns() []plane.Column {
	return []plane.Column{{Key: "class", Carrier: plane.CarrierIdentity}}
}

// EncodeSummaryResult canonically detaches one Static summary onto a sealed
// result layout. coordinateIDs is the already-issued Value coordinate plane in
// dense Value order. Static intentionally accepts those identities as data
// rather than importing Value or minting a parallel coordinate authority.
//
// The wire order is ascending ContentID order, independent of the supplied
// dense order. The small selection walk avoids a scratch permutation slice, so
// encoding allocates only the plane's final payload.
func EncodeSummaryResult(layout *plane.Sealed, observation TypeSummaryObservation, coordinateIDs []identity.ContentID) (present bool, rows uint64, payload []byte, ok bool) {
	classes := observation.owner
	if classes == nil || !typeSummaryObservationOwned(classes, observation) || len(coordinateIDs) != len(observation.Values) || len(coordinateIDs) == 0 || !classes.linkID.Available() {
		return false, 0, nil, false
	}
	if !typeSummaryCoordinateIDsValid(coordinateIDs) {
		return false, 0, nil, false
	}
	writer, begun := plane.Begin(layout, classes.linkID, len(coordinateIDs), 0)
	if !begun {
		return false, 0, nil, false
	}
	var previous identity.ContentID
	for position := 0; position < len(coordinateIDs); position++ {
		dense, found := nextTypeSummaryCoordinate(coordinateIDs, previous, position != 0)
		if !found {
			return false, 0, nil, false
		}
		previous = coordinateIDs[dense]
		id := previous
		if !observation.Present[dense] {
			if !writer.Absent(id) || !writer.EndRow() {
				return false, 0, nil, false
			}
			continue
		}
		classID, classOK := classes.Identity(observation.Values[dense].class)
		if !classOK || !writer.Row(id, structure.PublicationClassHeld) || !writer.Identity(true, classID) || !writer.EndRow() {
			return false, 0, nil, false
		}
	}
	return writer.Finish(uint64(observation.Rows))
}

func typeSummaryCoordinateIDsValid(ids []identity.ContentID) bool {
	for index, id := range ids {
		if !id.Available() {
			return false
		}
		for previous := 0; previous < index; previous++ {
			if ids[previous] == id {
				return false
			}
		}
	}
	return true
}

// nextTypeSummaryCoordinate selects the least Value identity greater than
// previous. It is an allocation-free canonicalization of the supplied dense
// denominator; duplicate identities have already been rejected.
func nextTypeSummaryCoordinate(ids []identity.ContentID, previous identity.ContentID, hasPrevious bool) (int, bool) {
	selected := -1
	for index, id := range ids {
		if hasPrevious && compareTypeSummaryIdentity(id, previous) <= 0 {
			continue
		}
		if selected < 0 || compareTypeSummaryIdentity(id, ids[selected]) < 0 {
			selected = index
		}
	}
	return selected, selected >= 0
}

func compareTypeSummaryIdentity(left, right identity.ContentID) int {
	for index := range left {
		switch {
		case left[index] < right[index]:
			return -1
		case left[index] > right[index]:
			return 1
		}
	}
	return 0
}
