package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestCheckedSourceSpanRejectsMalformedSignedCoordinates(t *testing.T) {
	max := int(^uint32(0))
	for _, test := range []struct {
		name                string
		startLine, startCol int
		endLine, endCol     int
	}{
		{name: "negative start", startLine: -1, startCol: 1, endLine: 1, endCol: 2},
		{name: "negative end", startLine: 1, startCol: 1, endLine: -1, endCol: 2},
		{name: "overflow start", startLine: max + 1, startCol: 1, endLine: 1, endCol: 2},
		{name: "overflow end", startLine: 1, startCol: 1, endLine: max + 1, endCol: 2},
		{name: "one-sided end", startLine: 1, startCol: 1, endLine: 2},
		{name: "reversed range", startLine: 2, startCol: 1, endLine: 1, endCol: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got, ok := coord.Build("types.lua", test.startLine, test.startCol, test.endLine, test.endCol); ok || got != (source.Span{}) {
				t.Fatalf("coord.Build = %#v/%v, want rejection", got, ok)
			}
		})
	}

	if got, ok := coord.Build("types.lua", 1, 2, 1, 3); !ok || got != (source.Span{
		File: "types.lua", StartLine: 1, StartCol: 2, EndLine: 1, EndCol: 3,
	}) {
		t.Fatalf("coord.Build(valid) = %#v/%v", got, ok)
	}
	if got, ok := coord.Build("types.lua", 0, 0, 0, 0); !ok || got != (source.Span{File: "types.lua"}) {
		t.Fatalf("coord.Build(absent) = %#v/%v", got, ok)
	}
}

func TestWriterSpanFailsClosedOnMalformedASTCoordinates(t *testing.T) {
	const sourceName = "types.lua"
	max := int(^uint32(0))
	writer := &Writer{sourceName: sourceName}

	badNode := &ast.Node{}
	badNode.SetLine(-1)
	badNode.SetColumn(1)
	badNode.SetLastLine(1)
	badNode.SetLastColumn(2)
	if got := writer.span(badNode); got != coord.Invalid(sourceName) {
		t.Fatalf("span(negative AST coordinate) = %#v, want invalid sentinel", got)
	}

	badPosition := ast.Position{File: sourceName, Line: max + 1, Column: 1, EndLine: max + 1, EndColumn: 2}
	if got := writer.nameSpan(badPosition); got != coord.Invalid(sourceName) {
		t.Fatalf("nameSpan(overflow AST coordinate) = %#v, want invalid sentinel", got)
	}
}

func TestMalformedWriterSpanTerminallyRejectsCollector(t *testing.T) {
	const sourceName = "types.lua"
	c := assembly.New(sourceName, 0, bind.GlobalCensus{})
	body := c.Body(source.Span{File: sourceName})
	if body == 0 {
		t.Fatal("failed to create collector body")
	}
	node := &ast.Node{}
	node.SetLine(-1)
	node.SetColumn(1)
	node.SetLastLine(1)
	node.SetLastColumn(2)
	writer := &Writer{sourceName: sourceName}
	if got := c.String(writer.span(node), body, "bad"); got != 0 {
		t.Fatalf("collector accepted malformed writer span as term %v", got)
	}
}

func TestBeginAliasRequiresExactBinderTypeParameterCount(t *testing.T) {
	const sourceName = "alias-count.lua"
	for _, test := range []struct {
		name   string
		mutate func(*ast.TypeDefStmt)
	}{
		{
			name: "missing source parameter",
			mutate: func(def *ast.TypeDefStmt) {
				def.TypeParams = nil
			},
		},
		{
			name: "trailing source parameter",
			mutate: func(def *ast.TypeDefStmt) {
				def.TypeParams = append(def.TypeParams, ast.TypeParamExpr{Name: "Trailing"})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stmts, err := parse.ParseString("type Alias<T> = T", sourceName)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			def := stmts[0].(*ast.TypeDefStmt)
			binding := bind.BindChunk(stmts, typeindex.Table{})
			c := assembly.New(sourceName, 0, binding.GlobalCensus())
			body := c.Body(source.Span{File: sourceName})
			writer := New(nil, c, c, binding, nil, nil, nil, sourceName)
			if err := writer.Predeclare(body, stmts); err != nil {
				t.Fatalf("predeclare: %v", err)
			}
			test.mutate(def)
			if _, err := writer.BeginAlias(def); err == nil {
				t.Fatal("BeginAlias accepted inconsistent binder/source parameter counts")
			}
		})
	}
}
