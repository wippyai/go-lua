package readmodel

import "github.com/wippyai/go-lua/analysis/type/typ"

type RedundantConditionProofState uint8

const (
	RedundantConditionProofTruthy RedundantConditionProofState = iota + 1
	RedundantConditionProofFalsy
	RedundantConditionProofNil
	RedundantConditionProofNotNil
	RedundantConditionProofRuntimeType
	RedundantConditionProofNotRuntimeType
	RedundantConditionProofLiteral
	RedundantConditionProofNotLiteral
)

type RedundantConditionProven struct {
	State       RedundantConditionProofState
	RuntimeType string
	Literal     typ.Type
}

// RedundantConditionProof captures the prior-guard fact proving that a later
// condition is redundant. Readmodel owns the proof semantics; renderers own the
// wording.
type RedundantConditionProof struct {
	always    bool
	check     BranchCheck
	proven    RedundantConditionProven
	proofSpan SourceSpan
}

func (p RedundantConditionProof) Always() bool                     { return p.always }
func (p RedundantConditionProof) Check() BranchCheck               { return p.check }
func (p RedundantConditionProof) Proven() RedundantConditionProven { return p.proven }
func (p RedundantConditionProof) ProofSpan() SourceSpan            { return p.proofSpan }

// DeriveRedundantConditionProof derives the user-visible redundant-condition
// proof from canonical dominating branch facts exposed through Reader.
func DeriveRedundantConditionProof(reader Reader, branch RedundantConditionBranch) (RedundantConditionProof, bool) {
	check := branch.Check
	if check.Path.IsEmpty() {
		return RedundantConditionProof{}, false
	}
	if proof, ok := reader.DominatingTruthyBranchForPath(branch.Point, check); ok {
		if out, ok := proofFromDominatingTruthy(check, proof.Span); ok {
			return out, true
		}
	}
	proof, ok := reader.DominatingBranchCheckForPath(branch.Point, check, func(prior BranchCheck, edge bool) bool {
		_, ok := proofFromDirectBranch(prior, edge, check, SourceSpan{})
		return ok
	})
	if !ok {
		return RedundantConditionProof{}, false
	}
	return proofFromDirectBranch(proof.Check, proof.Edge, check, proof.Span)
}

func proofFromDominatingTruthy(check BranchCheck, span SourceSpan) (RedundantConditionProof, bool) {
	switch check.Kind {
	case BranchCheckTruthy:
		return redundantConditionProof(check, true, RedundantConditionProven{State: RedundantConditionProofTruthy}, span), true
	case BranchCheckFalsy:
		return redundantConditionProof(check, false, RedundantConditionProven{State: RedundantConditionProofTruthy}, span), true
	case BranchCheckNil:
		return redundantConditionProof(check, false, RedundantConditionProven{State: RedundantConditionProofNotNil}, span), true
	case BranchCheckNotNil:
		return redundantConditionProof(check, true, RedundantConditionProven{State: RedundantConditionProofNotNil}, span), true
	case BranchCheckTypeEqual:
		if check.TypeName == "nil" {
			return redundantConditionProof(check, false, RedundantConditionProven{State: RedundantConditionProofNotNil}, span), true
		}
	case BranchCheckTypeNot:
		if check.TypeName == "nil" {
			return redundantConditionProof(check, true, RedundantConditionProven{State: RedundantConditionProofNotNil}, span), true
		}
	}
	return RedundantConditionProof{}, false
}

func proofFromDirectBranch(prior BranchCheck, edge bool, current BranchCheck, span SourceSpan) (RedundantConditionProof, bool) {
	switch current.Kind {
	case BranchCheckTruthy, BranchCheckFalsy:
		proven, ok := truthinessProof(prior, edge)
		if !ok {
			return RedundantConditionProof{}, false
		}
		return redundantConditionProof(current, proven.State == truthinessWanted(current), proven, span), true
	case BranchCheckLiteralEqual, BranchCheckLiteralNot:
		return literalConditionProof(prior, edge, current, span)
	case BranchCheckTypeEqual, BranchCheckTypeNot:
		if current.TypeName == "nil" {
			if proof, ok := nilConditionProof(prior, edge, current, span); ok {
				return proof, true
			}
		}
		return runtimeTypeConditionProof(prior, edge, current, span)
	case BranchCheckNil, BranchCheckNotNil:
		return nilConditionProof(prior, edge, current, span)
	default:
		return RedundantConditionProof{}, false
	}
}

func truthinessWanted(check BranchCheck) RedundantConditionProofState {
	if check.Kind == BranchCheckTruthy {
		return RedundantConditionProofTruthy
	}
	return RedundantConditionProofFalsy
}

func truthinessProof(check BranchCheck, edge bool) (RedundantConditionProven, bool) {
	switch check.Kind {
	case BranchCheckTruthy:
		return truthinessState(edge), true
	case BranchCheckFalsy:
		return truthinessState(!edge), true
	case BranchCheckTypeEqual, BranchCheckTypeNot:
		positive := (check.Kind == BranchCheckTypeEqual) == edge
		return runtimeTypeTruthinessProof(check.TypeName, positive)
	case BranchCheckLiteralEqual, BranchCheckLiteralNot:
		lit, ok := check.LiteralValue()
		if !ok {
			return RedundantConditionProven{}, false
		}
		positive := (check.Kind == BranchCheckLiteralEqual) == edge
		return literalTruthinessProof(lit, positive)
	default:
		return RedundantConditionProven{}, false
	}
}

func truthinessState(truthy bool) RedundantConditionProven {
	if truthy {
		return RedundantConditionProven{State: RedundantConditionProofTruthy}
	}
	return RedundantConditionProven{State: RedundantConditionProofFalsy}
}

func runtimeTypeTruthinessProof(name string, positive bool) (RedundantConditionProven, bool) {
	if !positive {
		return RedundantConditionProven{}, false
	}
	switch name {
	case "nil":
		return RedundantConditionProven{State: RedundantConditionProofFalsy}, true
	case "string", "number", "function", "table":
		return RedundantConditionProven{State: RedundantConditionProofTruthy}, true
	default:
		return RedundantConditionProven{}, false
	}
}

func literalTruthinessProof(lit typ.Type, positive bool) (RedundantConditionProven, bool) {
	if !positive {
		return RedundantConditionProven{}, false
	}
	if typ.Nil.Equals(lit) || typ.False.Equals(lit) {
		return RedundantConditionProven{State: RedundantConditionProofFalsy}, true
	}
	return RedundantConditionProven{State: RedundantConditionProofTruthy}, true
}

func literalConditionProof(prior BranchCheck, edge bool, current BranchCheck, span SourceSpan) (RedundantConditionProof, bool) {
	currentLit, ok := current.LiteralValue()
	if !ok {
		return RedundantConditionProof{}, false
	}
	priorLit, positive, ok := literalProof(prior, edge)
	if !ok {
		return RedundantConditionProof{}, false
	}
	match := typ.TypeEquals(priorLit, currentLit)
	if !positive && !match {
		return RedundantConditionProof{}, false
	}
	state := RedundantConditionProofLiteral
	if !positive {
		state = RedundantConditionProofNotLiteral
	}
	always := (positive && match) != (current.Kind == BranchCheckLiteralNot)
	return redundantConditionProof(current, always, RedundantConditionProven{State: state, Literal: priorLit}, span), true
}

func literalProof(check BranchCheck, edge bool) (typ.Type, bool, bool) {
	lit, ok := check.LiteralValue()
	if !ok {
		return nil, false, false
	}
	switch check.Kind {
	case BranchCheckLiteralEqual:
		return lit, edge, true
	case BranchCheckLiteralNot:
		return lit, !edge, true
	default:
		return nil, false, false
	}
}

func runtimeTypeConditionProof(prior BranchCheck, edge bool, current BranchCheck, span SourceSpan) (RedundantConditionProof, bool) {
	name, positive, ok := runtimeTypeProof(prior, edge)
	if !ok {
		return RedundantConditionProof{}, false
	}
	if !positive && name != current.TypeName {
		return RedundantConditionProof{}, false
	}
	state := RedundantConditionProofRuntimeType
	if !positive {
		state = RedundantConditionProofNotRuntimeType
	}
	always := (positive && name == current.TypeName) != (current.Kind == BranchCheckTypeNot)
	return redundantConditionProof(current, always, RedundantConditionProven{State: state, RuntimeType: name}, span), true
}

func runtimeTypeProof(check BranchCheck, edge bool) (string, bool, bool) {
	if check.TypeName == "" {
		return "", false, false
	}
	switch check.Kind {
	case BranchCheckTypeEqual:
		return check.TypeName, edge, true
	case BranchCheckTypeNot:
		return check.TypeName, !edge, true
	default:
		return "", false, false
	}
}

func nilConditionProof(prior BranchCheck, edge bool, current BranchCheck, span SourceSpan) (RedundantConditionProof, bool) {
	provenNil, ok := nilProof(prior, edge)
	if !ok {
		return RedundantConditionProof{}, false
	}
	wantsNil, ok := nilWanted(current)
	if !ok {
		return RedundantConditionProof{}, false
	}
	state := RedundantConditionProofNotNil
	if provenNil {
		state = RedundantConditionProofNil
	}
	return redundantConditionProof(current, provenNil == wantsNil, RedundantConditionProven{State: state}, span), true
}

func nilProof(check BranchCheck, edge bool) (bool, bool) {
	switch check.Kind {
	case BranchCheckNil:
		return edge, true
	case BranchCheckNotNil:
		return !edge, true
	case BranchCheckTypeEqual:
		return edge, check.TypeName == "nil"
	case BranchCheckTypeNot:
		return !edge, check.TypeName == "nil"
	default:
		return false, false
	}
}

func nilWanted(check BranchCheck) (bool, bool) {
	switch check.Kind {
	case BranchCheckNil:
		return true, true
	case BranchCheckNotNil:
		return false, true
	case BranchCheckTypeEqual:
		return true, check.TypeName == "nil"
	case BranchCheckTypeNot:
		return false, check.TypeName == "nil"
	default:
		return false, false
	}
}

func redundantConditionProof(check BranchCheck, always bool, proven RedundantConditionProven, span SourceSpan) RedundantConditionProof {
	return RedundantConditionProof{always: always, check: check, proven: proven, proofSpan: span}
}
