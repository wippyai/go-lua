package pass

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// DirectCallArguments emits call-argument type obligations from solved state.
type DirectCallArguments struct{}

func (DirectCallArguments) Name() string {
	return "direct_call.argument_type"
}

func (DirectCallArguments) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}

	var out []judgment.Judgment
	ctx.Reader.ForEachCall(func(call readmodel.CallSite) bool {
		point := call.Point
		for _, report := range call.Reports {
			switch report.Kind {
			case readmodel.CallArgumentReportGenericConflict:
				out = append(out, genericInferenceConflictJudgment(ctx, functionKey, point, report.Argument, report.Conflict))
			case readmodel.CallArgumentReportObligation:
				if report.Argument.CallerOwnedParameter {
					continue
				}
				obligation := report.Obligation
				if obligation.Type == nil {
					continue
				}
				out = append(out, callArgumentJudgment(ctx, functionKey, point, report.Check, obligation.ExpectedLabel))
			}
		}
		return true
	})
	return out
}

func genericInferenceConflictJudgment(
	ctx Context,
	functionKey string,
	point cfg.Point,
	arg readmodel.CallArgument,
	conflict readmodel.CallGenericInferenceConflict,
) judgment.Judgment {
	primary := arg.Span
	if len(conflict.Contributions) > 0 && conflict.Contributions[0].Span.StartLine != 0 {
		primary = conflict.Contributions[0].Span
	}
	evidence := judgment.EvidenceChain{{
		Kind:  judgment.EvidenceUserAssertion,
		Trust: judgment.EvidenceTrustClaimed,
		Origin: judgment.OriginRef{
			Point: point,
			Key:   fmt.Sprintf("arg:%d:generic:%s:consistent", arg.Index, conflict.ParamName),
		},
	}}
	for i := range conflict.Contributions {
		contribution := conflict.Contributions[i]
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Origin: judgment.OriginRef{
				Point: point,
				Key:   fmt.Sprintf("arg:%d:generic:%s:contribution:%d", arg.Index, conflict.ParamName, i),
			},
			Span: spanFromReadModel(ctx.SourceFile, contribution.Span),
		})
	}
	evidence = append(evidence, judgment.Evidence{
		Kind:  judgment.EvidenceMissingProof,
		Trust: judgment.EvidenceTrustRefuted,
		Origin: judgment.OriginRef{
			Point: point,
			Key:   fmt.Sprintf("arg:%d:generic:%s:conflict", arg.Index, conflict.ParamName),
		},
		Detail: judgment.GenericConflictEvidenceDetail(conflict.ParamName),
	})
	return judgment.Judgment{
		Code:  judgment.CodeCallArgType,
		Point: point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectCallArgument,
			fmt.Sprintf("call:%d:arg:%d:generic:%s", point, arg.Index, conflict.ParamName),
		).WithLabel(callArgumentSubjectLabel(arg)),
		Expected: judgment.NewTypeRef(conflict.Contributions[0].Type),
		Actual:   judgment.NewValueRef(0, conflict.Contributions[1].Type),
		Verdict:  judgment.VerdictRefuted,
		Evidence: evidence,
		Spans:    []judgment.SpanRef{spanFromReadModel(ctx.SourceFile, primary)},
	}
}

func callArgumentJudgment(
	ctx Context,
	functionKey string,
	point cfg.Point,
	check readmodel.CallArgumentCheck,
	expectedLabel string,
) judgment.Judgment {
	arg := check.Argument
	want := check.Expected
	if check.ExpectedLabel != "" {
		expectedLabel = check.ExpectedLabel
	}
	verdict := judgment.VerdictUnknown
	if check.ProvenMismatch {
		verdict = judgment.VerdictRefuted
	}
	actual := arg.TypeWithPresence
	actualTrust := judgment.EvidenceTrustUnknown
	if actual != nil {
		actualTrust = judgment.EvidenceTrustProven
	}
	missingProofTrust := judgment.EvidenceTrustUnknown
	if verdict == judgment.VerdictRefuted {
		missingProofTrust = judgment.EvidenceTrustRefuted
	}
	var missingProofDetail judgment.EvidenceDetail
	switch arg.Mismatch.Kind {
	case readmodel.CallArgumentMismatchMissingRequiredField:
		missingProofDetail = judgment.MissingRequiredFieldEvidenceDetail(arg.Mismatch.Field)
	case readmodel.CallArgumentMismatchMayBeNil:
		missingProofDetail = judgment.MayBeNilEvidenceDetail()
	}
	evidence := judgment.EvidenceChain{
		{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: actualTrust,
			Origin: judgment.OriginRef{
				Point: point,
				Key:   fmt.Sprintf("arg:%d:actual", arg.Index),
			},
		},
		{
			Kind:  judgment.EvidenceUserAssertion,
			Trust: judgment.EvidenceTrustClaimed,
			Origin: judgment.OriginRef{
				Point: point,
				Key:   fmt.Sprintf("arg:%d:expected", arg.Index),
			},
			Span: spanFromReadModel(ctx.SourceFile, check.ExpectedSpan),
		},
		{
			Kind:  judgment.EvidenceMissingProof,
			Trust: missingProofTrust,
			Origin: judgment.OriginRef{
				Point: point,
				Key:   fmt.Sprintf("arg:%d:proof", arg.Index),
			},
			Detail: missingProofDetail,
		},
	}
	if check.ExpectedOrigin.HasOrigin {
		evidence[1].Detail = judgment.CallParamObligationEvidenceDetail(
			check.ExpectedOrigin.FunctionName,
			check.ExpectedOrigin.SubjectLabel,
			check.ExpectedOrigin.ProviderLabel,
			check.ExpectedOrigin.MemberParamNumber,
		)
	}
	if arg.UntrustedTopOrigin {
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidencePrecisionBoundary,
			Trust: judgment.EvidenceTrustUnknown,
			Origin: judgment.OriginRef{
				Point: point,
				Key:   fmt.Sprintf("arg:%d:untrusted", arg.Index),
			},
		})
	}
	return judgment.Judgment{
		Code:  judgment.CodeCallArgType,
		Point: point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectCallArgument,
			fmt.Sprintf("call:%d:arg:%d", point, arg.Index),
		).WithLabel(callArgumentSubjectLabel(arg)),
		Expected: judgment.NewTypeRef(want).WithLabel(expectedLabel),
		Actual:   judgment.NewValueRef(arg.ValueHash, actual),
		Verdict:  verdict,
		Evidence: evidence,
		Spans:    []judgment.SpanRef{spanFromReadModel(ctx.SourceFile, arg.Span)},
	}
}

func callArgumentSubjectLabel(arg readmodel.CallArgument) string {
	if arg.Label == "" {
		return ""
	}
	if strings.HasPrefix(arg.Label, "argument ") {
		return arg.Label
	}
	return fmt.Sprintf("argument %d (%s)", arg.Index+1, arg.Label)
}

func spanFromReadModel(file string, span readmodel.SourceSpan) judgment.SpanRef {
	return judgment.SpanRef{
		File:      file,
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}
