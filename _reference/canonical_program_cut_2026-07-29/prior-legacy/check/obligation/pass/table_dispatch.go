package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// TableDispatches emits exhaustiveness obligations for dispatch tables indexed
// by discriminated-union case keys.
type TableDispatches struct{}

func (TableDispatches) Name() string {
	return "union.table_dispatch.exhaustiveness"
}

func (TableDispatches) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachTableDispatchExhaustiveness(func(item readmodel.TableDispatchExhaustiveness) bool {
		out = append(out, tableDispatchJudgment(ctx, functionKey, item))
		return true
	})
	return out
}

func tableDispatchJudgment(ctx Context, functionKey string, item readmodel.TableDispatchExhaustiveness) judgment.Judgment {
	lookupSpan := spanFromReadModel(ctx.SourceFile, item.LookupSpan)
	tableSpan := spanFromReadModel(ctx.SourceFile, item.TableSpan)
	return judgment.Judgment{
		Code:  judgment.CodeTableDispatch,
		Point: item.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("table-dispatch:%d:%s:%s", item.Point, item.Table, item.Target),
		).WithLabel(item.Table),
		Verdict: judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Detail: judgment.TableDispatchLookupEvidenceDetail(item.Table, item.Target),
				Origin: judgment.OriginRef{
					Point: item.Point,
					Key:   "table-dispatch:lookup",
				},
				Span: lookupSpan,
			},
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Detail: judgment.TableDispatchPossibleEvidenceDetail(discriminatedUnionCaseListKey(item.Possible)),
				Origin: judgment.OriginRef{
					Point: item.Point,
					Key:   "table-dispatch:possible",
				},
				Span: lookupSpan,
			},
			{
				Kind:   judgment.EvidenceAbstractFact,
				Trust:  judgment.EvidenceTrustProven,
				Detail: judgment.TableDispatchKeysEvidenceDetail(discriminatedUnionCaseListKey(item.Keys)),
				Origin: judgment.OriginRef{
					Point: item.Point,
					Key:   "table-dispatch:keys",
				},
				Span: tableSpan,
			},
			{
				Kind:   judgment.EvidenceMissingProof,
				Trust:  judgment.EvidenceTrustUnknown,
				Detail: judgment.TableDispatchMissingEvidenceDetail(registrationMissingCasesKey(item.Missing, item.MissingFor)),
				Origin: judgment.OriginRef{
					Point: item.Point,
					Key:   "table-dispatch:missing",
				},
				Span: lookupSpan,
			},
		},
		Spans: []judgment.SpanRef{lookupSpan, tableSpan},
	}
}
