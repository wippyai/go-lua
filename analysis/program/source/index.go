package source

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	sealedindex "github.com/wippyai/go-lua/analysis/program/source/index"
)

func installIndex(a *authority, input IndexInput) error {
	if a == nil || !input.SourceID.Available() || input.SourceID != a.content {
		return errors.New("program/source: Index Source identity disagrees with authored authority")
	}
	var next *sealedindex.Table
	locations, err := buildDirectLocations(a, input.Positions)
	if err != nil {
		return err
	}
	if err := installOutcomeIdentity(a, input.OutcomeOrigins); err != nil {
		return err
	}
	if err := installPositions(a, &next, locations, input); err != nil {
		return err
	}
	a.index = next
	return nil
}

// installOutcomeIdentity installs the sole derived Term family from Flow's
// canonical ordered origin Bodies. Source does not know Outcome semantics or
// mint an order: it validates the typed Body foreign keys, copies each owning
// Body's authored coordinate, and assigns the dense Outcome ordinal implied by
// the supplied order. The resulting count/spans participate in final identity
// and position validation, but remain absent from authored ContentID.
func installOutcomeIdentity(a *authority, origins []keyspace.Term) error {
	if a == nil || a.identity.familyCount(keyspace.FamilyOutcome) != 0 ||
		len(a.identity.spans[keyspace.FamilyOutcome]) != 0 || len(a.identity.outcomeOrigins) != 0 {
		return errors.New("program/source: Outcome identity was already installed")
	}
	if !keyspace.TermOrdinalFits(len(origins)) {
		return errors.New("program/source: Outcome cardinality overflow")
	}
	for _, body := range origins {
		if !a.validFamilyTerm(body, keyspace.FamilyBody) {
			return errors.New("program/source: invalid Outcome origin Body")
		}
	}
	termCount, ok := a.identity.termCount()
	if !ok || uint64(termCount)+uint64(len(origins)) > uint64(^uint32(0)) {
		return errors.New("program/source: final Term cardinality overflow")
	}
	a.identity.outcomeOrigins = append([]keyspace.Term(nil), origins...)
	return nil
}
