package program

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func programSourceCounts(view source.View) ([8]int, error) {
	var counts [8]int
	if !view.Identity().ContentID().Available() || view.Identity().Name() == "" {
		return counts, errors.New("unavailable Source view")
	}
	bodyCount := view.Identity().FamilyCount(keyspace.FamilyBody)
	if !programSemanticSourceCountFits(bodyCount) {
		return counts, errors.New("invalid Source body cardinality")
	}
	direct, roots := 0, 0
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		bodyLen, ok := view.Order().BodyLen(body)
		if !ok || !addProgramSemanticSourceMeasure(&direct, bodyLen) {
			return counts, errors.New("invalid Source body order column")
		}
		rootLen, ok := view.Index().BodyRootLen(body)
		if !ok || !addProgramSemanticSourceMeasure(&roots, rootLen) {
			return counts, errors.New("invalid Source body-root column")
		}
	}
	literals := view.Literals()
	literalCount := literals.Nils().Count() + literals.Bools().Count() + literals.Integers().Count() + literals.Floats().Count() + literals.Strings().Count()
	keys, exactKeys := view.Keys().Count(), view.Keys().ExactCount()
	faults := view.Faults().Count()
	if !programSemanticSourceCountFits(literalCount) || !programSemanticSourceCountFits(keys) || !programSemanticSourceCountFits(exactKeys) || !programSemanticSourceCountFits(faults) {
		return counts, errors.New("invalid Source cardinality")
	}
	return [...]int{direct, direct, keys, exactKeys, faults, literalCount, bodyCount, roots}, nil
}
