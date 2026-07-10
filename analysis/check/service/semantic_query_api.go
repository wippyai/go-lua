package service

import (
	"context"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/embedding"
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

func (s *BatchSession) SemanticTokens(ctx context.Context, req SemanticTokensRequest) (SemanticTokensResponse, error) {
	snapshot, meta, err := s.selectedResult(ctx, req.Selector)
	if err != nil {
		return SemanticTokensResponse{}, err
	}
	if snapshot.semantic == nil {
		return SemanticTokensResponse{Meta: meta}, nil
	}
	document, source, ok := semanticSource(snapshot.semantic, req.Document, req.ContentDigest)
	if !ok {
		return SemanticTokensResponse{Meta: meta}, nil
	}
	items := make([]SemanticToken, 0, len(snapshot.semantic.tokens))
	for _, item := range snapshot.semantic.tokens {
		if item.Location.Document != document || item.Location.ContentDigest != source.ContentDigest {
			continue
		}
		items = append(items, cloneSemanticToken(item))
	}
	return SemanticTokensResponse{Meta: meta, Tokens: items}, nil
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
	document, source, ok := semanticSource(semantic, req.Document, req.ContentDigest)
	if !ok {
		return PositionLookupResponse{Meta: meta}, nil
	}
	offset, ok := offsetForPosition(source.Content, req.Position)
	if !ok {
		return PositionLookupResponse{Meta: meta}, nil
	}
	response := PositionLookupResponse{Meta: meta, Found: true}
	if body, ok := innermostBody(semantic.bodies, document, source.ContentDigest, offset); ok {
		response.Body = EnclosingBody{ID: body.id, Location: body.location}
	}
	response.SubjectAnchors = anchorsAt(semantic.anchors, document, source.ContentDigest, offset)
	if expr, ok := innermostExpression(semantic.exprs, document, source.ContentDigest, offset); ok && expr.display != "" {
		response.Expression = &ExpressionType{Location: expr.location, Display: expr.display}
	}
	if binder, ok := binderAt(semantic.binders, document, source.ContentDigest, offset); ok {
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
	document, source, ok := semanticSource(snapshot.semantic, req.Document, req.ContentDigest)
	if !ok {
		return DocumentSymbolsResponse{Meta: meta}, nil
	}
	items := make([]DocumentSymbol, 0, len(snapshot.semantic.symbols))
	for _, item := range snapshot.semantic.symbols {
		if item.Location.Document != document || item.Location.ContentDigest != source.ContentDigest {
			continue
		}
		items = append(items, cloneDocumentSymbol(item))
	}
	return DocumentSymbolsResponse{Meta: meta, Symbols: items}, nil
}

func semanticSource(semantic *semanticQuerySnapshot, document embedding.DocumentID, digest embedding.Digest) (embedding.DocumentID, embedding.SourceSnapshot, bool) {
	if semantic == nil {
		return embedding.DocumentID{}, embedding.SourceSnapshot{}, false
	}
	if !document.Valid() {
		document = semantic.entryDocument
	}
	source, ok := semantic.sources[document]
	if !ok || !source.ContentDigest.IsZero() && !digest.IsZero() && source.ContentDigest != digest {
		return embedding.DocumentID{}, embedding.SourceSnapshot{}, false
	}
	return document, source, true
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
			if _, ok := allowed[item.Code]; !ok {
				continue
			}
		}
		items = append(items, cloneRepairAction(item))
	}
	return RepairActionsResponse{Meta: meta, Actions: items}, nil
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

func locationContains(location SourceLocation, document embedding.DocumentID, digest embedding.Digest, offset int) bool {
	return location.Valid() && location.Document == document && location.ContentDigest == digest && offset >= location.ByteSpan.StartByte && offset < location.ByteSpan.EndByte
}

func locationWidth(location SourceLocation) int {
	if !location.Valid() || location.ByteSpan.EndByte < location.ByteSpan.StartByte {
		return int(^uint(0) >> 1)
	}
	return location.ByteSpan.EndByte - location.ByteSpan.StartByte
}

func innermostBody(items []queryBody, document embedding.DocumentID, digest embedding.Digest, offset int) (queryBody, bool) {
	var best queryBody
	bestWidth := int(^uint(0) >> 1)
	for _, item := range items {
		if !locationContains(item.location, document, digest, offset) {
			continue
		}
		if width := locationWidth(item.location); width < bestWidth {
			best, bestWidth = item, width
		}
	}
	return best, bestWidth != int(^uint(0)>>1)
}

func innermostExpression(items []expressionAt, document embedding.DocumentID, digest embedding.Digest, offset int) (expressionAt, bool) {
	var best expressionAt
	bestWidth := int(^uint(0) >> 1)
	for _, item := range items {
		if !locationContains(item.location, document, digest, offset) {
			continue
		}
		width := locationWidth(item.location)
		if width < bestWidth || width == bestWidth && locationLess(item.location, best.location) {
			best, bestWidth = item, width
		}
	}
	return best, bestWidth != int(^uint(0)>>1)
}

func binderAt(items []BinderInfo, document embedding.DocumentID, digest embedding.Digest, offset int) (BinderInfo, bool) {
	var best BinderInfo
	bestLocation := SourceLocation{}
	bestWidth := int(^uint(0) >> 1)
	for _, item := range items {
		locations := append([]SourceLocation{item.Definition}, occurrenceLocations(item.Occurrences)...)
		for _, location := range locations {
			if !locationContains(location, document, digest, offset) {
				continue
			}
			width := locationWidth(location)
			if width < bestWidth || width == bestWidth && locationLess(location, bestLocation) || width == bestWidth && location == bestLocation && item.SymbolID < best.SymbolID {
				best, bestLocation, bestWidth = item, location, width
			}
		}
	}
	return best, bestWidth != int(^uint(0)>>1)
}

func anchorsAt(items []anchoredSubject, document embedding.DocumentID, digest embedding.Digest, offset int) []judgment.SubjectAnchor {
	seen := make(map[string]judgment.SubjectAnchor)
	for _, item := range items {
		if locationContains(item.location, document, digest, offset) {
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
func cloneSemanticToken(item SemanticToken) SemanticToken {
	item.Modifiers = append([]SemanticTokenModifier(nil), item.Modifiers...)
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

func cloneRepairAction(item RepairAction) RepairAction {
	item.Payload.Edits = append([]RepairEdit(nil), item.Payload.Edits...)
	return item
}
