package service

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/embedding"
)

// documentLabel is a display projection only. In particular, file documents
// retain the original path byte-for-byte, which keeps the fixture renderer
// independent of the new semantic identity layer.
func documentLabel(input UnitInput, document embedding.DocumentID) string {
	if label := input.DocumentLabels.Label(document); label != "" {
		return label
	}
	return embedding.DefaultDocumentLabel(document)
}

func documentForLabel(input UnitInput, label string) (embedding.DocumentID, bool) {
	if label == "" {
		return input.EntryDocument, true
	}
	for document, candidate := range input.DocumentLabels {
		if candidate == label {
			return document, true
		}
	}
	return embedding.DocumentID{}, false
}

func bindJudgmentLocations(items []judgment.Judgment, input UnitInput) {
	for itemIndex := range items {
		for spanIndex := range items[itemIndex].Spans {
			span := &items[itemIndex].Spans[spanIndex]
			document, ok := span.Location.Document, span.Location.Document.Valid()
			if !ok {
				document, ok = documentForLabel(input, span.DisplayFile())
			}
			if !ok {
				continue
			}
			snapshot, ok := input.Sources[document]
			if !ok {
				continue
			}
			span.Location = locationForSpan(document, snapshot, span.StartLine, span.StartCol, span.EndLine, span.EndCol)
			span.SetDisplayFileIfEmpty(documentLabel(input, document))
		}
	}
}

func bindDiagnosticLocations(items []diagnostic.Diagnostic, input UnitInput) {
	for itemIndex := range items {
		item := &items[itemIndex]
		document, ok := item.Location.Document, item.Location.Document.Valid()
		if !ok {
			document, ok = documentForLabel(input, item.Position.File)
		}
		if ok {
			if snapshot, exists := input.Sources[document]; exists {
				item.Location = locationForSpan(document, snapshot, item.Span.StartLine, item.Span.StartCol, item.Span.EndLine, item.Span.EndCol)
				if item.Position.File == "" {
					item.Position.File = documentLabel(input, document)
				}
			}
		}
		for labelIndex := range item.Labels {
			label := &item.Labels[labelIndex]
			labelDocument, labelOK := label.Location.Document, label.Location.Document.Valid()
			if !labelOK {
				labelDocument, labelOK = documentForLabel(input, label.DisplayFile())
			}
			if !labelOK {
				continue
			}
			if snapshot, exists := input.Sources[labelDocument]; exists {
				label.Location = locationForSpan(labelDocument, snapshot, label.Span.StartLine, label.Span.StartCol, label.Span.EndLine, label.Span.EndCol)
				label.SetDisplayFileIfEmpty(documentLabel(input, labelDocument))
			}
		}
	}
}

func locationForSpan(document embedding.DocumentID, snapshot embedding.SourceSnapshot, startLine, startColumn, endLine, endColumn int) embedding.SourceLocation {
	start := byteOffset(snapshot.Content, startLine, startColumn)
	end := start
	if endLine > 0 && endColumn > 0 {
		end = byteOffset(snapshot.Content, endLine, endColumn)
		if end < len(snapshot.Content) {
			end++ // Compiler spans have inclusive ends; embedding spans are half-open.
		}
		if end < start {
			end = start
		}
	}
	return embedding.SourceLocation{
		Document:      document,
		ContentDigest: snapshot.ContentDigest,
		Span:          embedding.ByteSpan{StartByte: start, EndByte: end},
		StartLine:     startLine,
		StartColumn:   startColumn,
		EndLine:       endLine,
		EndColumn:     endColumn,
	}
}

// byteOffset projects the checker's 1-indexed byte columns onto the exact
// source snapshot. LSP UTF encodings are intentionally a host/frontend task.
func byteOffset(content []byte, line, column int) int {
	if line <= 0 || column <= 0 {
		return 0
	}
	offset := 0
	for current := 1; current < line && offset < len(content); current++ {
		for offset < len(content) && content[offset] != '\n' {
			offset++
		}
		if offset < len(content) {
			offset++
		}
	}
	offset += column - 1
	if offset > len(content) {
		return len(content)
	}
	return offset
}
