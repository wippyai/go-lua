package binaryprimitive

import (
	"errors"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/program/flow/internal/causal"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// Seal derives the executable primitive Binary projection. Candidate bucket
// membership is the liveness authority; this pass never reconstructs it from
// authored spelling or from causal reachability. Branch comparisons are
// added only for an exact authored Branch condition with the exact two causal
// arms.
func Seal(
	sourceView source.View,
	flow authored.View,
	candidateResult *candidates.Result,
	causalResult *causal.Result,
	staticID keyspace.ContentID,
	moduleID keyspace.ContentID,
) (*Result, error) {
	sourceID := sourceView.Identity().ContentID()
	flowID := flow.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return nil, errors.New("program/flow/binaryprimitive: owner identity is unavailable")
	}
	if sourceView.Identity().Name() == "" || sourceView.Identity().TermCount() == 0 {
		return nil, errors.New("program/flow/binaryprimitive: Source view is unavailable")
	}
	if !candidates.Matches(candidateResult, sourceID, flowID, staticID, moduleID) {
		return nil, errors.New("program/flow/binaryprimitive: candidate provenance disagrees with Source, Flow, Static, or Module")
	}
	if !causal.Matches(causalResult, sourceID, flowID, staticID, moduleID) {
		return nil, errors.New("program/flow/binaryprimitive: causal provenance disagrees with Source, Flow, Static, or Module")
	}

	binaries := flow.Operators().Binaries()
	if binaries.Count() != sourceView.Identity().FamilyCount(keyspace.FamilyBinary) {
		return nil, errors.New("program/flow/binaryprimitive: Binary denominator disagrees with Source")
	}
	if !keyspace.TermOrdinalFits(binaries.Count()) {
		return nil, errors.New("program/flow/binaryprimitive: Binary denominator is unrepresentable")
	}

	result := &Result{
		sourceID: sourceID,
		flowID:   flowID,
		staticID: staticID,
		moduleID: moduleID,
		slots:    make([]uint32, binaries.Count()+1),
		buckets: bucketStore{
			arithmetic: make([]keyspace.Term, 0, candidateResult.Arithmetic().Count()),
			bitwise:    make([]keyspace.Term, 0, candidateResult.Bitwise().Count()),
			equality:   make([]keyspace.Term, 0, candidateResult.Equality().Count()),
			order:      make([]keyspace.Term, 0, candidateResult.Order().Count()),
		},
	}

	// First materialize the dense target buckets and primitive rows in
	// authored ordinal order. Concat is an intentional non-member of this
	// projection; no future operation can enter a bucket without an explicit
	// category in binaryCategoryFor.
	for index := 0; index < binaries.Count(); index++ {
		binary, ok := binaries.At(index)
		if !ok {
			return nil, errors.New("program/flow/binaryprimitive: Binary view is not live")
		}
		owner, op, left, right, ok := binaries.Get(binary)
		if !ok || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 ||
			!validBinaryOperands(left, right) {
			return nil, errors.New("program/flow/binaryprimitive: malformed Binary row")
		}
		category := candidateCategory(candidateResult, binary)
		if category == binaryCategoryInvalid {
			if candidateResult.Concat().Contains(binary) {
				if op != kind.BinaryConcat {
					return nil, errors.New("program/flow/binaryprimitive: candidate Concat category disagrees with Binary operation")
				}
				continue
			}
			// A candidate Binary must be one of the explicit candidate
			// buckets. An unclassified executable row is a construction bug,
			// not an empty semantic case.
			if candidateResult.Arithmetic().Contains(binary) || candidateResult.Bitwise().Contains(binary) ||
				candidateResult.Equality().Contains(binary) || candidateResult.Order().Contains(binary) {
				return nil, errors.New("program/flow/binaryprimitive: candidate Binary has inconsistent category")
			}
			continue
		}
		if category != binaryCategoryFor(op) {
			return nil, errors.New("program/flow/binaryprimitive: candidate category disagrees with Binary operation")
		}
		ordinal := keyspace.TermOrdinal(binary)
		if ordinal == 0 || uint64(ordinal) >= uint64(len(result.slots)) || result.slots[ordinal] != 0 {
			return nil, errors.New("program/flow/binaryprimitive: duplicate Binary slot")
		}
		result.primitives = append(result.primitives, primitiveRow{
			source:    binary,
			operation: Operation{Owner: owner, Op: op, Left: left, Right: right},
		})
		result.slots[ordinal] = uint32(len(result.primitives))
		appendBucket(&result.buckets, category, binary)
	}

	if err := sealComparisons(result, flow, causalResult); err != nil {
		return nil, err
	}
	return result, nil
}

func candidateCategory(result *candidates.Result, binary keyspace.Term) binaryCategory {
	if result == nil {
		return binaryCategoryInvalid
	}
	var category binaryCategory
	if result.Arithmetic().Contains(binary) {
		category = kindBinaryArithmetic
	}
	if result.Bitwise().Contains(binary) {
		if category != binaryCategoryInvalid {
			return binaryCategoryInvalid
		}
		category = kindBinaryBitwise
	}
	if result.Equality().Contains(binary) {
		if category != binaryCategoryInvalid {
			return binaryCategoryInvalid
		}
		category = kindBinaryEquality
	}
	if result.Order().Contains(binary) {
		if category != binaryCategoryInvalid {
			return binaryCategoryInvalid
		}
		category = kindBinaryOrder
	}
	return category
}

func appendBucket(buckets *bucketStore, category binaryCategory, binary keyspace.Term) {
	switch category {
	case kindBinaryArithmetic:
		buckets.arithmetic = append(buckets.arithmetic, binary)
	case kindBinaryBitwise:
		buckets.bitwise = append(buckets.bitwise, binary)
	case kindBinaryEquality:
		buckets.equality = append(buckets.equality, binary)
	case kindBinaryOrder:
		buckets.order = append(buckets.order, binary)
	}
}

func validBinaryOperands(left, right keyspace.Term) bool {
	return left != 0 && right != 0 && keyspace.TermFamily(left) > keyspace.FamilyInvalid &&
		keyspace.TermFamily(left) < keyspace.FamilyCount && keyspace.TermOrdinal(left) != 0 &&
		keyspace.TermFamily(right) > keyspace.FamilyInvalid && keyspace.TermFamily(right) < keyspace.FamilyCount &&
		keyspace.TermOrdinal(right) != 0
}

func sealComparisons(result *Result, flow authored.View, causalResult *causal.Result) error {
	return sealBranchComparisons(result, flow.Control().Branches(), causalResult)
}

// branchReader is the small authored Branch surface needed by the
// comparison pass. The production implementation is authored.Branches; the
// narrow seam also lets laws exercise duplicate-Branch rejection without
// manufacturing or mutating an authored owner.
type branchReader interface {
	Count() int
	At(index int) (keyspace.Term, bool)
	Get(term keyspace.Term) (owner, condition, whenTrue, whenFalse keyspace.Term, ok bool)
}

func sealBranchComparisons(result *Result, branches branchReader, causalResult *causal.Result) error {
	branchByBinary := make([]keyspace.Term, len(result.slots))
	for index := 0; index < branches.Count(); index++ {
		branch, ok := branches.At(index)
		if !ok {
			return errors.New("program/flow/binaryprimitive: Branch view is not live")
		}
		branchOwner, condition, whenTrue, whenFalse, ok := branches.Get(branch)
		if !ok || keyspace.TermFamily(branchOwner) != keyspace.FamilyBody || keyspace.TermOrdinal(branchOwner) == 0 ||
			keyspace.TermFamily(whenTrue) != keyspace.FamilyBody || keyspace.TermOrdinal(whenTrue) == 0 ||
			keyspace.TermFamily(whenFalse) != keyspace.FamilyBody || keyspace.TermOrdinal(whenFalse) == 0 {
			return errors.New("program/flow/binaryprimitive: malformed Branch arms")
		}
		if keyspace.TermFamily(condition) != keyspace.FamilyBinary || keyspace.TermOrdinal(condition) == 0 {
			continue
		}
		ordinal := keyspace.TermOrdinal(condition)
		if uint64(ordinal) >= uint64(len(result.slots)) || result.slots[ordinal] == 0 {
			continue
		}
		if branchByBinary[ordinal] != 0 {
			return errors.New("program/flow/binaryprimitive: duplicate Branch condition for Binary")
		}
		branchByBinary[ordinal] = branch
		slot := result.slots[ordinal] - 1
		if uint64(slot) >= uint64(len(result.primitives)) {
			return errors.New("program/flow/binaryprimitive: Binary slot is unavailable")
		}
		row := &result.primitives[slot]
		if branchOwner != row.operation.Owner || whenTrue == whenFalse {
			return errors.New("program/flow/binaryprimitive: Branch owner or arms disagree with Binary")
		}
		if binaryCategoryFor(row.operation.Op) != kindBinaryEquality && binaryCategoryFor(row.operation.Op) != kindBinaryOrder {
			return errors.New("program/flow/binaryprimitive: non-comparison Binary used as Branch condition")
		}
		comparison := Comparison{
			Branch: branch, TrueBody: whenTrue, FalseBody: whenFalse,
			Left: row.operation.Left, Right: row.operation.Right,
		}
		normalizeComparison(row.operation.Op, &comparison)
		if err := validateCausalComparison(causalResult, row.source, comparison); err != nil {
			return err
		}
		row.comparison = comparison
		row.hasCompare = true
	}
	return nil
}

func normalizeComparison(op kind.BinaryOp, comparison *Comparison) {
	if comparison == nil {
		return
	}
	switch op {
	case kind.BinaryNotEqual:
		comparison.Invert = true
	case kind.BinaryGreater, kind.BinaryGreaterEqual:
		comparison.Left, comparison.Right = comparison.Right, comparison.Left
	}
}

type causalSuccessorReader interface {
	Count(from keyspace.Term) int
	At(from keyspace.Term, index int) (causal.Successor, bool)
}

func validateCausalComparison(result *causal.Result, binary keyspace.Term, comparison Comparison) error {
	if result == nil {
		return errors.New("program/flow/binaryprimitive: causal result is unavailable")
	}
	return validateCausalSuccessorArms(result.Successors(), binary, comparison)
}

func validateCausalSuccessorArms(successors causalSuccessorReader, binary keyspace.Term, comparison Comparison) error {
	if successors.Count(binary) != 2 {
		return errors.New("program/flow/binaryprimitive: comparison causal arm count is not exactly two")
	}
	seenTrue, seenFalse := false, false
	for index := 0; index < 2; index++ {
		successor, ok := successors.At(binary, index)
		if !ok || !successor.IsLocal() || successor.From != binary || successor.Decision != comparison.Branch {
			return errors.New("program/flow/binaryprimitive: comparison causal arm is malformed")
		}
		switch {
		case successor.Truth && successor.To == comparison.TrueBody && !seenTrue:
			seenTrue = true
		case !successor.Truth && successor.To == comparison.FalseBody && !seenFalse:
			seenFalse = true
		default:
			return errors.New("program/flow/binaryprimitive: comparison causal arm does not match Branch")
		}
	}
	if !seenTrue || !seenFalse {
		return errors.New("program/flow/binaryprimitive: comparison causal arms are incomplete")
	}
	return nil
}
