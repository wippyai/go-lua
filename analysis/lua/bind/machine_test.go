package bind

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func parsedWideFunctions(tb testing.TB, width int) []ast.Stmt {
	tb.Helper()
	var source strings.Builder
	for i := 0; i < width; i++ {
		source.WriteString("local f")
		source.WriteString(strconv.Itoa(i))
		source.WriteString(" = function() end\n")
	}
	stmts, err := parse.ParseString(source.String(), "wide_functions.lua")
	if err != nil {
		tb.Fatal(err)
	}
	return stmts
}

func parsedDeepLookup(tb testing.TB, depth int) ([]ast.Stmt, *ast.LocalAssignStmt, *ast.LocalAssignStmt) {
	tb.Helper()
	var source strings.Builder
	source.WriteString("type Root = number\nlocal x = 1\n")
	for i := 0; i < depth; i++ {
		source.WriteString("do\nlocal y")
		source.WriteString(strconv.Itoa(i))
		source.WriteString(": Root = x\n")
	}
	for i := 0; i < depth; i++ {
		source.WriteString("end\n")
	}
	stmts, err := parse.ParseString(source.String(), "deep_lookup.lua")
	if err != nil {
		tb.Fatal(err)
	}
	root := stmts[1].(*ast.LocalAssignStmt)
	block := stmts[2].(*ast.DoBlockStmt)
	var deepest *ast.LocalAssignStmt
	for i := 0; i < depth; i++ {
		deepest = block.Stmts[0].(*ast.LocalAssignStmt)
		if i+1 < depth {
			block = block.Stmts[1].(*ast.DoBlockStmt)
		}
	}
	return stmts, root, deepest
}

func TestGeneratedSourceScaleRetainsLexicalAndTypeAuthority(t *testing.T) {
	stmts, root, deepest := parsedDeepLookup(t, 256)
	result := BindChunk(stmts, typeindex.Table{})
	rootID := mustLocalAt(t, result, root, 0)
	use := deepest.Exprs[0].(*ast.IdentExpr)
	if got := mustSymbol(t, result, use); got != rootID {
		t.Fatalf("deepest x = %d, want root %d", got, rootID)
	}
	annotation := deepest.Types[0].(*ast.PrimitiveTypeExpr)
	if _, ok := result.PrimitiveTypeRef(annotation); !ok {
		t.Fatal("deepest Root annotation lost type authority")
	}
}

func BenchmarkBindWideFunctionsSource(b *testing.B) {
	for _, width := range []int{512, 1024} {
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			stmts := parsedWideFunctions(b, width)
			result := BindChunk(stmts, typeindex.Table{})
			for i, stmt := range stmts {
				fn := stmt.(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
				if _, ok := result.FunctionOrigin(fn); !ok {
					b.Fatalf("function %d origin missing", i)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				BindChunk(stmts, typeindex.Table{})
			}
		})
	}
}

func BenchmarkBindDeepLookupSource(b *testing.B) {
	for _, depth := range []int{512, 1024} {
		b.Run(strconv.Itoa(depth), func(b *testing.B) {
			stmts, root, deepest := parsedDeepLookup(b, depth)
			result := BindChunk(stmts, typeindex.Table{})
			rootID, rootOK := result.LocalSymbolAt(root, 0)
			got, gotOK := result.SymbolOf(deepest.Exprs[0].(*ast.IdentExpr))
			if !rootOK || !gotOK || got != rootID {
				b.Fatalf("deepest x = %d/%v, want %d/%v", got, gotOK, rootID, rootOK)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				BindChunk(stmts, typeindex.Table{})
			}
		})
	}
}

func BenchmarkBindControlWidthSource(b *testing.B) {
	for _, width := range []int{512, 1024} {
		b.Run(strconv.Itoa(width), func(b *testing.B) {
			var source strings.Builder
			for i := 0; i < width; i++ {
				source.WriteString("goto L")
				source.WriteString(strconv.Itoa(i))
				source.WriteString("\n::L")
				source.WriteString(strconv.Itoa(i))
				source.WriteString("::\n")
			}
			stmts, err := parse.ParseString(source.String(), "control_width.lua")
			if err != nil {
				b.Fatal(err)
			}
			if issues := BindChunk(stmts, typeindex.Table{}).ControlIssues(); len(issues) != 0 {
				b.Fatalf("control issues = %#v", issues)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				BindChunk(stmts, typeindex.Table{})
			}
		})
	}
}
