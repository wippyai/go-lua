package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// AdviceRedundantGuards emits hint judgments for nil and runtime-type checks
// whose truth value is already proved by a dominating, stable guard fact.
//
// The proof is intentionally delegated to the canonical redundant-condition
// readmodel. This pass only narrows that solved surface to the nil/type checks
// covered by the advice rule; it does not inspect syntax or rebuild flow.
type AdviceRedundantGuards struct{}

func (AdviceRedundantGuards) Name() string {
	return "advice.redundant_guard"
}

func (AdviceRedundantGuards) Produce(ctx Context) []judgment.Judgment {
	functionKey := contextFunctionKey(ctx)
	var out []judgment.Judgment
	ctx.Reader.ForEachRedundantConditionBranch(func(branch readmodel.RedundantConditionBranch) bool {
		if !isAdviceRedundantGuardCheck(branch.Check) {
			return true
		}
		proof, ok := readmodel.DeriveRedundantConditionProof(ctx.Reader, branch)
		if !ok {
			return true
		}
		out = append(out, adviceRedundantGuardJudgment(ctx, functionKey, branch, proof))
		return true
	})
	return out
}

func isAdviceRedundantGuardCheck(check readmodel.BranchCheck) bool {
	switch check.Kind {
	case readmodel.BranchCheckNil, readmodel.BranchCheckNotNil:
		return true
	case readmodel.BranchCheckTypeEqual, readmodel.BranchCheckTypeNot:
		return check.TypeName != ""
	default:
		return false
	}
}

func adviceRedundantGuardJudgment(
	ctx Context,
	functionKey string,
	branch readmodel.RedundantConditionBranch,
	proof readmodel.RedundantConditionProof,
) judgment.Judgment {
	conditionSpan := branch.ConditionSpan
	if !conditionSpan.Valid() {
		conditionSpan = branch.StatementSpan
	}
	span := spanFromReadModel(ctx.SourceFile, conditionSpan)
	proofSpan := spanFromReadModel(ctx.SourceFile, proof.ProofSpan())
	label := redundantConditionCheckLabel(branch.Check)
	return judgment.Judgment{
		// Reuse the established hint-level advice code and renderer. The
		// condition's solved truth value is the proof consumed by that path.
		Code:  judgment.CodeAdviceAlwaysTrueGuard,
		Point: branch.Point,
		Subject: subjectRef(functionKey, judgment.SubjectExpression,
			fmt.Sprintf("advice-redundant-guard:%d:%d:%d", branch.Point, conditionSpan.StartLine, conditionSpan.StartCol),
			label),
		Actual:  judgment.NewValueRef(0, typ.Boolean).WithLabel(label),
		Verdict: judgment.VerdictProven,
		Evidence: judgment.EvidenceChain{
			provenAbstractEvidence(branch.Point, "advice:guard:redundant", judgment.AdviceGuardValueEvidenceDetail(proof.Always()), span),
		},
		Spans: []judgment.SpanRef{span, proofSpan},
	}
}
