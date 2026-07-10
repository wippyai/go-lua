package service

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func (s *BatchSession) selectedResult(ctx context.Context, selector ResultSelector) (*completedSnapshot, QueryMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, QueryMeta{}, err
	}
	s.mu.RLock()
	snapshot, meta, ok := s.resultForSelectorLocked(selector)
	s.mu.RUnlock()
	if !ok {
		return nil, QueryMeta{}, fmt.Errorf("%w: unit=%s profile=%s solve_seq=%d", ErrResultNotFound, selector.UnitID, selector.Profile, selector.SolveSeq)
	}
	return snapshot, meta, nil
}

func (s *BatchSession) ListJudgments(ctx context.Context, req ListJudgmentsRequest) (ListJudgmentsResponse, error) {
	snapshot, meta, err := s.selectedResult(ctx, req.Selector)
	if err != nil {
		return ListJudgmentsResponse{}, err
	}
	codes := codeSet(req.Codes)
	verdicts := verdictSet(req.Verdicts)
	anchors := anchorSet(req.Anchors)
	items := make([]judgment.Judgment, 0, len(snapshot.judgments))
	for _, item := range snapshot.judgments {
		if len(codes) != 0 {
			if _, ok := codes[item.Code]; !ok {
				continue
			}
		}
		if len(verdicts) != 0 {
			if _, ok := verdicts[item.Verdict]; !ok {
				continue
			}
		}
		if len(anchors) != 0 {
			if _, ok := anchors[item.Subject.Anchor.StableKey()]; !ok {
				continue
			}
		}
		if req.Range != nil && !judgmentInRange(item, *req.Range) {
			continue
		}
		items = append(items, item)
	}
	items = cloneJudgments(items)
	return ListJudgmentsResponse{
		Meta:      meta,
		Judgments: items,
		CodeSpecs: codeSpecs(items),
	}, nil
}

func (s *BatchSession) JudgmentsByAnchor(ctx context.Context, req JudgmentsByAnchorRequest) (JudgmentsByAnchorResponse, error) {
	snapshot, meta, err := s.selectedResult(ctx, req.Selector)
	if err != nil {
		return JudgmentsByAnchorResponse{}, err
	}
	codes := codeSet(req.Codes)
	var anchor judgment.SubjectAnchor
	var items []judgment.Judgment
	for _, item := range snapshot.judgments {
		if item.Subject.Anchor.StableKey() != req.AnchorKey {
			continue
		}
		if len(codes) != 0 {
			if _, ok := codes[item.Code]; !ok {
				continue
			}
		}
		if anchor.IsZero() {
			anchor = item.Subject.Anchor
		}
		items = append(items, item)
	}
	return JudgmentsByAnchorResponse{Meta: meta, Anchor: anchor, Judgments: cloneJudgments(items)}, nil
}

func (s *BatchSession) Diagnostics(ctx context.Context, req ListDiagnosticsRequest) (ListDiagnosticsResponse, error) {
	snapshot, meta, err := s.selectedResult(ctx, req.Selector)
	if err != nil {
		return ListDiagnosticsResponse{}, err
	}
	raw := snapshot.judgments
	if req.Range != nil {
		filtered := make([]judgment.Judgment, 0, len(raw))
		for _, item := range raw {
			if judgmentInRange(item, *req.Range) {
				filtered = append(filtered, item)
			}
		}
		raw = filtered
	}

	rendered := snapshot.diagnostics
	if !judgmentPolicyConfigZero(req.JudgmentPolicy) || len(req.DiagnosticPolicy.Rules) != 0 {
		config := snapshot.diagnosticConfig
		if !judgmentPolicyConfigZero(req.JudgmentPolicy) {
			config.Judgment = req.JudgmentPolicy
		}
		if len(req.DiagnosticPolicy.Rules) != 0 {
			config.Policy = cloneDiagnosticPolicy(req.DiagnosticPolicy)
		}
		rendered = diagnostics.RenderJudgments(snapshot.judgments, config)
	}
	if req.Range != nil {
		filtered := make([]diagnostic.Diagnostic, 0, len(rendered))
		for _, item := range rendered {
			if diagnosticInRange(item, *req.Range) {
				filtered = append(filtered, item)
			}
		}
		rendered = filtered
	}
	return ListDiagnosticsResponse{
		Meta:     meta,
		Rendered: cloneDiagnostics(rendered),
		Raw:      cloneJudgments(raw),
	}, nil
}

func (s *BatchSession) ManifestBytes(ctx context.Context, req ExportManifestRequest) (ExportManifestResponse, error) {
	snapshot, meta, err := s.selectedResult(ctx, req.Selector)
	if err != nil {
		return ExportManifestResponse{}, err
	}
	return ExportManifestResponse{
		Meta:   meta,
		Path:   snapshot.manifestPath,
		Digest: snapshot.tag.ManifestDigest,
		Data:   append([]byte(nil), snapshot.manifestData...),
	}, nil
}

func (s *BatchSession) PlacementPlan(ctx context.Context, req PlacementPlanRequest) (PlacementPlanResponse, error) {
	snapshot, meta, err := s.selectedResult(ctx, req.Selector)
	if err != nil {
		return PlacementPlanResponse{}, err
	}
	return PlacementPlanResponse{Meta: meta, Plan: clonePlacementPlan(snapshot.placement)}, nil
}

func (s *BatchSession) SummarySnapshot(ctx context.Context, req SummarySnapshotRequest) (SummarySnapshotResponse, error) {
	snapshot, meta, err := s.selectedResult(ctx, req.Selector)
	if err != nil {
		return SummarySnapshotResponse{}, err
	}
	wanted := make(map[summary.SummaryKey]struct{}, len(req.Keys))
	for _, key := range req.Keys {
		wanted[key] = struct{}{}
	}
	entries := snapshot.summaries.Entries()
	if len(wanted) != 0 {
		filtered := make([]summary.EntrySummary, 0, len(req.Keys))
		for _, entry := range entries {
			if _, ok := wanted[entry.Key]; ok {
				filtered = append(filtered, entry)
			}
		}
		entries = filtered
	}
	digests := make(map[summary.SummaryKey]summary.Digest, len(entries))
	for _, entry := range entries {
		if digest, ok := snapshot.summaryDigests[entry.Key]; ok {
			digests[entry.Key] = digest
		}
	}
	return SummarySnapshotResponse{Meta: meta, Summaries: entries, Digests: digests}, nil
}

func (s *BatchSession) BodyInputDigests(ctx context.Context, req BodyInputDigestsRequest) (BodyInputDigestsResponse, error) {
	snapshot, meta, err := s.selectedResult(ctx, req.Selector)
	if err != nil {
		return BodyInputDigestsResponse{}, err
	}
	return BodyInputDigestsResponse{Meta: meta, Digests: cloneMap(snapshot.tag.BodyInputDigests)}, nil
}

func judgmentPolicyConfigZero(config judgment.PolicyConfig) bool {
	return config.Policy.IsZero() && config.Strictness == ""
}

func codeSet(items []judgment.Code) map[judgment.Code]struct{} {
	if len(items) == 0 {
		return nil
	}
	out := make(map[judgment.Code]struct{}, len(items))
	for _, item := range items {
		out[item] = struct{}{}
	}
	return out
}

func verdictSet(items []judgment.Verdict) map[judgment.Verdict]struct{} {
	if len(items) == 0 {
		return nil
	}
	out := make(map[judgment.Verdict]struct{}, len(items))
	for _, item := range items {
		out[item] = struct{}{}
	}
	return out
}

func anchorSet(items []judgment.SubjectAnchor) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		out[item.StableKey()] = struct{}{}
	}
	return out
}

func codeSpecs(items []judgment.Judgment) []judgment.CodeSpec {
	wanted := make(map[judgment.Code]struct{}, len(items))
	for _, item := range items {
		wanted[item.Code] = struct{}{}
	}
	registry := judgment.DefaultRegistry()
	out := make([]judgment.CodeSpec, 0, len(wanted))
	for _, code := range registry.Codes() {
		if _, ok := wanted[code]; !ok {
			continue
		}
		if spec, ok := registry.Lookup(code); ok {
			out = append(out, spec)
		}
	}
	return out
}

func judgmentInRange(item judgment.Judgment, sourceRange SourceRange) bool {
	for _, span := range item.Spans {
		if sourceRange.Document.Valid() && span.Location.Document != sourceRange.Document {
			continue
		}
		if rangesOverlap(span.StartLine, span.StartCol, span.EndLine, span.EndCol, sourceRange) {
			return true
		}
	}
	return false
}

func diagnosticInRange(item diagnostic.Diagnostic, sourceRange SourceRange) bool {
	if sourceRange.Document.Valid() && item.Location.Document != sourceRange.Document {
		return false
	}
	return rangesOverlap(item.Span.StartLine, item.Span.StartCol, item.Span.EndLine, item.Span.EndCol, sourceRange)
}

func rangesOverlap(startLine, startCol, endLine, endCol int, sourceRange SourceRange) bool {
	if startLine == 0 {
		return false
	}
	if endLine == 0 {
		endLine, endCol = startLine, startCol
	}
	rangeEndLine, rangeEndCol := sourceRange.EndLine, sourceRange.EndCol
	if rangeEndLine == 0 {
		rangeEndLine, rangeEndCol = sourceRange.StartLine, sourceRange.StartCol
	}
	return positionLessOrEqual(startLine, startCol, rangeEndLine, rangeEndCol) &&
		positionLessOrEqual(sourceRange.StartLine, sourceRange.StartCol, endLine, endCol)
}

func positionLessOrEqual(leftLine, leftCol, rightLine, rightCol int) bool {
	return leftLine < rightLine || (leftLine == rightLine && leftCol <= rightCol)
}
