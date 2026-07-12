package program

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

// projectCertifiedContextSummary materializes relation preservation metadata
// only where the exact concrete entry carried the same root refinement. This
// mirrors canonical projectsummary: Bottom and Top are omitted and portable
// boundary axes are projected before publication.
func projectCertifiedContextSummary(reg *axis.Registry, result transformer.SpecializationResult, certificate *relationContextEntryCertificate) (summary.Summary, bool) {
	if reg == nil || certificate == nil || len(certificate.params) != len(certificate.rootRefinements) ||
		len(certificate.captures) != len(certificate.captureRootRefinements) {
		return summary.Summary{}, false
	}
	out := result.Summary.Clone()
	previous := -1
	for _, raw := range result.PreservedParams {
		index := int(raw)
		if index < 0 || index >= len(certificate.params) || index <= previous {
			return summary.Summary{}, false
		}
		previous = index
		if !certificate.rootRefinements[index] {
			continue
		}
		value := certificate.params[index].value
		value, useful := callboundary.ProjectPathRefinementValue(reg, value)
		if !useful {
			continue
		}
		out.NormalReturnFacts.PathRefinements = append(out.NormalReturnFacts.PathRefinements, callboundary.PathValueFact{
			Path: pathdom.NewPlaceholder(index), Value: value,
		})
	}
	previous = -1
	for _, raw := range result.PreservedCaptures {
		index := int(raw)
		if index < 0 || index >= len(certificate.captures) || index <= previous {
			return summary.Summary{}, false
		}
		previous = index
		if !certificate.captureRootRefinements[index] {
			continue
		}
		capture := certificate.captures[index]
		value, useful := callboundary.ProjectPathRefinementValue(reg, capture.value)
		if !useful || capture.symbol == 0 || capture.name == "" || capture.path.IsEmpty() {
			continue
		}
		out.NormalReturnFacts.PathRefinements = append(out.NormalReturnFacts.PathRefinements, callboundary.PathValueFact{
			Path: capture.path, Value: value,
		})
	}
	return summary.Normalize(reg, out), true
}
