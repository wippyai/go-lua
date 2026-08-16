// Package programlaw owns exact source-to-Program semantic obligations.
//
// Its closed rows are deliberately narrower than the parser field-state
// inventory.  A row can discharge only after a parsed AST occurrence is
// anchored to one exact canonical Program Term and its typed relation is
// checked.  It never turns parser observation into a lowering claim, and it
// never reports global Program completion.
package programlaw

import "github.com/wippyai/go-lua/analysis/program/flow/kind"

// Site is the closed Program relation family owned by one exact source
// occurrence.  It is not a generic string-labelled test category.
type Site uint8

const (
	SiteInvalid Site = iota
	SiteUnary
	SiteBinary
	SiteSelect
	SiteCall
	SiteValues
	SiteOutcome
)

// ValuesMode distinguishes Lua's non-final scalar adjustment from final
// multiple-value propagation.  It is a Program Values law, not an AST-list
// count.
type ValuesMode uint8

const (
	ValuesInvalid ValuesMode = iota
	ValuesNonFinalScalar
	ValuesFinalOpen
)

// CallMode keeps plain and receiver calls disjoint because their canonical
// Call relation has distinct callee/receiver coordinates.
type CallMode uint8

const (
	CallInvalid CallMode = iota
	CallPlain
	CallMethod
)

// Requirement identifies one exact semantic relation.  Exactly the typed
// coordinate for Site is populated.  The representation is intentionally a
// tagged union rather than a catch-all law name, so adding an operation to the
// language requires adding its Program relation explicitly.
type Requirement struct {
	Site    Site
	Unary   kind.UnaryOp
	Binary  kind.BinaryOp
	Select  kind.SelectOp
	Call    CallMode
	Values  ValuesMode
	Outcome kind.OutcomeKind
}

// OperationRequirements is the closed operator denominator.  Its rows are
// independent of parser traces and fixture observations.
func OperationRequirements() []Requirement {
	rows := make([]Requirement, 0, 25)
	for _, op := range [...]kind.UnaryOp{
		kind.UnaryNeg,
		kind.UnaryNot,
		kind.UnaryLen,
		kind.UnaryBitNot,
	} {
		rows = append(rows, Requirement{Site: SiteUnary, Unary: op})
	}
	for _, op := range [...]kind.BinaryOp{
		kind.BinaryAdd,
		kind.BinarySub,
		kind.BinaryMul,
		kind.BinaryDiv,
		kind.BinaryIDiv,
		kind.BinaryMod,
		kind.BinaryPow,
		kind.BinaryConcat,
		kind.BinaryBitAnd,
		kind.BinaryBitOr,
		kind.BinaryBitXor,
		kind.BinaryShiftLeft,
		kind.BinaryShiftRight,
		kind.BinaryEqual,
		kind.BinaryNotEqual,
		kind.BinaryLess,
		kind.BinaryLessEqual,
		kind.BinaryGreater,
		kind.BinaryGreaterEqual,
	} {
		rows = append(rows, Requirement{Site: SiteBinary, Binary: op})
	}
	for _, op := range [...]kind.SelectOp{kind.SelectAnd, kind.SelectOr} {
		rows = append(rows, Requirement{Site: SiteSelect, Select: op})
	}
	return rows
}

// BoundaryRequirements covers the exact Program relations at source-list and
// source-control boundaries.  These rows intentionally do not claim the
// unimplemented candidate, Link, Target, or analysis-domain relations.
func BoundaryRequirements() []Requirement {
	return []Requirement{
		{Site: SiteCall, Call: CallPlain},
		{Site: SiteCall, Call: CallMethod},
		{Site: SiteValues, Values: ValuesNonFinalScalar},
		{Site: SiteValues, Values: ValuesFinalOpen},
		{Site: SiteOutcome, Outcome: kind.OutcomeReturn},
		{Site: SiteOutcome, Outcome: kind.OutcomeThrow},
		{Site: SiteOutcome, Outcome: kind.OutcomeYield},
		{Site: SiteOutcome, Outcome: kind.OutcomeCancel},
	}
}

// Requirements returns the independent finite exact-law denominator currently
// owned by this package.  It is intentionally not a Stage-3 completion count:
// parser occurrence residue and outstanding candidate/Target/Link schema rows
// remain separate blocking ledgers.
func Requirements() []Requirement {
	operations := OperationRequirements()
	boundaries := BoundaryRequirements()
	result := make([]Requirement, 0, len(operations)+len(boundaries))
	result = append(result, operations...)
	result = append(result, boundaries...)
	return result
}
