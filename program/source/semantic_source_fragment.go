package source

import (
	"errors"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
)

var (
	// ErrSemanticSourceUnavailable means the supplied View is not a published
	// Source capability. A zero View must never be mistaken for an empty
	// semantic fragment.
	ErrSemanticSourceUnavailable = errors.New("program/source: semantic-source fragment is unavailable")
	// ErrSemanticSourceIncomplete means a published Source query did not expose
	// one of the dense rows needed to establish the owner-local fragment.
	ErrSemanticSourceIncomplete = errors.New("program/source: semantic-source fragment is incomplete")
)

const semanticSourceFragmentPublicationCount = 8

// SemanticSourceFragment publishes Source's cold contribution to the
// semantic-source catalog. It is intentionally a fixed owner-local fragment:
// Source exposes no generic catalog walk, and this function emits no relation
// outside the eight Source/Flow definitions it owns here.
//
// ProgramSourceProvenance and ProgramSourceOrder both name the one flattened
// direct-Body source sequence. ProgramFlowBody's primary row counts dense Body
// identities; its roots facet counts the separate sealed Body-root ranges.
// Every required row, including a zero row, is issued exactly once in catalog
// token order.
func buildSemanticSourceFragment(view View) ([]semanticsource.Publication, error) {
	if view.authority == nil || view.authority.identity.name == "" || !view.authority.content.Available() {
		return nil, ErrSemanticSourceUnavailable
	}

	direct, roots, bodyCount, err := sourceSemanticSourceCounts(view)
	if err != nil {
		return nil, err
	}
	literals, err := sourceLiteralCount(view)
	if err != nil {
		return nil, err
	}
	keys, exactKeys, err := sourceKeyCounts(view)
	if err != nil {
		return nil, err
	}
	faults, err := sourceFaultCount(view)
	if err != nil {
		return nil, err
	}

	definitions, err := sourceSemanticSourceDefinitions()
	if err != nil {
		return nil, err
	}
	counts := [...]int{direct, direct, keys, exactKeys, faults, literals, bodyCount, roots}
	if len(definitions) != len(counts) {
		return nil, ErrSemanticSourceIncomplete
	}
	publications := make([]semanticsource.Publication, 0, len(definitions))
	for index, definition := range definitions {
		count := counts[index]
		publication, err := semanticsource.SealPublication(definition, count)
		if err != nil {
			return nil, err
		}
		publications = append(publications, publication)
	}
	return publications, nil
}

func sourceSemanticSourceCounts(view View) (direct, roots, bodyCount int, err error) {
	identity := view.Identity()
	bodyCount = identity.FamilyCount(keyspace.FamilyBody)
	if !keyspace.TermOrdinalFits(bodyCount) {
		return 0, 0, 0, ErrSemanticSourceIncomplete
	}
	// BodyLen and BodyRootLen are intentionally small hot queries and assume
	// their sealed dense ranges are present. Check the range denominators before
	// entering those queries so a malformed/incomplete owner fails closed rather
	// than indexing an absent dense row.
	if len(view.authority.order.bodyRanges) != bodyCount ||
		len(view.authority.index.rootRanges) != bodyCount ||
		len(view.authority.index.parents) != bodyCount {
		return 0, 0, 0, ErrSemanticSourceIncomplete
	}
	order := view.Order()
	index := view.Index()
	for ordinal := uint64(1); ordinal <= uint64(bodyCount); ordinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		bodyLen, ok := order.BodyLen(body)
		if !ok || !addSemanticSourceCount(&direct, bodyLen) {
			return 0, 0, 0, ErrSemanticSourceIncomplete
		}
		for index := 0; index < bodyLen; index++ {
			term, ok := order.BodyAt(body, index)
			if !ok || !view.authority.validDirectBodyTerm(term) {
				return 0, 0, 0, ErrSemanticSourceIncomplete
			}
		}
		rootLen, ok := index.BodyRootLen(body)
		if !ok || !addSemanticSourceCount(&roots, rootLen) {
			return 0, 0, 0, ErrSemanticSourceIncomplete
		}
		for offset := 0; offset < rootLen; offset++ {
			term, ok := index.BodyRootAt(body, offset)
			if !ok || !view.authority.validDirectBodyTerm(term) {
				return 0, 0, 0, ErrSemanticSourceIncomplete
			}
		}
	}
	return direct, roots, bodyCount, nil
}

func sourceLiteralCount(view View) (int, error) {
	literals := view.Literals()
	count := 0
	nils := literals.Nils()
	nilCount := nils.Count()
	if !keyspace.TermOrdinalFits(nilCount) {
		return 0, ErrSemanticSourceIncomplete
	}
	for index := 0; index < nilCount; index++ {
		term, owner, _ := nils.At(index)
		if !validSourceLiteralRow(view.authority, term, owner, keyspace.FamilyNil, index) {
			return 0, ErrSemanticSourceIncomplete
		}
	}
	if _, _, ok := nils.At(nilCount); ok {
		return 0, ErrSemanticSourceIncomplete
	}
	if !addSemanticSourceCount(&count, nilCount) {
		return 0, ErrSemanticSourceIncomplete
	}

	bools := literals.Bools()
	boolCount := bools.Count()
	if !keyspace.TermOrdinalFits(boolCount) {
		return 0, ErrSemanticSourceIncomplete
	}
	for index := 0; index < boolCount; index++ {
		term, owner, _, _ := bools.At(index)
		if !validSourceLiteralRow(view.authority, term, owner, keyspace.FamilyBool, index) {
			return 0, ErrSemanticSourceIncomplete
		}
	}
	if _, _, _, ok := bools.At(boolCount); ok {
		return 0, ErrSemanticSourceIncomplete
	}
	if !addSemanticSourceCount(&count, boolCount) {
		return 0, ErrSemanticSourceIncomplete
	}

	integers := literals.Integers()
	integerCount := integers.Count()
	if !keyspace.TermOrdinalFits(integerCount) {
		return 0, ErrSemanticSourceIncomplete
	}
	for index := 0; index < integerCount; index++ {
		term, owner, _, _ := integers.At(index)
		if !validSourceLiteralRow(view.authority, term, owner, keyspace.FamilyInteger, index) {
			return 0, ErrSemanticSourceIncomplete
		}
	}
	if _, _, _, ok := integers.At(integerCount); ok {
		return 0, ErrSemanticSourceIncomplete
	}
	if !addSemanticSourceCount(&count, integerCount) {
		return 0, ErrSemanticSourceIncomplete
	}

	floats := literals.Floats()
	floatCount := floats.Count()
	if !keyspace.TermOrdinalFits(floatCount) {
		return 0, ErrSemanticSourceIncomplete
	}
	for index := 0; index < floatCount; index++ {
		term, owner, _, _ := floats.At(index)
		if !validSourceLiteralRow(view.authority, term, owner, keyspace.FamilyFloat, index) {
			return 0, ErrSemanticSourceIncomplete
		}
	}
	if _, _, _, ok := floats.At(floatCount); ok {
		return 0, ErrSemanticSourceIncomplete
	}
	if !addSemanticSourceCount(&count, floatCount) {
		return 0, ErrSemanticSourceIncomplete
	}

	strings := literals.Strings()
	stringCount := strings.Count()
	if !keyspace.TermOrdinalFits(stringCount) {
		return 0, ErrSemanticSourceIncomplete
	}
	for index := 0; index < stringCount; index++ {
		term, owner, _, _ := strings.At(index)
		if !validSourceLiteralRow(view.authority, term, owner, keyspace.FamilyString, index) {
			return 0, ErrSemanticSourceIncomplete
		}
	}
	if _, _, _, ok := strings.At(stringCount); ok {
		return 0, ErrSemanticSourceIncomplete
	}
	if !addSemanticSourceCount(&count, stringCount) {
		return 0, ErrSemanticSourceIncomplete
	}
	return count, nil
}

func validSourceLiteralRow(authority *authority, term, owner keyspace.Term, family keyspace.Family, index int) bool {
	return authority != nil && term == keyspace.MakeTerm(family, uint32(index+1)) &&
		authority.validFamilyTerm(term, family) && authority.validFamilyTerm(owner, keyspace.FamilyBody)
}

func sourceKeyCounts(view View) (keys, exactKeys int, err error) {
	keyView := view.Keys()
	keys = keyView.Count()
	exactKeys = keyView.ExactCount()
	if !keyspace.TermOrdinalFits(keys) || !keyspace.TermOrdinalFits(exactKeys) {
		return 0, 0, ErrSemanticSourceIncomplete
	}
	for index := 0; index < exactKeys; index++ {
		key, value, ok := keyView.ExactAt(index)
		if !ok || key != keyspace.Key(index+1) || key == 0 {
			return 0, 0, ErrSemanticSourceIncomplete
		}
		normalized, valid := NormalizeExactKey(value)
		if !valid || normalized != value {
			return 0, 0, ErrSemanticSourceIncomplete
		}
		resolved, valid := keyView.Exact(key)
		if !valid || resolved != value {
			return 0, 0, ErrSemanticSourceIncomplete
		}
	}
	if _, _, ok := keyView.ExactAt(exactKeys); ok {
		return 0, 0, ErrSemanticSourceIncomplete
	}
	for index := 0; index < keys; index++ {
		term := keyspace.MakeTerm(keyspace.FamilyKey, uint32(index+1))
		nameOwner, name, nameKey, nameOK := keyView.Name(term)
		listOwner, listOrdinal, listKey, listOK := keyView.List(term)
		if nameOK == listOK || !keyViewTerm(view.authority, term, keyspace.FamilyKey, index) {
			return 0, 0, ErrSemanticSourceIncomplete
		}
		var owner keyspace.Term
		var key keyspace.Key
		if nameOK {
			if nameOwner == 0 || nameKey == 0 || listOwner != 0 || listOrdinal != 0 || listKey != 0 {
				return 0, 0, ErrSemanticSourceIncomplete
			}
			owner, key = nameOwner, nameKey
			value, valid := keyView.Exact(key)
			if !valid || value.Kind != keyspace.LiteralString || value.String != name {
				return 0, 0, ErrSemanticSourceIncomplete
			}
		} else {
			if listOwner == 0 || listOrdinal <= 0 || listKey == 0 || nameOwner != 0 || name != "" || nameKey != 0 {
				return 0, 0, ErrSemanticSourceIncomplete
			}
			owner, key = listOwner, listKey
			value, valid := keyView.Exact(key)
			if !valid || value.Kind != keyspace.LiteralInteger || value.Integer != listOrdinal {
				return 0, 0, ErrSemanticSourceIncomplete
			}
		}
		if !view.authority.validFamilyTerm(owner, keyspace.FamilyBody) || uint64(key) > uint64(exactKeys) {
			return 0, 0, ErrSemanticSourceIncomplete
		}
	}
	return keys, exactKeys, nil
}

func keyViewTerm(authority *authority, term keyspace.Term, family keyspace.Family, index int) bool {
	return authority != nil && term == keyspace.MakeTerm(family, uint32(index+1)) && authority.validFamilyTerm(term, family)
}

func sourceFaultCount(view View) (int, error) {
	faults := view.Faults()
	count := faults.Count()
	if !keyspace.TermOrdinalFits(count) {
		return 0, ErrSemanticSourceIncomplete
	}
	for index := 0; index < count; index++ {
		term := keyspace.MakeTerm(keyspace.FamilyControlFault, uint32(index+1))
		fault, ok := faults.At(term)
		if !ok || !keyViewTerm(view.authority, term, keyspace.FamilyControlFault, index) || !validControlFault(view.authority, fault) {
			return 0, ErrSemanticSourceIncomplete
		}
	}
	if _, ok := faults.At(keyspace.MakeTerm(keyspace.FamilyControlFault, uint32(count+1))); ok {
		return 0, ErrSemanticSourceIncomplete
	}
	return count, nil
}

func addSemanticSourceCount(total *int, value int) bool {
	if total == nil || value < 0 || *total < 0 || value > maxSemanticSourceInt()-*total {
		return false
	}
	*total += value
	return true
}

func maxSemanticSourceInt() int { return int(^uint(0) >> 1) }
