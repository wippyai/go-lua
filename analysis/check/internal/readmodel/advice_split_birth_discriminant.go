package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// ForEachSplitBirthDiscriminant visits locally born table records whose tag
// field and payload fields are assigned at separate program points before the
// tag field is used structurally as a discriminant.
func (r Reader) ForEachSplitBirthDiscriminant(visit func(SplitBirthDiscriminant) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	return r.result.ForEachSplitBirthDiscriminantOccurrence(func(occ body.SplitBirthDiscriminantOccurrence) bool {
		receiverLabel := r.displayPathCanonical(occ.Receiver)
		tagLabel := r.splitBirthFieldLabel(occ.Receiver, occ.TagField)
		payloads := make([]SplitBirthPayloadWrite, 0, len(occ.PayloadWrites))
		for _, payload := range occ.PayloadWrites {
			payloads = append(payloads, SplitBirthPayloadWrite{
				Point: payload.Point,
				Label: r.splitBirthFieldLabel(occ.Receiver, payload.Field),
				Span:  sourceSpanFromBody(payload.Span),
			})
		}
		return visit(SplitBirthDiscriminant{
			Point:                occ.Point,
			ReceiverLabel:        receiverLabel,
			TagLabel:             tagLabel,
			TagValue:             occ.TagValue,
			BirthPoint:           occ.BirthPoint,
			BirthSpan:            sourceSpanFromBodyRaw(occ.BirthSpan),
			TagWriteSpan:         sourceSpanFromBody(occ.TagWriteSpan),
			PayloadWrites:        payloads,
			DiscriminantUsePoint: occ.DiscriminantUsePoint,
			DiscriminantUseSpan:  sourceSpanFromBody(occ.DiscriminantUseSpan),
		})
	})
}

func (r Reader) splitBirthFieldLabel(receiver pathdom.Path, field string) string {
	base := r.displayPathCanonical(receiver)
	if base == "" {
		return field
	}
	return base + "." + field
}
