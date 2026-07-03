package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// RedundantConditions emits judgments for branch checks already proved by a
// dominating, non-invalidated guard.
type RedundantConditions struct{}

func (RedundantConditions) Name() string {
	return "condition.redundant"
}

func (RedundantConditions) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachRedundantConditionBranch(func(branch readmodel.RedundantConditionBranch) bool {
		if proof, ok := redundantConditionProofFromDominatingTruthy(ctx.Reader, branch); ok {
			out = append(out, redundantConditionJudgment(ctx, functionKey, branch, proof))
			return true
		}
		if proof, ok := redundantConditionProofFromDominatingDirectCheck(ctx.Reader, branch); ok {
			out = append(out, redundantConditionJudgment(ctx, functionKey, branch, proof))
			return true
		}
		return true
	})
	return out
}

type redundantConditionProof struct {
	always    bool
	check     string
	proven    string
	stable    string
	proofSpan readmodel.SourceSpan
}

func redundantConditionProofFromDominatingTruthy(reader readmodel.Reader, branch readmodel.RedundantConditionBranch) (redundantConditionProof, bool) {
	check := branch.Check
	if check.Path.IsEmpty() {
		return redundantConditionProof{}, false
	}
	proof, ok := reader.DominatingTruthyBranchForPath(branch.Point, check)
	if !ok {
		return redundantConditionProof{}, false
	}
	proofSpan := proof.Span
	switch check.Kind {
	case readmodel.BranchCheckTruthy:
		return redundantConditionProof{
			always:    true,
			check:     truthyConditionCheck(check.Path.String()),
			proven:    conditionPathProofEvidence(check.Path.String(), "truthy"),
			stable:    conditionStabilityEvidence(check.Path.String()),
			proofSpan: proofSpan,
		}, true
	case readmodel.BranchCheckFalsy:
		return redundantConditionProof{
			check:     falsyConditionCheck(check.Path.String()),
			proven:    conditionPathProofEvidence(check.Path.String(), "truthy"),
			stable:    conditionStabilityEvidence(check.Path.String()),
			proofSpan: proofSpan,
		}, true
	case readmodel.BranchCheckNil:
		return redundantConditionProof{
			check:     nilConditionCheck(check.Path.String()),
			proven:    conditionPathProofEvidence(check.Path.String(), "not nil"),
			stable:    conditionStabilityEvidence(check.Path.String()),
			proofSpan: proofSpan,
		}, true
	case readmodel.BranchCheckNotNil:
		return redundantConditionProof{
			always:    true,
			check:     nonNilConditionCheck(check.Path.String()),
			proven:    conditionPathProofEvidence(check.Path.String(), "not nil"),
			stable:    conditionStabilityEvidence(check.Path.String()),
			proofSpan: proofSpan,
		}, true
	case readmodel.BranchCheckTypeEqual:
		if check.TypeName == "nil" {
			return runtimeTypeConditionProof(check, false, "not nil", false, proofSpan), true
		}
	case readmodel.BranchCheckTypeNot:
		if check.TypeName == "nil" {
			return runtimeTypeConditionProof(check, true, "not nil", true, proofSpan), true
		}
	}
	return redundantConditionProof{}, false
}

func redundantConditionProofFromDominatingDirectCheck(reader readmodel.Reader, branch readmodel.RedundantConditionBranch) (redundantConditionProof, bool) {
	check := branch.Check
	if check.Path.IsEmpty() {
		return redundantConditionProof{}, false
	}
	proof, ok := reader.DominatingBranchCheckForPath(branch.Point, check, func(prior readmodel.BranchCheck, edge bool) bool {
		return directBranchCheckDetermines(prior, edge, check)
	})
	if !ok {
		return redundantConditionProof{}, false
	}
	out, ok := directBranchConditionProof(proof.Check, proof.Edge, check, proof.Span)
	if !ok {
		return redundantConditionProof{}, false
	}
	return out, true
}

func directBranchCheckDetermines(prior readmodel.BranchCheck, cond bool, current readmodel.BranchCheck) bool {
	_, ok := directBranchConditionProof(prior, cond, current, readmodel.SourceSpan{})
	return ok
}

func directBranchConditionProof(prior readmodel.BranchCheck, cond bool, current readmodel.BranchCheck, proofSpan readmodel.SourceSpan) (redundantConditionProof, bool) {
	switch current.Kind {
	case readmodel.BranchCheckTruthy, readmodel.BranchCheckFalsy:
		return directTruthyConditionProof(prior, cond, current, proofSpan)
	case readmodel.BranchCheckLiteralEqual, readmodel.BranchCheckLiteralNot:
		return directLiteralConditionProof(prior, cond, current, proofSpan)
	case readmodel.BranchCheckTypeEqual, readmodel.BranchCheckTypeNot:
		if current.TypeName == "nil" {
			if proof, ok := directNilConditionProof(prior, cond, current, proofSpan); ok {
				return proof, true
			}
		}
		return directTypeConditionProof(prior, cond, current, proofSpan)
	case readmodel.BranchCheckNil, readmodel.BranchCheckNotNil:
		return directNilConditionProof(prior, cond, current, proofSpan)
	default:
		return redundantConditionProof{}, false
	}
}

func directTruthyConditionProof(prior readmodel.BranchCheck, cond bool, current readmodel.BranchCheck, proofSpan readmodel.SourceSpan) (redundantConditionProof, bool) {
	provenTruthy, proven, ok := truthinessProofFromBranch(prior, cond)
	if !ok {
		return redundantConditionProof{}, false
	}
	wantsTruthy := current.Kind == readmodel.BranchCheckTruthy
	check := truthyConditionCheck(current.Path.String())
	if !wantsTruthy {
		check = falsyConditionCheck(current.Path.String())
	}
	return redundantConditionProof{
		always:    provenTruthy == wantsTruthy,
		check:     check,
		proven:    conditionPathProofEvidence(current.Path.String(), proven),
		stable:    conditionStabilityEvidence(current.Path.String()),
		proofSpan: proofSpan,
	}, true
}

func truthinessProofFromBranch(check readmodel.BranchCheck, cond bool) (bool, string, bool) {
	switch check.Kind {
	case readmodel.BranchCheckTruthy:
		if cond {
			return true, "truthy", true
		}
		return false, "falsy", true
	case readmodel.BranchCheckFalsy:
		if cond {
			return false, "falsy", true
		}
		return true, "truthy", true
	case readmodel.BranchCheckTypeEqual, readmodel.BranchCheckTypeNot:
		positive := (check.Kind == readmodel.BranchCheckTypeEqual) == cond
		return runtimeTypeTruthinessProof(check.TypeName, positive)
	case readmodel.BranchCheckLiteralEqual, readmodel.BranchCheckLiteralNot:
		lit, ok := check.LiteralValue()
		if !ok {
			return false, "", false
		}
		positive := (check.Kind == readmodel.BranchCheckLiteralEqual) == cond
		return literalTruthinessProof(lit, positive)
	}
	return false, "", false
}

func runtimeTypeTruthinessProof(name string, positive bool) (bool, string, bool) {
	if !positive {
		return false, "", false
	}
	switch name {
	case "nil":
		return false, "falsy", true
	case "string", "number", "function", "table":
		return true, "truthy", true
	default:
		return false, "", false
	}
}

func literalTruthinessProof(lit typ.Type, positive bool) (bool, string, bool) {
	if !positive {
		return false, "", false
	}
	if typ.Nil.Equals(lit) || typ.False.Equals(lit) {
		return false, "falsy", true
	}
	return true, "truthy", true
}

func directLiteralConditionProof(prior readmodel.BranchCheck, cond bool, current readmodel.BranchCheck, proofSpan readmodel.SourceSpan) (redundantConditionProof, bool) {
	lit, ok := current.LiteralValue()
	if !ok {
		return redundantConditionProof{}, false
	}
	priorLit, positive, ok := literalProofFromBranch(prior, cond)
	if !ok {
		return redundantConditionProof{}, false
	}
	match := typ.TypeEquals(priorLit, lit)
	if !positive && !match {
		return redundantConditionProof{}, false
	}
	equalHolds := positive && match
	if positive && !match {
		equalHolds = false
	}
	negated := current.Kind == readmodel.BranchCheckLiteralNot
	operator := "equals"
	if negated {
		operator = "does not equal"
	}
	proven := conditionPathProofEvidence(current.Path.String(), priorLit.String())
	if !positive {
		proven = conditionPathProofEvidence(current.Path.String(), "not "+priorLit.String())
	}
	return redundantConditionProof{
		always:    equalHolds != negated,
		check:     fmt.Sprintf("%s %s %s", current.Path.String(), operator, lit.String()),
		proven:    proven,
		stable:    conditionStabilityEvidence(current.Path.String()),
		proofSpan: proofSpan,
	}, true
}

func literalProofFromBranch(check readmodel.BranchCheck, cond bool) (typ.Type, bool, bool) {
	lit, ok := check.LiteralValue()
	if !ok {
		return nil, false, false
	}
	switch check.Kind {
	case readmodel.BranchCheckLiteralEqual:
		return lit, cond, true
	case readmodel.BranchCheckLiteralNot:
		return lit, !cond, true
	default:
		return nil, false, false
	}
}

func directTypeConditionProof(prior readmodel.BranchCheck, cond bool, current readmodel.BranchCheck, proofSpan readmodel.SourceSpan) (redundantConditionProof, bool) {
	name, positive, ok := typeProofFromBranch(prior, cond)
	if !ok {
		return redundantConditionProof{}, false
	}
	if !positive && name != current.TypeName {
		return redundantConditionProof{}, false
	}
	typeEqualHolds := positive && name == current.TypeName
	if positive && name != current.TypeName {
		typeEqualHolds = false
	}
	negated := current.Kind == readmodel.BranchCheckTypeNot
	proven := conditionTypeProofEvidence(current.Path.String(), name)
	if !positive {
		proven = conditionPathProofEvidence(current.Path.String(), "not "+name)
	}
	return redundantConditionProof{
		always:    typeEqualHolds != negated,
		check:     fmt.Sprintf("type(%s) %s %q", current.Path.String(), typeCheckOperator(negated), current.TypeName),
		proven:    proven,
		stable:    conditionStabilityEvidence(current.Path.String()),
		proofSpan: proofSpan,
	}, true
}

func typeProofFromBranch(check readmodel.BranchCheck, cond bool) (string, bool, bool) {
	if check.TypeName == "" {
		return "", false, false
	}
	switch check.Kind {
	case readmodel.BranchCheckTypeEqual:
		return check.TypeName, cond, true
	case readmodel.BranchCheckTypeNot:
		return check.TypeName, !cond, true
	default:
		return "", false, false
	}
}

func directNilConditionProof(prior readmodel.BranchCheck, cond bool, current readmodel.BranchCheck, proofSpan readmodel.SourceSpan) (redundantConditionProof, bool) {
	provenNil, ok := nilProofFromBranch(prior, cond)
	if !ok {
		return redundantConditionProof{}, false
	}
	var wantsNil bool
	switch current.Kind {
	case readmodel.BranchCheckNil:
		wantsNil = true
	case readmodel.BranchCheckNotNil:
		wantsNil = false
	case readmodel.BranchCheckTypeEqual:
		if current.TypeName != "nil" {
			return redundantConditionProof{}, false
		}
		wantsNil = true
	case readmodel.BranchCheckTypeNot:
		if current.TypeName != "nil" {
			return redundantConditionProof{}, false
		}
		wantsNil = false
	default:
		return redundantConditionProof{}, false
	}
	proven := "not nil"
	if provenNil {
		proven = "nil"
	}
	always := provenNil == wantsNil
	if current.Kind == readmodel.BranchCheckNil {
		return redundantConditionProof{
			always:    always,
			check:     nilConditionCheck(current.Path.String()),
			proven:    conditionPathProofEvidence(current.Path.String(), proven),
			stable:    conditionStabilityEvidence(current.Path.String()),
			proofSpan: proofSpan,
		}, true
	}
	if current.Kind == readmodel.BranchCheckNotNil {
		return redundantConditionProof{
			always:    always,
			check:     nonNilConditionCheck(current.Path.String()),
			proven:    conditionPathProofEvidence(current.Path.String(), proven),
			stable:    conditionStabilityEvidence(current.Path.String()),
			proofSpan: proofSpan,
		}, true
	}
	negated := current.Kind == readmodel.BranchCheckTypeNot
	return runtimeTypeConditionProof(current, negated, proven, always, proofSpan), true
}

func nilProofFromBranch(check readmodel.BranchCheck, cond bool) (bool, bool) {
	switch check.Kind {
	case readmodel.BranchCheckNil:
		return cond, true
	case readmodel.BranchCheckNotNil:
		return !cond, true
	case readmodel.BranchCheckTypeEqual:
		if check.TypeName == "nil" {
			return cond, true
		}
	case readmodel.BranchCheckTypeNot:
		if check.TypeName == "nil" {
			return !cond, true
		}
	}
	return false, false
}

func runtimeTypeConditionProof(check readmodel.BranchCheck, negated bool, proven string, always bool, proofSpan readmodel.SourceSpan) redundantConditionProof {
	return redundantConditionProof{
		always:    always,
		check:     fmt.Sprintf("type(%s) %s %q", check.Path.String(), typeCheckOperator(negated), check.TypeName),
		proven:    conditionPathProofEvidence(check.Path.String(), proven),
		stable:    conditionStabilityEvidence(check.Path.String()),
		proofSpan: proofSpan,
	}
}

func typeCheckOperator(negated bool) string {
	if negated {
		return "is not"
	}
	return "is"
}

func redundantConditionJudgment(
	ctx Context,
	functionKey string,
	branch readmodel.RedundantConditionBranch,
	proof redundantConditionProof,
) judgment.Judgment {
	span := branch.ConditionSpan
	if !span.Valid() {
		span = branch.StatementSpan
	}
	checkSpan := spanFromReadModel(ctx.SourceFile, span)
	proofSpan := spanFromReadModel(ctx.SourceFile, proof.proofSpan)
	return judgment.Judgment{
		Code:  judgment.CodeRedundantCondition,
		Point: branch.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("condition:%d:%d:%d", branch.Point, span.StartLine, span.StartCol),
		).WithLabel(proof.check),
		Verdict: judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{Point: branch.Point, Key: "condition:check"},
				Detail: judgment.RedundantConditionCheckEvidenceDetail(proof.check, proof.always),
				Span:   checkSpan,
			},
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{Point: branch.Point, Key: "condition:prior-guard"},
				Detail: judgment.RedundantConditionProofEvidenceDetail(proof.proven),
				Span:   proofSpan,
			},
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{Point: branch.Point, Key: "condition:stability"},
				Detail: judgment.RedundantConditionStabilityEvidenceDetail(proof.stable),
			},
		},
		Spans: []judgment.SpanRef{checkSpan, proofSpan},
	}
}

func truthyConditionCheck(path string) string {
	return path + " is checked as truthy"
}

func falsyConditionCheck(path string) string {
	return path + " is checked as falsy"
}

func nilConditionCheck(path string) string {
	return path + " == nil"
}

func nonNilConditionCheck(path string) string {
	return path + " ~= nil"
}

func conditionStabilityEvidence(path string) string {
	return path + " is unchanged between the prior guard and this check"
}

func conditionPathProofEvidence(path, state string) string {
	return "prior guard established " + path + " is " + state
}

func conditionTypeProofEvidence(path, runtimeType string) string {
	return "prior guard established type(" + path + ") is " + runtimeType
}
