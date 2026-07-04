package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// Returns emits declared-return type obligations from solved state.
type Returns struct{}

func (Returns) Name() string {
	return "return.type"
}

func (Returns) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachReturn(func(ret readmodel.Return) bool {
		if ret.Check.Admissible || ret.Expected == nil || ret.HasUnownedTopActual() {
			return true
		}
		out = append(out, returnJudgment(ctx, functionKey, ret))
		return true
	})
	return out
}

func returnJudgment(ctx Context, functionKey string, ret readmodel.Return) judgment.Judgment {
	verdict := judgment.VerdictUnknown
	if ret.Check.ProvenMismatch {
		verdict = judgment.VerdictRefuted
	}
	actualTrust := judgment.EvidenceTrustUnknown
	if ret.ActualTypeKnown() {
		actualTrust = judgment.EvidenceTrustProven
	}
	missingProofTrust := judgment.EvidenceTrustUnknown
	if ret.MissingProofRefuted() {
		missingProofTrust = judgment.EvidenceTrustRefuted
	}
	var missingProofDetail judgment.EvidenceDetail
	switch ret.Check.Mismatch.Kind {
	case readmodel.ReturnMismatchMissingRequiredField:
		missingProofDetail = judgment.MissingRequiredFieldTypeEvidenceDetail(
			ret.Check.Mismatch.Field,
			ret.Check.Mismatch.Type,
		)
	case readmodel.ReturnMismatchMayBeNil:
		if ret.SourceIndexedRead {
			missingProofDetail = judgment.IndexedReadMissingProofEvidenceDetail()
		} else {
			missingProofDetail = judgment.MayBeNilEvidenceDetail()
		}
	}
	evidence := judgment.EvidenceChain{
		{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: actualTrust,
			Origin: judgment.OriginRef{
				Point: ret.Point,
				Key:   "return:actual",
			},
			Span: spanFromReadModel(ctx.SourceFile, ret.SourceSpan),
		},
		{
			Kind:  judgment.EvidenceUserAssertion,
			Trust: judgment.EvidenceTrustClaimed,
			Origin: judgment.OriginRef{
				Point: ret.Point,
				Key:   "return:expected",
			},
			Span: spanFromReadModel(ctx.SourceFile, ret.DeclarationSpan),
		},
		{
			Kind:   judgment.EvidenceMissingProof,
			Trust:  missingProofTrust,
			Detail: missingProofDetail,
			Origin: judgment.OriginRef{
				Point: ret.Point,
				Key:   "return:proof",
			},
		},
	}
	if ret.ExplicitTopOrigin {
		sourceLabel := ret.SourceLabel
		if sourceLabel == "" {
			sourceLabel = ret.ExpectedLabel
		}
		evidence = append(evidence, judgment.Evidence{
			Kind:   judgment.EvidenceUserAssertion,
			Trust:  judgment.EvidenceTrustClaimed,
			Detail: judgment.UserAssertedAnyEvidenceDetail(sourceLabel),
			Origin: judgment.OriginRef{
				Point: ret.Point,
				Key:   "return:explicit-any",
			},
			Span: spanFromReadModel(ctx.SourceFile, ret.SourceSpan),
		})
	}
	if ret.UntrustedTopOrigin {
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidencePrecisionBoundary,
			Trust: judgment.EvidenceTrustUnknown,
			Origin: judgment.OriginRef{
				Point: ret.Point,
				Key:   "return:untrusted",
			},
		})
	}
	return judgment.Judgment{
		Code:  judgment.CodeReturn,
		Point: ret.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectReturnValue,
			fmt.Sprintf("return:%d:%d", ret.Point, ret.Index),
		).WithLabel(ret.ExpectedLabel),
		Expected: judgment.NewTypeRef(ret.Expected).WithLabel(ret.ExpectedLabel),
		Actual:   judgment.NewValueRef(ret.ValueHash, ret.EffectiveActualType()).WithLabel(ret.SourceLabel),
		Verdict:  verdict,
		Evidence: evidence,
		Spans: []judgment.SpanRef{
			spanFromReadModel(ctx.SourceFile, ret.SourceSpan),
		},
	}
}
