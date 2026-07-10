package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/compiler/source"
)

func sourceSpanFromFactflow(span factflow.SourceSpan) SourceSpan {
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}

func sourceSpanFromAST(span source.Span) SourceSpan {
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    normalizedEndCol(span.StartLine, span.StartCol, span.EndLine, span.EndCol),
	}
}

func sourceSpanFromBody(span body.SourceSpan) SourceSpan {
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    normalizedEndCol(span.StartLine, span.StartCol, span.EndLine, span.EndCol),
	}
}

func sourceSpanFromBodyRaw(span body.SourceSpan) SourceSpan {
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}

func sourceSpansFromBody(spans []body.SourceSpan) []SourceSpan {
	if len(spans) == 0 {
		return nil
	}
	out := make([]SourceSpan, len(spans))
	for i, span := range spans {
		out[i] = sourceSpanFromBody(span)
	}
	return out
}

func normalizedEndCol(startLine, startCol, endLine, endCol int) int {
	if endLine == startLine && endCol <= startCol {
		return startCol + 1
	}
	return endCol
}
