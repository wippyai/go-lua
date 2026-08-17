package candidates

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal derives the canonical typed candidate buckets from one source identity,
// authored rows, and the executable proof. Every relevant source relation is
// visited in its already-canonical ordinal order, so the pass needs no
// sorting, recursion, maps, or retained authority pointers.
func Seal(
	identity source.Identity,
	view authored.View,
	proof *executable.Result,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (*Result, error) {
	counts, err := validateInputs(identity, view, proof, staticID, moduleID)
	if err != nil {
		return nil, err
	}

	operators := view.Operators()
	unaries := operators.Unaries()
	binaries := operators.Binaries()
	selects := operators.Selects()
	reads := view.Storage().Reads()
	writes := view.Storage().Writes()
	values := view.Values()
	loops := view.Control().Loops()

	result := &Result{
		sourceID: identity.ContentID(),
		flowID:   view.Cold().ContentID(),
		staticID: staticID,
		moduleID: moduleID,
		// Natural append keeps retained bucket capacity proportional to the
		// actual executable members rather than reserving every mutually
		// exclusive bucket at the authored denominator size.
		classes: classStore{
			unaryClass:  make([]uint8, counts[keyspace.FamilyUnary]),
			binaryClass: make([]uint8, counts[keyspace.FamilyBinary]),
			readClass:   make([]uint8, counts[keyspace.FamilyRead]),
			writeClass:  make([]uint8, counts[keyspace.FamilyWrite]),
			loopClass:   make([]uint8, counts[keyspace.FamilyLoop]),
		},
	}

	for index := 0; index < unaries.Count(); index++ {
		term, ok := unaries.At(index)
		if !ok {
			return nil, errors.New("program/flow/candidates: Unary view is not live")
		}
		_, op, _, ok := unaries.Get(term)
		if !ok {
			return nil, errors.New("program/flow/candidates: Unary row is not live")
		}
		class, err := classifyUnary(op)
		if err != nil {
			return nil, err
		}
		if proof.Executable(term) && class != unaryNoCandidate {
			result.classes.unaryClass[index] = class
			switch class {
			case unaryNumericCandidate:
				result.buckets.unaryNumeric = append(result.buckets.unaryNumeric, term)
			case unaryLengthCandidate:
				result.buckets.length = append(result.buckets.length, term)
			}
		}
	}

	for index := 0; index < binaries.Count(); index++ {
		term, ok := binaries.At(index)
		if !ok {
			return nil, errors.New("program/flow/candidates: Binary view is not live")
		}
		_, op, _, _, ok := binaries.Get(term)
		if !ok {
			return nil, errors.New("program/flow/candidates: Binary row is not live")
		}
		class, err := classifyBinary(op)
		if err != nil {
			return nil, err
		}
		if !proof.Executable(term) || class == binaryNoCandidate {
			continue
		}
		result.classes.binaryClass[index] = class
		switch class {
		case binaryArithmeticCandidate:
			result.buckets.arithmetic = append(result.buckets.arithmetic, term)
		case binaryBitwiseCandidate:
			result.buckets.bitwise = append(result.buckets.bitwise, term)
		case binaryConcatCandidate:
			result.buckets.concat = append(result.buckets.concat, term)
		case binaryEqualityCandidate:
			result.buckets.equality = append(result.buckets.equality, term)
		case binaryOrderCandidate:
			result.buckets.order = append(result.buckets.order, term)
		}
	}

	// Select has no bucket, but its closed authored vocabulary is still
	// checked explicitly so a future or malformed enum cannot disappear.
	for index := 0; index < selects.Count(); index++ {
		term, ok := selects.At(index)
		if !ok {
			return nil, errors.New("program/flow/candidates: Select view is not live")
		}
		_, op, _, _, ok := selects.Get(term)
		if !ok {
			return nil, errors.New("program/flow/candidates: Select row is not live")
		}
		if err := classifySelect(op); err != nil {
			return nil, err
		}
	}

	for index := 0; index < reads.Count(); index++ {
		term, ok := reads.At(index)
		if !ok {
			return nil, errors.New("program/flow/candidates: Read view is not live")
		}
		_, sourceTerm, _, ok := reads.Get(term)
		if !ok {
			return nil, errors.New("program/flow/candidates: Read row is not live")
		}
		family := keyspace.TermFamily(sourceTerm)
		if family != keyspace.FamilyCell && !lensFamily(family) {
			return nil, errors.New("program/flow/candidates: invalid Read source family")
		}
		candidate := lensFamily(family)
		if proof.Executable(term) && candidate {
			result.classes.readClass[index] = accessIndexCandidate
			result.buckets.indexGet = append(result.buckets.indexGet, term)
		}
	}

	for index := 0; index < writes.Count(); index++ {
		term, ok := writes.At(index)
		if !ok {
			return nil, errors.New("program/flow/candidates: Write view is not live")
		}
		_, target, ok := writes.Get(term)
		if !ok {
			return nil, errors.New("program/flow/candidates: Write row is not live")
		}
		family := keyspace.TermFamily(target)
		if family != keyspace.FamilyCell && !lensFamily(family) {
			return nil, errors.New("program/flow/candidates: invalid Write target family")
		}
		if proof.Executable(term) && lensFamily(family) {
			result.classes.writeClass[index] = accessIndexCandidate
			result.buckets.indexSet = append(result.buckets.indexSet, term)
		}
	}

	for index := 0; index < loops.Count(); index++ {
		term, ok := loops.At(index)
		if !ok {
			return nil, errors.New("program/flow/candidates: Loop view is not live")
		}
		_, _, loopKind, control, ok := loops.Get(term)
		if !ok {
			return nil, errors.New("program/flow/candidates: Loop row is not live")
		}
		if proof.Executable(term) && loopKind == kind.LoopGenericFor && fixedHeader(values, control) {
			result.classes.loopClass[index] = genericLoopCandidate
			result.buckets.genericLoop = append(result.buckets.genericLoop, term)
		}
	}

	return result, nil
}

func isCallCallee(calls authored.Calls, read keyspace.Term) bool {
	for index := 0; index < calls.Count(); index++ {
		call, ok := calls.At(index)
		if !ok {
			continue
		}
		_, callee, _, _, ok := calls.Get(call)
		if ok && callee == read {
			return true
		}
	}
	return false
}

func fixedHeader(values authored.Values, control keyspace.Term) bool {
	if keyspace.TermFamily(control) != keyspace.FamilyValues || keyspace.TermOrdinal(control) == 0 {
		return false
	}
	length, ok := values.Len(control)
	return ok && length > 0
}

func validateInputs(
	identity source.Identity,
	view authored.View,
	proof *executable.Result,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) ([keyspace.FamilyCount]int, error) {
	var counts [keyspace.FamilyCount]int
	contentID := identity.ContentID()
	if !contentID.Available() || identity.Name() == "" || identity.TermCount() == 0 {
		return counts, errors.New("program/flow/candidates: Source identity is unavailable")
	}
	if !view.Cold().ContentID().Available() || !staticID.Available() || !moduleID.Available() {
		return counts, errors.New("program/flow/candidates: authored view is unavailable")
	}
	if proof == nil {
		return counts, errors.New("program/flow/candidates: executable result is nil")
	}
	if !executable.Matches(proof, identity.ContentID(), view.Cold().ContentID(), staticID, moduleID) {
		return counts, errors.New("program/flow/candidates: executable Source/Flow/Static/Module identity mismatch")
	}

	for _, family := range [...]keyspace.Family{
		keyspace.FamilyUnary,
		keyspace.FamilyBinary,
		keyspace.FamilyRead,
		keyspace.FamilyWrite,
		keyspace.FamilyValues,
		keyspace.FamilyLensExact,
		keyspace.FamilyLensKey,
		keyspace.FamilyLoop,
	} {
		want := identity.FamilyCount(family)
		if want < 0 || !keyspace.TermOrdinalFits(want) {
			return counts, errors.New("program/flow/candidates: invalid Source family count")
		}
		if authored.FamilyCount(view, family) != want || proof.FamilyCount(family) != want {
			return counts, errors.New("program/flow/candidates: candidate family count mismatch")
		}
		counts[family] = want
	}
	return counts, nil
}

func lensFamily(family keyspace.Family) bool {
	return family == keyspace.FamilyLensExact || family == keyspace.FamilyLensKey
}
