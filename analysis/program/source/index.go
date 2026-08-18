package source

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func installIndex(a *authority, input IndexInput) error {
	if a == nil || !input.SourceID.Available() || input.SourceID != a.content {
		return errors.New("program/source: Index Source identity disagrees with authored authority")
	}
	if len(input.Bodies) != a.count(keyspace.FamilyBody) ||
		!a.validFamilyTerm(input.Entry, keyspace.FamilyBody) {
		return errors.New("program/source: incomplete Body index")
	}
	var next indexStore
	next.rootRanges = make([]termRange, a.count(keyspace.FamilyBody))
	next.parents = make([]keyspace.Term, a.count(keyspace.FamilyBody))
	if err := installBodyRoots(a, &next, input.Bodies); err != nil {
		return err
	}
	locations, err := buildDirectLocations(a, &next)
	if err != nil {
		return err
	}
	if err := validateBodyForest(a, &next, locations, input.Entry); err != nil {
		return err
	}
	next.entry = input.Entry
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
	if a == nil || a.identity.counts[keyspace.FamilyOutcome] != 0 ||
		len(a.identity.spans[keyspace.FamilyOutcome]) != 0 {
		return errors.New("program/source: Outcome identity was already installed")
	}
	if !keyspace.TermOrdinalFits(len(origins)) {
		return errors.New("program/source: Outcome cardinality overflow")
	}
	spans := make([]storedSpan, len(origins))
	for index, body := range origins {
		if !a.validFamilyTerm(body, keyspace.FamilyBody) {
			return errors.New("program/source: invalid Outcome origin Body")
		}
		spans[index] = a.identity.spans[keyspace.FamilyBody][keyspace.TermOrdinal(body)-1]
	}
	if uint64(a.identity.termCount)+uint64(len(spans)) > uint64(^uint32(0)) {
		return errors.New("program/source: final Term cardinality overflow")
	}
	a.identity.counts[keyspace.FamilyOutcome] = uint32(len(spans))
	a.identity.spans[keyspace.FamilyOutcome] = spans
	a.identity.termCount += uint32(len(spans))
	return nil
}

func installBodyRoots(a *authority, index *indexStore, rows []BodyRoots) error {
	for ordinal, row := range rows {
		if !a.validFamilyTerm(row.Body, keyspace.FamilyBody) || keyspace.TermOrdinal(row.Body) != uint32(ordinal+1) {
			return errors.New("program/source: invalid indexed Body")
		}
		if (row.Parent != 0 && !a.validFamilyTerm(row.Parent, keyspace.FamilyBody)) || row.Parent == row.Body {
			return errors.New("program/source: invalid Body parent")
		}
		start := len(index.rootTerms)
		for _, root := range row.Roots {
			if !a.validTerm(root) {
				return errors.New("program/source: invalid statement root")
			}
			index.rootTerms = append(index.rootTerms, root)
		}
		r, ok := makeRange(start, len(row.Roots))
		if !ok {
			return errors.New("program/source: Body root range overflow")
		}
		index.rootRanges[ordinal] = r
		index.parents[ordinal] = row.Parent
	}
	return nil
}
