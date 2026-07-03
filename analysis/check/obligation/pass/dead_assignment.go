package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// DeadAssignments emits lint obligations for writes whose value is overwritten
// before any reachable read on every path.
type DeadAssignments struct{}

func (DeadAssignments) Name() string {
	return "lint.dead.assignment"
}

func (DeadAssignments) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachDeadAssignment(func(assignment readmodel.DeadAssignment) bool {
		out = append(out, deadAssignmentJudgment(ctx, functionKey, assignment))
		return true
	})
	return out
}

func deadAssignmentJudgment(ctx Context, functionKey string, assignment readmodel.DeadAssignment) judgment.Judgment {
	evidence := make(judgment.EvidenceChain, 0, len(assignment.Overwrites)+len(assignment.Exits))
	for _, overwrite := range assignment.Overwrites {
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Detail: judgment.EvidenceDetail{
				Kind: judgment.EvidenceDetailDeadAssignmentOverwrite,
			},
			Origin: judgment.OriginRef{
				Point: overwrite.Point,
				Key:   "dead-assignment:overwrite",
			},
			Span: spanFromReadModel(ctx.SourceFile, overwrite.Span),
		})
	}
	for _, exit := range assignment.Exits {
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Detail: judgment.EvidenceDetail{
				Kind: judgment.EvidenceDetailDeadAssignmentExit,
			},
			Origin: judgment.OriginRef{
				Point: exit.Point,
				Key:   "dead-assignment:exit",
			},
			Span: spanFromReadModel(ctx.SourceFile, exit.Span),
		})
	}
	return judgment.Judgment{
		Code:  judgment.CodeDeadAssignment,
		Point: assignment.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectPath,
			fmt.Sprintf("dead-assignment:%d:%s", assignment.Point, assignment.Key),
		).WithLabel(assignment.Name),
		Verdict:  judgment.VerdictRefuted,
		Evidence: evidence,
		Spans:    []judgment.SpanRef{spanFromReadModel(ctx.SourceFile, assignment.WriteSpan)},
	}
}
