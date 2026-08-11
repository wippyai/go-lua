package evaluation

import (
	"errors"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/containment"
	flowrole "github.com/wippyai/go-lua/program/flow/role"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// SealPorts proves the assembly-local Entry and Finish formulas over one
// complete pre-Outcome Source identity and one committed authored View.  The
// returned proof retains only dense port planes; neither input authority is
// retained.  No successor, route, template, or Outcome relation is emitted.
// Containment is the sole ownership authority; this seal never rebuilds a
// second owner or parent relation.
func SealPorts(
	identity source.Identity,
	view authored.View,
	forest *containment.Result,
	staticID keyspace.ContentID,
	moduleID keyspace.ContentID,
) (*Ports, error) {
	counts, err := sourceCounts(identity)
	if err != nil {
		return nil, err
	}
	if !view.Cold().ContentID().Available() || !staticID.Available() || !moduleID.Available() {
		return nil, errors.New("program/flow/evaluation: authored view is unavailable")
	}
	if forest == nil || !containment.Matches(forest, identity.ContentID(), view.Cold().ContentID(), staticID, moduleID) ||
		forest.Count() != int(identity.TermCount()) {
		return nil, errors.New("program/flow/evaluation: containment proof is unavailable")
	}
	if err := validateAuthoredCounts(view, counts); err != nil {
		return nil, err
	}
	if err := validateRows(view, counts, forest); err != nil {
		return nil, err
	}
	ports := newPorts(counts, identity.ContentID(), view.Cold().ContentID(), staticID, moduleID)
	if err := sealEntries(ports, view, counts); err != nil {
		return nil, err
	}
	if err := sealFinishes(ports, view, counts); err != nil {
		return nil, err
	}
	return ports, nil
}

func sourceCounts(identity source.Identity) ([keyspace.FamilyCount]int, error) {
	var counts [keyspace.FamilyCount]int
	if !identity.ContentID().Available() || identity.Name() == "" || identity.TermCount() == 0 {
		return counts, errors.New("program/flow/evaluation: Source identity is unavailable")
	}
	if identity.FamilyCount(keyspace.FamilyOutcome) != 0 {
		return counts, errors.New("program/flow/evaluation: Outcome is not pre-Outcome")
	}
	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := identity.FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return counts, errors.New("program/flow/evaluation: invalid Source family cardinality")
		}
		counts[family] = count
		total += uint64(count)
	}
	if total != uint64(identity.TermCount()) {
		return counts, errors.New("program/flow/evaluation: Source family cardinality mismatch")
	}
	if counts[keyspace.FamilyBody] == 0 {
		return counts, errors.New("program/flow/evaluation: Source has no Body")
	}
	return counts, nil
}

func authoredCount(view authored.View, family keyspace.Family) int {
	switch family {
	case keyspace.FamilyValues:
		return view.Values().Count()
	case keyspace.FamilyLensExact:
		return view.Access().Exact().Count()
	case keyspace.FamilyLensKey:
		return view.Access().Dynamic().Count()
	case keyspace.FamilyCell:
		return view.Storage().Cells().Count()
	case keyspace.FamilyRead:
		return view.Storage().Reads().Count()
	case keyspace.FamilyVararg:
		return view.Storage().Varargs().Count()
	case keyspace.FamilyBind:
		return view.Storage().Binds().Count()
	case keyspace.FamilyAssign:
		return view.Storage().Assigns().Count()
	case keyspace.FamilyWrite:
		return view.Storage().Writes().Count()
	case keyspace.FamilyTable:
		return view.Tables().Count()
	case keyspace.FamilyTableField:
		return view.Fields().Count()
	case keyspace.FamilyUnary:
		return view.Operators().Unaries().Count()
	case keyspace.FamilyBinary:
		return view.Operators().Binaries().Count()
	case keyspace.FamilySelect:
		return view.Operators().Selects().Count()
	case keyspace.FamilyFunction:
		return view.Functions().Count()
	case keyspace.FamilyCall:
		return view.Calls().Count()
	case keyspace.FamilyReturn:
		return view.Control().Returns().Count()
	case keyspace.FamilyBreak:
		return view.Control().Breaks().Count()
	case keyspace.FamilyLabel:
		return view.Control().Labels().Count()
	case keyspace.FamilyGoto:
		return view.Control().Gotos().Count()
	case keyspace.FamilyBranch:
		return view.Control().Branches().Count()
	case keyspace.FamilyLoop:
		return view.Control().Loops().Count()
	case keyspace.FamilyValueClaim:
		return view.Claims().Count()
	case keyspace.FamilyTypeValue:
		return view.TypeValues().Count()
	default:
		return -1
	}
}

// authoredFamilies is deliberately explicit: adding a new authored family
// must update the denominator gate before it can enter the evaluation proof.
var authoredFamilies = [...]keyspace.Family{
	keyspace.FamilyValues, keyspace.FamilyLensExact, keyspace.FamilyLensKey,
	keyspace.FamilyCell, keyspace.FamilyRead, keyspace.FamilyVararg,
	keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyWrite,
	keyspace.FamilyTable, keyspace.FamilyTableField, keyspace.FamilyUnary,
	keyspace.FamilyBinary, keyspace.FamilySelect, keyspace.FamilyFunction,
	keyspace.FamilyCall, keyspace.FamilyReturn, keyspace.FamilyBreak,
	keyspace.FamilyLabel, keyspace.FamilyGoto, keyspace.FamilyBranch,
	keyspace.FamilyLoop, keyspace.FamilyValueClaim, keyspace.FamilyTypeValue,
}

func validateAuthoredCounts(view authored.View, counts [keyspace.FamilyCount]int) error {
	for _, family := range authoredFamilies {
		want := counts[family]
		got := authoredCount(view, family)
		if got < 0 || got != want {
			return errors.New("program/flow/evaluation: authored family cardinality mismatch")
		}
	}
	return nil
}

func newPorts(counts [keyspace.FamilyCount]int, sourceID, flowID, staticID, moduleID keyspace.ContentID) *Ports {
	ports := &Ports{sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID}
	for _, family := range entryFamilies {
		count := counts[family]
		if count == 0 {
			continue
		}
		ports.entry[family] = make([]keyspace.Term, count+1)
	}
	for _, family := range finishFamilies {
		count := counts[family]
		if count == 0 {
			continue
		}
		ports.finish[family] = make([]keyspace.Term, count+1)
	}
	return ports
}

func validTerm(counts [keyspace.FamilyCount]int, term keyspace.Term) bool {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount &&
		ordinal != 0 && uint64(ordinal) <= uint64(counts[family])
}

func hasFamily(counts [keyspace.FamilyCount]int, term keyspace.Term, family keyspace.Family) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 &&
		uint64(keyspace.TermOrdinal(term)) <= uint64(counts[family])
}

func valueOccurrence(counts [keyspace.FamilyCount]int, term keyspace.Term) bool {
	return validTerm(counts, term) && flowrole.ValueOccurrenceFamily(keyspace.TermFamily(term))
}

func openOccurrence(counts [keyspace.FamilyCount]int, term keyspace.Term) bool {
	return validTerm(counts, term) && flowrole.OpenOccurrenceFamily(keyspace.TermFamily(term))
}
