package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// Registrations emits exhaustiveness obligations for callback registries that
// dispatch discriminated-union values after registering only some cases.
type Registrations struct{}

func (Registrations) Name() string {
	return "union.registration.exhaustiveness"
}

func (Registrations) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachRegistrationExhaustiveness(func(item readmodel.RegistrationExhaustiveness) bool {
		out = append(out, registrationJudgment(ctx, functionKey, item))
		return true
	})
	return out
}

func registrationJudgment(ctx Context, functionKey string, item readmodel.RegistrationExhaustiveness) judgment.Judgment {
	dispatchSpan := spanFromReadModel(ctx.SourceFile, item.DispatchSpan)
	registrationSpans := registrationSpanRefs(ctx.SourceFile, item)
	registrationSpan := dispatchSpan
	if len(registrationSpans) > 0 {
		registrationSpan = registrationSpans[0]
	}
	spans := make([]judgment.SpanRef, 0, 1+len(registrationSpans))
	spans = append(spans, dispatchSpan)
	spans = append(spans, registrationSpans...)
	evidence := judgment.EvidenceChain{
		{
			Kind:   judgment.EvidenceAbstractFact,
			Trust:  judgment.EvidenceTrustProven,
			Detail: judgment.RegistrationDispatchEvidenceDetail(item.Registry, item.Target),
			Origin: judgment.OriginRef{
				Point: item.Point,
				Key:   "registration:dispatch",
			},
			Span: dispatchSpan,
		},
		{
			Kind:   judgment.EvidenceAbstractFact,
			Trust:  judgment.EvidenceTrustProven,
			Detail: judgment.RegistrationPossibleEvidenceDetail(discriminatedUnionCaseListKey(item.Possible)),
			Origin: judgment.OriginRef{
				Point: item.Point,
				Key:   "registration:possible",
			},
			Span: dispatchSpan,
		},
		{
			Kind:   judgment.EvidenceAbstractFact,
			Trust:  judgment.EvidenceTrustProven,
			Detail: judgment.RegistrationRegisteredEvidenceDetail(discriminatedUnionCaseListKey(item.Registered)),
			Origin: judgment.OriginRef{
				Point: item.Point,
				Key:   "registration:registered",
			},
			Span: registrationSpan,
		},
		{
			Kind:   judgment.EvidenceMissingProof,
			Trust:  judgment.EvidenceTrustUnknown,
			Detail: judgment.RegistrationMissingEvidenceDetail(registrationMissingCasesKey(item.Missing, item.MissingFor)),
			Origin: judgment.OriginRef{
				Point: item.Point,
				Key:   "registration:missing",
			},
			Span: dispatchSpan,
		},
	}
	for i, span := range registrationSpans[1:] {
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Origin: judgment.OriginRef{
				Point: item.Point,
				Key:   fmt.Sprintf("registration:span:%d", i+1),
			},
			Span: span,
		})
	}
	return judgment.Judgment{
		Code:  judgment.CodeRegistration,
		Point: item.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("registration:%d:%s:%s", item.Point, item.Registry, item.Target),
		).WithLabel(item.Registry),
		Verdict:  judgment.VerdictRefuted,
		Evidence: evidence,
		Spans:    spans,
	}
}

func registrationSpanRefs(sourceFile string, item readmodel.RegistrationExhaustiveness) []judgment.SpanRef {
	spans := item.RegistrationSpans
	if len(spans) == 0 && item.RegistrationSpan.Valid() {
		spans = []readmodel.SourceSpan{item.RegistrationSpan}
	}
	out := make([]judgment.SpanRef, 0, len(spans))
	for _, span := range spans {
		if !span.Valid() {
			continue
		}
		out = append(out, spanFromReadModel(sourceFile, span))
	}
	return out
}

func registrationMissingCasesKey(missing, missingFor []string) string {
	if len(missing) != len(missingFor) {
		return discriminatedUnionCaseListKey(missing)
	}
	pairs := make([]string, 0, len(missing))
	for i := range missing {
		pairs = append(pairs, missing[i]+"\x1e"+missingFor[i])
	}
	return discriminatedUnionCaseListKey(pairs)
}
