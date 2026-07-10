package pass

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func contextFunctionKey(ctx Context) string {
	if ctx.FunctionKey != "" {
		return ctx.FunctionKey
	}
	return "body"
}

func produceReaderJudgments[T any](
	ctx Context,
	each func(readmodel.Reader, func(T) bool) bool,
	build func(Context, string, T) judgment.Judgment,
) []judgment.Judgment {
	functionKey := contextFunctionKey(ctx)
	var out []judgment.Judgment
	each(ctx.Reader, func(item T) bool {
		out = append(out, build(ctx, functionKey, item))
		return true
	})
	return out
}

func subjectRef(functionKey string, kind judgment.SubjectKind, key, label string) judgment.SubjectRef {
	return judgment.NewSubjectRef(functionKey, kind, key).WithLabel(label)
}

func abstractEvidence(point cfg.Point, key string, trust judgment.EvidenceTrust, detail judgment.EvidenceDetail, span judgment.SpanRef) judgment.Evidence {
	return judgment.Evidence{
		Kind:   judgment.EvidenceAbstractFact,
		Trust:  trust,
		Origin: judgment.OriginRef{Point: point, Key: key},
		Detail: detail,
		Span:   span,
	}
}

func provenAbstractEvidence(point cfg.Point, key string, detail judgment.EvidenceDetail, span judgment.SpanRef) judgment.Evidence {
	return abstractEvidence(point, key, judgment.EvidenceTrustProven, detail, span)
}
