package evaluation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type pendingBuilder struct {
	view       authored.View
	executable *executable.Result
	candidates *candidates.Result
	store      *pendingTermStore
	roots      [keyspace.FamilyCount][]uint32
	// parents records only the 17 structural evaluation-composite families.
	// Scalar/literal/function payloads do not acquire parent identity planes.
	// They use compact claimed planes to reject repeated authored occurrences,
	// while true references (Key, Cell, Body) may be shared.
	parents  [pendingAncestorFamilyCount][]keyspace.Term
	claimed  [pendingClaimFamilyCount][]bool
	demand   [pendingAncestorFamilyCount][]bool
	discover bool
	// subjectsExpected is derived from the executable/candidate denominator;
	// subjectsWalked is incremented only when Source's direct-root walk reaches
	// one exact subject. Comparing the counters closes root coverage without a
	// second full subject scan.
	subjectsExpected uint32
	subjectsWalked   uint32
}

// pendingAncestorFamilies is the exact union which can be an evaluation
// ancestor.  Function is intentionally absent: a Function Term is a scalar
// callee payload, not an evaluation row with children.  Keep this list in
// lock-step with Session's composite vocabulary and pending root handling.
type pendingAncestorFamily uint8

const (
	pendingValues pendingAncestorFamily = iota
	pendingLensExact
	pendingLensKey
	pendingRead
	pendingUnary
	pendingBinary
	pendingSelect
	pendingBind
	pendingAssign
	pendingCall
	pendingReturn
	pendingTable
	pendingTableField
	pendingValueClaim
	pendingWrite
	pendingBranch
	pendingLoop
	pendingAncestorFamilyCount
)

var pendingAncestorFamilyKeys = [...]keyspace.Family{
	keyspace.FamilyValues, keyspace.FamilyLensExact, keyspace.FamilyLensKey,
	keyspace.FamilyRead, keyspace.FamilyUnary, keyspace.FamilyBinary,
	keyspace.FamilySelect, keyspace.FamilyBind, keyspace.FamilyAssign,
	keyspace.FamilyCall, keyspace.FamilyReturn, keyspace.FamilyTable,
	keyspace.FamilyTableField, keyspace.FamilyValueClaim, keyspace.FamilyWrite,
	keyspace.FamilyBranch, keyspace.FamilyLoop,
}

type pendingClaimFamily uint8

const (
	pendingNil pendingClaimFamily = iota
	pendingBool
	pendingInteger
	pendingFloat
	pendingString
	pendingVararg
	pendingFunction
	pendingTypeValue
	pendingClaimFamilyCount
)

var pendingClaimFamilyKeys = [...]keyspace.Family{
	keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
	keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyVararg,
	keyspace.FamilyFunction, keyspace.FamilyTypeValue,
}

func pendingClaimIndex(family keyspace.Family) (pendingClaimFamily, bool) {
	switch family {
	case keyspace.FamilyNil:
		return pendingNil, true
	case keyspace.FamilyBool:
		return pendingBool, true
	case keyspace.FamilyInteger:
		return pendingInteger, true
	case keyspace.FamilyFloat:
		return pendingFloat, true
	case keyspace.FamilyString:
		return pendingString, true
	case keyspace.FamilyVararg:
		return pendingVararg, true
	case keyspace.FamilyFunction:
		return pendingFunction, true
	case keyspace.FamilyTypeValue:
		return pendingTypeValue, true
	default:
		return 0, false
	}
}

func pendingAncestorIndex(family keyspace.Family) (pendingAncestorFamily, bool) {
	switch family {
	case keyspace.FamilyValues:
		return pendingValues, true
	case keyspace.FamilyLensExact:
		return pendingLensExact, true
	case keyspace.FamilyLensKey:
		return pendingLensKey, true
	case keyspace.FamilyRead:
		return pendingRead, true
	case keyspace.FamilyUnary:
		return pendingUnary, true
	case keyspace.FamilyBinary:
		return pendingBinary, true
	case keyspace.FamilySelect:
		return pendingSelect, true
	case keyspace.FamilyBind:
		return pendingBind, true
	case keyspace.FamilyAssign:
		return pendingAssign, true
	case keyspace.FamilyCall:
		return pendingCall, true
	case keyspace.FamilyReturn:
		return pendingReturn, true
	case keyspace.FamilyTable:
		return pendingTable, true
	case keyspace.FamilyTableField:
		return pendingTableField, true
	case keyspace.FamilyValueClaim:
		return pendingValueClaim, true
	case keyspace.FamilyWrite:
		return pendingWrite, true
	case keyspace.FamilyBranch:
		return pendingBranch, true
	case keyspace.FamilyLoop:
		return pendingLoop, true
	default:
		return 0, false
	}
}

func (builder *pendingBuilder) neededTerm(term keyspace.Term) bool {
	if builder == nil {
		return false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 {
		return false
	}
	index, ok := pendingAncestorIndex(family)
	if !ok {
		return false
	}
	plane := builder.demand[index]
	return uint64(ordinal) < uint64(len(plane)) && plane[ordinal]
}

func (builder *pendingBuilder) needed(term keyspace.Term) bool { return builder.neededTerm(term) }

func (builder *pendingBuilder) add(root uint32, term keyspace.Term) (uint32, error) {
	if builder == nil || builder.store == nil || builder.executable == nil || term == 0 || !builder.executable.Executable(term) || !pendingPayloadTerm(term) {
		return root, nil
	}
	return builder.store.insert(root, term)
}

func (builder *pendingBuilder) subject(term keyspace.Term, root uint32) error {
	if builder == nil || builder.discover || !pendingSubject(builder.view, builder.executable, builder.candidates, term) {
		return nil
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if !pendingSubjectFamily(family) || ordinal == 0 || uint64(ordinal) >= uint64(len(builder.roots[family])) {
		return errors.New("program/flow/evaluation: pending subject is outside dense roots")
	}
	if builder.roots[family][ordinal] != 0 {
		return errors.New("program/flow/evaluation: duplicate pending subject")
	}
	code, err := builder.store.code(root)
	if err != nil {
		return err
	}
	builder.roots[family][ordinal] = code
	builder.subjectsWalked++
	return nil
}

func (builder *pendingBuilder) rootAllowed(term keyspace.Term) bool {
	if builder == nil || !builder.needed(term) {
		return false
	}
	_, ok := pendingAncestorIndex(keyspace.TermFamily(term))
	return ok
}

func (builder *pendingBuilder) recordEdge(parent, child keyspace.Term) error {
	if builder == nil || parent == 0 || child == 0 {
		return errors.New("program/flow/evaluation: invalid pending containment edge")
	}
	if parent == child {
		return errors.New("program/flow/evaluation: cyclic pending containment")
	}
	family, ordinal := keyspace.TermFamily(child), keyspace.TermOrdinal(child)
	index, ok := pendingAncestorIndex(family)
	if ok {
		if ordinal == 0 || uint64(ordinal) >= uint64(len(builder.parents[index])) {
			return errors.New("program/flow/evaluation: pending edge leaves dense universe")
		}
		if builder.parents[index][ordinal] != 0 {
			return errors.New("program/flow/evaluation: pending child has multiple parents")
		}
		builder.parents[index][ordinal] = parent
		return nil
	}
	claimIndex, claim := pendingClaimIndex(family)
	if !claim {
		// Key/Cell/Body are true references and may be shared. They are
		// validated by their own Source/Flow authorities.
		return nil
	}
	if ordinal == 0 || uint64(ordinal) >= uint64(len(builder.claimed[claimIndex])) {
		return errors.New("program/flow/evaluation: pending payload leaves dense universe")
	}
	if builder.claimed[claimIndex][ordinal] {
		return errors.New("program/flow/evaluation: pending payload has multiple occurrences")
	}
	builder.claimed[claimIndex][ordinal] = true
	return nil
}

func pendingPayloadTerm(term keyspace.Term) bool {
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil, keyspace.FamilyBool, keyspace.FamilyInteger,
		keyspace.FamilyFloat, keyspace.FamilyString, keyspace.FamilyRead,
		keyspace.FamilyVararg, keyspace.FamilyUnary, keyspace.FamilyBinary,
		keyspace.FamilySelect, keyspace.FamilyFunction, keyspace.FamilyCall,
		keyspace.FamilyTable, keyspace.FamilyTypeValue, keyspace.FamilyValueClaim,
		keyspace.FamilyLensExact, keyspace.FamilyLensKey:
		return keyspace.TermOrdinal(term) != 0
	default:
		return false
	}
}

// pendingPrefixWrapper is a non-payload evaluation container whose already
// evaluated children may still contribute to a later subject prefix. A
// demanded child causes the ordinary walk to enter the wrapper; prefixCarry
// also enters an otherwise non-demanded wrapper when it precedes a demanded
// sibling. Payload boundaries (Call, Unary, Binary, and so on) stay opaque and
// are retained as one Term instead of flattening their own operands.
func pendingPrefixWrapper(term keyspace.Term) bool {
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyValues, keyspace.FamilyTableField:
		return true
	default:
		return false
	}
}

func pendingSubject(view authored.View, executableResult *executable.Result, candidateResult *candidates.Result, term keyspace.Term) bool {
	if executableResult == nil || !executableResult.Executable(term) {
		return false
	}
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyCall:
		return true
	case keyspace.FamilyUnary:
		return candidateResult != nil && (candidateResult.UnaryNumeric().Contains(term) || candidateResult.Length().Contains(term))
	case keyspace.FamilyBinary:
		return candidateResult != nil && (candidateResult.Arithmetic().Contains(term) || candidateResult.Bitwise().Contains(term) ||
			candidateResult.Concat().Contains(term) || candidateResult.Equality().Contains(term) || candidateResult.Order().Contains(term))
	case keyspace.FamilyRead:
		return candidateResult != nil && candidateResult.IndexGet().Contains(term)
	case keyspace.FamilyWrite:
		return candidateResult != nil && candidateResult.IndexSet().Contains(term)
	case keyspace.FamilyLoop:
		return candidateResult != nil && candidateResult.GenericLoop().Contains(term)
	default:
		return false
	}
}
