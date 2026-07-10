package service

import (
	"context"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/compiler/source"
)

func (s *BatchSession) BinderOccurrences(ctx context.Context, req BinderOccurrencesRequest) (BinderOccurrencesResponse, error) {
	snapshot, meta, err := s.selectedResult(ctx, req.Selector)
	if err != nil {
		return BinderOccurrencesResponse{}, err
	}
	if snapshot.semantic == nil {
		return BinderOccurrencesResponse{Meta: meta}, nil
	}
	return BinderOccurrencesResponse{Meta: meta, Binders: cloneBinderInfos(snapshot.semantic.binders)}, nil
}

func (s *BatchSession) PositionLookup(ctx context.Context, req PositionLookupRequest) (PositionLookupResponse, error) {
	snapshot, meta, err := s.selectedResult(ctx, req.Selector)
	if err != nil {
		return PositionLookupResponse{}, err
	}
	semantic := snapshot.semantic
	if semantic == nil {
		return PositionLookupResponse{Meta: meta}, nil
	}
	file := req.File
	if file == "" {
		file = semantic.entryFile
	}
	data, ok := semantic.sources[file]
	if !ok {
		return PositionLookupResponse{Meta: meta}, nil
	}
	offset, ok := offsetForPosition(data, req.Position)
	if !ok {
		return PositionLookupResponse{Meta: meta}, nil
	}
	response := PositionLookupResponse{Meta: meta, Found: true}
	if body, ok := innermostBody(semantic.bodies, data, file, offset); ok {
		response.Body = EnclosingBody{ID: body.id, Location: body.location}
	}
	response.SubjectAnchors = anchorsAt(semantic.anchors, data, file, offset)
	if expr, ok := innermostExpression(semantic.exprs, data, file, offset); ok && expr.display != "" {
		response.Expression = &ExpressionType{Location: expr.location, Display: expr.display}
	}
	if binder, ok := binderAt(semantic.binders, data, file, offset); ok {
		copy := cloneBinderInfo(binder)
		response.Binder = &copy
	}
	return response, nil
}

func (s *BatchSession) DocumentSymbols(ctx context.Context, req DocumentSymbolsRequest) (DocumentSymbolsResponse, error) {
	snapshot, meta, err := s.selectedResult(ctx, req.Selector)
	if err != nil {
		return DocumentSymbolsResponse{}, err
	}
	if snapshot.semantic == nil {
		return DocumentSymbolsResponse{Meta: meta}, nil
	}
	file := req.File
	if file == "" {
		file = snapshot.semantic.entryFile
	}
	items := make([]DocumentSymbol, 0, len(snapshot.semantic.symbols))
	for _, item := range snapshot.semantic.symbols {
		if item.Location.File != file {
			continue
		}
		items = append(items, cloneDocumentSymbol(item))
	}
	return DocumentSymbolsResponse{Meta: meta, Symbols: items}, nil
}

func (s *BatchSession) CallRelations(ctx context.Context, req CallRelationsRequest) (CallRelationsResponse, error) {
	snapshot, meta, err := s.selectedResult(ctx, req.Selector)
	if err != nil {
		return CallRelationsResponse{}, err
	}
	if snapshot.semantic == nil {
		return CallRelationsResponse{Meta: meta}, nil
	}
	items := make([]BodyCallRelations, 0, len(snapshot.semantic.calls))
	for _, body := range snapshot.semantic.calls {
		if req.Body != "" && req.Body != body.Body {
			continue
		}
		items = append(items, cloneBodyCallRelations(body))
	}
	return CallRelationsResponse{Meta: meta, Bodies: items}, nil
}

func (s *BatchSession) RepairActions(ctx context.Context, req RepairActionsRequest) (RepairActionsResponse, error) {
	snapshot, meta, err := s.selectedResult(ctx, req.Selector)
	if err != nil {
		return RepairActionsResponse{}, err
	}
	if snapshot.semantic == nil {
		return RepairActionsResponse{Meta: meta}, nil
	}
	allowed := make(map[judgment.Code]struct{}, len(req.Codes))
	for _, code := range req.Codes {
		allowed[code] = struct{}{}
	}
	items := make([]RepairAction, 0, len(snapshot.semantic.repairs))
	for _, item := range snapshot.semantic.repairs {
		if len(allowed) != 0 {
			// RepairAction intentionally does not duplicate judgment code. The
			// action set is therefore selected by reconstructing descriptor
			// membership below from the immutable judgment list.
			continue
		}
		items = append(items, item)
	}
	if len(allowed) != 0 {
		items = repairActionsForCodes(snapshot.semantic.entryFile, snapshot.judgments, allowed)
	}
	return RepairActionsResponse{Meta: meta, Actions: append([]RepairAction(nil), items...)}, nil
}

func offsetForPosition(data []byte, position SourcePosition) (int, bool) {
	if position.Line != 0 || position.Column != 0 {
		return offsetAt(data, position.Line, position.Column)
	}
	if position.Offset < 0 || position.Offset > len(data) {
		return 0, false
	}
	return position.Offset, true
}

func locationContains(data []byte, location SourceLocation, file string, offset int) bool {
	if location.File != file || !location.Valid() {
		return false
	}
	start, end, ok := offsetsForSpan(data, sourceSpan(location.Span))
	return ok && offset >= start && offset < end
}

func sourceSpan(span SourceSpan) source.Span {
	return source.Span{StartLine: span.StartLine, StartCol: span.StartCol, EndLine: span.EndLine, EndCol: span.EndCol}
}

func locationWidth(data []byte, location SourceLocation) int {
	start, end, ok := offsetsForSpan(data, sourceSpan(location.Span))
	if !ok || end < start {
		return int(^uint(0) >> 1)
	}
	return end - start
}

func innermostBody(items []queryBody, data []byte, file string, offset int) (queryBody, bool) {
	var best queryBody
	bestWidth := int(^uint(0) >> 1)
	for _, item := range items {
		if !locationContains(data, item.location, file, offset) {
			continue
		}
		if width := locationWidth(data, item.location); width < bestWidth {
			best, bestWidth = item, width
		}
	}
	return best, bestWidth != int(^uint(0)>>1)
}

func innermostExpression(items []expressionAt, data []byte, file string, offset int) (expressionAt, bool) {
	var best expressionAt
	bestWidth := int(^uint(0) >> 1)
	for _, item := range items {
		if !locationContains(data, item.location, file, offset) {
			continue
		}
		width := locationWidth(data, item.location)
		if width < bestWidth || width == bestWidth && locationLess(item.location, best.location) {
			best, bestWidth = item, width
		}
	}
	return best, bestWidth != int(^uint(0)>>1)
}

func binderAt(items []BinderInfo, data []byte, file string, offset int) (BinderInfo, bool) {
	var best BinderInfo
	bestLocation := SourceLocation{}
	bestWidth := int(^uint(0) >> 1)
	for _, item := range items {
		locations := append([]SourceLocation{item.Definition}, occurrenceLocations(item.Occurrences)...)
		for _, location := range locations {
			if !locationContains(data, location, file, offset) {
				continue
			}
			width := locationWidth(data, location)
			if width < bestWidth || width == bestWidth && locationLess(location, bestLocation) || width == bestWidth && location == bestLocation && item.SymbolID < best.SymbolID {
				best, bestLocation, bestWidth = item, location, width
			}
		}
	}
	return best, bestWidth != int(^uint(0)>>1)
}

func anchorsAt(items []anchoredSubject, data []byte, file string, offset int) []judgment.SubjectAnchor {
	seen := make(map[string]judgment.SubjectAnchor)
	for _, item := range items {
		if locationContains(data, item.location, file, offset) {
			seen[item.anchor.StableKey()] = item.anchor
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]judgment.SubjectAnchor, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func occurrenceLocations(items []BinderOccurrence) []SourceLocation {
	out := make([]SourceLocation, len(items))
	for i, item := range items {
		out[i] = item.Location
	}
	return out
}

func cloneBinderInfos(items []BinderInfo) []BinderInfo {
	out := make([]BinderInfo, len(items))
	for i, item := range items {
		out[i] = cloneBinderInfo(item)
	}
	return out
}
func cloneBinderInfo(item BinderInfo) BinderInfo {
	item.Occurrences = append([]BinderOccurrence(nil), item.Occurrences...)
	return item
}
func cloneDocumentSymbol(item DocumentSymbol) DocumentSymbol {
	item.Children = append([]DocumentSymbol(nil), item.Children...)
	for i := range item.Children {
		item.Children[i] = cloneDocumentSymbol(item.Children[i])
	}
	return item
}
func cloneBodyCallRelations(item BodyCallRelations) BodyCallRelations {
	item.Calls = append([]CallRelation(nil), item.Calls...)
	for i := range item.Calls {
		if item.Calls[i].Callee != nil {
			value := *item.Calls[i].Callee
			item.Calls[i].Callee = &value
		}
	}
	return item
}

func repairActionsForCodes(defaultFile string, items []judgment.Judgment, allowed map[judgment.Code]struct{}) []RepairAction {
	filtered := make([]judgment.Judgment, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.Code]; ok {
			filtered = append(filtered, item)
		}
	}
	return repairActionsFromJudgments(defaultFile, filtered)
}
