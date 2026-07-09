package service

import (
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/placementplan"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type completedSnapshot struct {
	tag              ResultTag
	bodies           []BodyResultRef
	judgments        []judgment.Judgment
	diagnostics      []diagnostic.Diagnostic
	parseErrors      []diagnostic.Diagnostic
	manifestPath     string
	manifestData     []byte
	placement        placementplan.Plan
	summaries        summary.Snapshot
	summaryDigests   map[summary.SummaryKey]summary.Digest
	diagnosticConfig diagnostics.Config
}

func cloneResultTag(tag ResultTag) ResultTag {
	tag.SourceDigests = cloneMap(tag.SourceDigests)
	tag.BodyVersions = cloneMap(tag.BodyVersions)
	return tag
}

func cloneMap[K comparable, V any](in map[K]V) map[K]V {
	if len(in) == 0 {
		return nil
	}
	out := make(map[K]V, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneJudgments(in []judgment.Judgment) []judgment.Judgment {
	if len(in) == 0 {
		return nil
	}
	out := append([]judgment.Judgment(nil), in...)
	for i := range out {
		out[i].Evidence = append(judgment.EvidenceChain(nil), in[i].Evidence...)
		out[i].Spans = append([]judgment.SpanRef(nil), in[i].Spans...)
	}
	return out
}

func cloneDiagnostics(in []diagnostic.Diagnostic) []diagnostic.Diagnostic {
	if len(in) == 0 {
		return nil
	}
	out := append([]diagnostic.Diagnostic(nil), in...)
	for i := range out {
		out[i].Explanation = diagnostic.NewExplanation(in[i].Explanation.Evidence()...)
		out[i].Labels = append([]diagnostic.Label(nil), in[i].Labels...)
	}
	return out
}

func clonePlacementPlan(in placementplan.Plan) placementplan.Plan {
	out := in
	out.Blockers = append([]placementplan.Blocker(nil), in.Blockers...)
	out.Entries = append([]placementplan.Entry(nil), in.Entries...)
	for i := range out.Entries {
		out.Entries[i].Reasons = append([]placementplan.Reason(nil), in.Entries[i].Reasons...)
		out.Entries[i].Obligations = append([]placementplan.Obligation(nil), in.Entries[i].Obligations...)
		out.Entries[i].Blockers = append([]placementplan.Blocker(nil), in.Entries[i].Blockers...)
		out.Entries[i].Children = append(out.Entries[i].Children[:0:0], in.Entries[i].Children...)
	}
	return out
}
