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

func sourceSpanFromSemantic(span body.SourceSpan) SourceSpan {
	endCol := span.EndCol
	if span.EndLine == span.StartLine && endCol <= span.StartCol {
		endCol = span.StartCol + 1
	}
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    endCol,
	}
}

func sourceSpanFromAST(span source.Span) SourceSpan {
	endCol := span.EndCol
	if span.EndLine == span.StartLine && endCol <= span.StartCol {
		endCol = span.StartCol + 1
	}
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    endCol,
	}
}

func sourceSpanFromBody(span body.SourceSpan) SourceSpan {
	endCol := span.EndCol
	if span.EndLine == span.StartLine && endCol <= span.StartCol {
		endCol = span.StartCol + 1
	}
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    endCol,
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

func sourceSpanValid(span SourceSpan) bool {
	return span.StartLine > 0 && span.StartCol > 0
}
