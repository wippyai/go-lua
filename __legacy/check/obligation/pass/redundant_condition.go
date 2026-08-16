package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
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
		if proof, ok := readmodel.DeriveRedundantConditionProof(ctx.Reader, branch); ok {
			out = append(out, redundantConditionJudgment(ctx, functionKey, branch, proof))
		}
		return true
	})
	return out
}

func redundantConditionJudgment(
	ctx Context,
	functionKey string,
	branch readmodel.RedundantConditionBranch,
	proof readmodel.RedundantConditionProof,
) judgment.Judgment {
	span := branch.ConditionSpan
	if !span.Valid() {
		span = branch.StatementSpan
	}
	checkSpan := spanFromReadModel(ctx.SourceFile, span)
	proofSpan := spanFromReadModel(ctx.SourceFile, proof.ProofSpan())
	checkLabel := redundantConditionCheckLabel(proof.Check())
	provenEvidence := redundantConditionProvenEvidence(proof.Check().Path.String(), proof.Proven())
	return judgment.Judgment{
		Code:  judgment.CodeRedundantCondition,
		Point: branch.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("condition:%d:%d:%d", branch.Point, span.StartLine, span.StartCol),
		).WithLabel(checkLabel),
		Verdict: judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{Point: branch.Point, Key: "condition:check"},
				Detail: judgment.RedundantConditionCheckEvidenceDetail(checkLabel, proof.Always()),
				Span:   checkSpan,
			},
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{Point: branch.Point, Key: "condition:prior-guard"},
				Detail: judgment.RedundantConditionProofEvidenceDetail(provenEvidence),
				Span:   proofSpan,
			},
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{Point: branch.Point, Key: "condition:stability"},
				Detail: judgment.RedundantConditionStabilityEvidenceDetail(conditionStabilityEvidence(proof.Check().Path.String())),
			},
		},
		Spans: []judgment.SpanRef{checkSpan, proofSpan},
	}
}

func redundantConditionCheckLabel(check readmodel.BranchCheck) string {
	path := check.Path.String()
	switch check.Kind {
	case readmodel.BranchCheckTruthy:
		return path + " is checked as truthy"
	case readmodel.BranchCheckFalsy:
		return path + " is checked as falsy"
	case readmodel.BranchCheckNil:
		return path + " == nil"
	case readmodel.BranchCheckNotNil:
		return path + " ~= nil"
	case readmodel.BranchCheckLiteralEqual, readmodel.BranchCheckLiteralNot:
		lit, ok := check.LiteralValue()
		if !ok {
			return path
		}
		operator := "equals"
		if check.Kind == readmodel.BranchCheckLiteralNot {
			operator = "does not equal"
		}
		return fmt.Sprintf("%s %s %s", path, operator, lit.String())
	case readmodel.BranchCheckTypeEqual, readmodel.BranchCheckTypeNot:
		negated := check.Kind == readmodel.BranchCheckTypeNot
		return fmt.Sprintf("type(%s) %s %q", path, typeCheckOperator(negated), check.TypeName)
	default:
		return path
	}
}

func redundantConditionProvenEvidence(path string, proven readmodel.RedundantConditionProven) string {
	switch proven.State {
	case readmodel.RedundantConditionProofTruthy:
		return conditionPathProofEvidence(path, "truthy")
	case readmodel.RedundantConditionProofFalsy:
		return conditionPathProofEvidence(path, "falsy")
	case readmodel.RedundantConditionProofNil:
		return conditionPathProofEvidence(path, "nil")
	case readmodel.RedundantConditionProofNotNil:
		return conditionPathProofEvidence(path, "not nil")
	case readmodel.RedundantConditionProofRuntimeType:
		return conditionTypeProofEvidence(path, proven.RuntimeType)
	case readmodel.RedundantConditionProofNotRuntimeType:
		return conditionPathProofEvidence(path, "not "+proven.RuntimeType)
	case readmodel.RedundantConditionProofLiteral:
		return conditionPathProofEvidence(path, literalProofLabel(proven.Literal))
	case readmodel.RedundantConditionProofNotLiteral:
		return conditionPathProofEvidence(path, "not "+literalProofLabel(proven.Literal))
	default:
		return conditionPathProofEvidence(path, "unknown")
	}
}

func literalProofLabel(lit typ.Type) string {
	if lit == nil {
		return "unknown"
	}
	return lit.String()
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

func typeCheckOperator(negated bool) string {
	if negated {
		return "is not"
	}
	return "is"
}
