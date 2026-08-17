package source

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
	"github.com/wippyai/go-lua/internal/framing"
)

// readSourceLiteralRef returns a zero-copy semantic view used only by
// preflight. A String payload points into the immutable Reader input; the
// conversion to a Go string is deferred until the allocation/fill pass.
func readSourceLiteralRef(reader *framing.Reader) (scalar.Ref, error) {
	kind, err := reader.Uint()
	if err != nil {
		return scalar.Ref{}, err
	}
	if kind < uint64(keyspace.LiteralBool) || kind > uint64(keyspace.LiteralString) {
		return scalar.Ref{}, framing.ErrMalformed
	}
	ref := scalar.Ref{Kind: keyspace.LiteralKind(kind)}
	switch ref.Kind {
	case keyspace.LiteralBool:
		ref.Bool, err = reader.Bool()
	case keyspace.LiteralInteger:
		var value uint64
		value, err = reader.Uint()
		ref.Integer = int64(value)
	case keyspace.LiteralFloat:
		ref.FloatBits, err = reader.Uint()
	case keyspace.LiteralString:
		ref.Bytes, err = sourceStringBytes(reader)
	}
	if err != nil {
		return scalar.Ref{}, err
	}
	return ref, nil
}

// preflightSourceKeys validates all three key/fault pools after their exact
// arities have been bounded. Exact atoms are emitted first in canonical dense
// order, so each Key row can carry and validate its nonzero exact ordinal in
// O(1), without a map, sort, replay, or retained lookup state.
func preflightSourceKeys(reader *framing.Reader, counts [keyspace.FamilyCount]uint32, faultOwners []keyspace.Term) error {
	if reader == nil {
		return framing.ErrMalformed
	}
	exactCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	keyCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 8)
	if err != nil {
		return err
	}
	if uint32(keyCount) != counts[keyspace.FamilyKey] {
		return framing.ErrMalformed
	}
	faultCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 8)
	if err != nil {
		return err
	}
	if uint32(faultCount) != counts[keyspace.FamilyControlFault] {
		return framing.ErrMalformed
	}

	exactRows := make([]scalar.Ref, exactCount)
	var previous scalar.Ref
	for index := 0; index < exactCount; index++ {
		value, err := readSourceLiteralRef(reader)
		if err != nil {
			return err
		}
		normalized, ok := scalar.NormalizeRef(value)
		if !ok || !scalar.EqualCanonical(value, normalized) {
			return framing.ErrMalformed
		}
		if index > 0 && scalar.CompareCanonical(previous, value) >= 0 {
			return framing.ErrMalformed
		}
		exactRows[index] = value
		previous = value
	}
	for index := 0; index < keyCount; index++ {
		if _, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false); err != nil {
			return err
		}
		form, err := reader.Uint()
		if err != nil {
			return err
		}
		if form < uint64(keyFormName) || form > uint64(keyFormList) {
			return framing.ErrMalformed
		}
		exactOrdinal, err := reader.Uint()
		if err != nil {
			return err
		}
		if exactOrdinal == 0 || exactOrdinal > uint64(exactCount) {
			return framing.ErrMalformed
		}
		exact := exactRows[exactOrdinal-1]
		if !validSourceKey(keyForm(form), exact.Literal(false)) {
			return framing.ErrMalformed
		}
	}
	if len(faultOwners) != faultCount {
		return framing.ErrMalformed
	}
	for index := 0; index < faultCount; index++ {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		kind, err := reader.Uint()
		if err != nil {
			return err
		}
		if kind < uint64(ControlFaultDuplicateLabel) || kind > uint64(ControlFaultBreakOutsideLoop) {
			return framing.ErrMalformed
		}
		label, err := readBoundTerm(reader, counts, keyspace.FamilyLabel, true)
		if err != nil {
			return err
		}
		blocker, err := readBoundTerm(reader, counts, keyspace.FamilyCell, true)
		if err != nil {
			return err
		}
		fault := ControlFault{Owner: owner, Kind: ControlFaultKind(kind), Label: label, Blocker: blocker}
		if faultOwners[index] == 0 || faultOwners[index] != owner || !validDecodedFault(fault) {
			return framing.ErrMalformed
		}
	}
	return nil
}

func readDirectTerms(reader *framing.Reader, counts [keyspace.FamilyCount]uint32, seen [keyspace.FamilyCount][]bool, faultOwners []keyspace.Term, body keyspace.Term) ([]keyspace.Term, error) {
	count, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 2)
	if err != nil {
		return nil, err
	}
	terms := make([]keyspace.Term, count)
	for index := range terms {
		term, err := readBoundTerm(reader, counts, keyspace.FamilyInvalid, false)
		if err != nil {
			return nil, err
		}
		if !sourceDirectFamily(keyspace.TermFamily(term)) {
			return nil, framing.ErrMalformed
		}
		family := keyspace.TermFamily(term)
		ordinal := keyspace.TermOrdinal(term)
		if seen[family][ordinal-1] {
			return nil, framing.ErrMalformed
		}
		seen[family][ordinal-1] = true
		if family == keyspace.FamilyControlFault {
			if faultOwners[ordinal-1] != 0 {
				return nil, framing.ErrMalformed
			}
			faultOwners[ordinal-1] = body
		}
		terms[index] = term
	}
	return terms, nil
}

func readBoundTerm(reader *framing.Reader, counts [keyspace.FamilyCount]uint32, family keyspace.Family, allowZero bool) (keyspace.Term, error) {
	raw, err := reader.Uint()
	if err != nil {
		return 0, err
	}
	if raw == 0 {
		if allowZero {
			return 0, nil
		}
		return 0, framing.ErrMalformed
	}
	if raw > uint64(^uint32(0)) {
		return 0, framing.ErrMalformed
	}
	term := keyspace.Term(uint32(raw))
	termFamily, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if termFamily == keyspace.FamilyInvalid || ordinal == 0 || ordinal > keyspace.MaxTermOrdinal || keyspace.MakeTerm(termFamily, ordinal) != term {
		return 0, framing.ErrMalformed
	}
	if family != keyspace.FamilyInvalid && termFamily != family {
		return 0, framing.ErrMalformed
	}
	if uint64(ordinal) > uint64(counts[termFamily]) {
		return 0, framing.ErrMalformed
	}
	return term, nil
}

func readSourceKeys(reader *framing.Reader, input *Input, counts [keyspace.FamilyCount]uint32, faultOwners []keyspace.Term) error {
	if input == nil {
		return framing.ErrMalformed
	}
	// The three key-section arities are consecutive, before any row payload.
	// Read and bound all of them first so no untrusted arity controls an
	// allocation while another section count remains unread.
	exactCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 4)
	if err != nil {
		return err
	}
	keyCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 8)
	if err != nil {
		return err
	}
	if uint32(keyCount) != counts[keyspace.FamilyKey] {
		return framing.ErrMalformed
	}
	faultCount, err := sourceCount(reader, uint64(keyspace.MaxTermOrdinal), 8)
	if err != nil {
		return err
	}
	if uint32(faultCount) != counts[keyspace.FamilyControlFault] {
		return framing.ErrMalformed
	}
	input.ExactAtoms = make([]keyspace.LiteralValue, exactCount)
	for index := range input.ExactAtoms {
		value, err := readExactValue(reader)
		if err != nil {
			return err
		}
		normalized, ok := scalar.Normalize(value)
		if !ok || !equalLiteral(value, normalized) {
			return framing.ErrMalformed
		}
		if index > 0 && scalar.CompareCanonical(scalar.FromLiteral(input.ExactAtoms[index-1]), scalar.FromLiteral(value)) >= 0 {
			return framing.ErrMalformed
		}
		input.ExactAtoms[index] = value
	}

	input.Keys = make([]KeyInput, keyCount)
	for index := range input.Keys {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		formRaw, err := reader.Uint()
		if err != nil {
			return err
		}
		if formRaw < uint64(keyFormName) || formRaw > uint64(keyFormList) {
			return framing.ErrMalformed
		}
		exactOrdinal, err := reader.Uint()
		if err != nil {
			return err
		}
		if exactOrdinal == 0 || exactOrdinal > uint64(len(input.ExactAtoms)) {
			return framing.ErrMalformed
		}
		// The exact atom denominator was fully materialized immediately above;
		// the ordinal is the canonical dense Key handle and needs no literal
		// replay or lookup.
		exact := input.ExactAtoms[exactOrdinal-1]
		if !validSourceKey(keyForm(formRaw), exact) {
			return framing.ErrMalformed
		}
		input.Keys[index] = KeyInput{owner: owner, exact: exact, form: keyForm(formRaw)}
	}

	input.Faults = make([]ControlFault, faultCount)
	for index := range input.Faults {
		owner, err := readBoundTerm(reader, counts, keyspace.FamilyBody, false)
		if err != nil {
			return err
		}
		kindRaw, err := reader.Uint()
		if err != nil {
			return err
		}
		if kindRaw < uint64(ControlFaultDuplicateLabel) || kindRaw > uint64(ControlFaultBreakOutsideLoop) {
			return framing.ErrMalformed
		}
		label, err := readBoundTerm(reader, counts, keyspace.FamilyLabel, true)
		if err != nil {
			return err
		}
		blocker, err := readBoundTerm(reader, counts, keyspace.FamilyCell, true)
		if err != nil {
			return err
		}
		fault := ControlFault{Owner: owner, Kind: ControlFaultKind(kindRaw), Label: label, Blocker: blocker}
		if !validDecodedFault(fault) || index >= len(faultOwners) || faultOwners[index] != owner {
			return framing.ErrMalformed
		}
		input.Faults[index] = fault
	}
	return nil
}

func readExactValue(reader *framing.Reader) (keyspace.LiteralValue, error) {
	ref, err := readSourceLiteralRef(reader)
	if err != nil {
		return keyspace.LiteralValue{}, err
	}
	return ref.Literal(true), nil
}

func equalLiteral(left, right keyspace.LiteralValue) bool {
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case keyspace.LiteralBool:
		return left.Bool == right.Bool
	case keyspace.LiteralInteger:
		return left.Integer == right.Integer
	case keyspace.LiteralFloat:
		return left.FloatBits == right.FloatBits
	case keyspace.LiteralString:
		return left.String == right.String
	default:
		return false
	}
}

func validDecodedFault(fault ControlFault) bool {
	if !fault.Kind.valid() {
		return false
	}
	label := fault.Label != 0
	blocker := fault.Blocker != 0
	switch fault.Kind {
	case ControlFaultDuplicateLabel:
		return label && !blocker
	case ControlFaultUndefinedGoto, ControlFaultBreakOutsideLoop:
		return !label && !blocker
	case ControlFaultGotoEntersLocal:
		return label && blocker
	default:
		return false
	}
}
