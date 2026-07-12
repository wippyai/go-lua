package architecture

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Graph.ID is an opaque identity for one in-memory CFG instance. It may guard
// graph-owned caches, but it must never become a semantic identity, summary
// payload, diagnostic, read-model record, or compiler artifact. Keep this
// allowlist deliberately small and function-scoped.
var reviewedRunLocalGraphIDCalls = map[graphIDCallSite]string{
	{
		path:       "analysis/engine/factapply/call_edges.go",
		function:   "resetForGraph",
		expression: "graph.ID()",
	}: "invalidates one transfer closure's graph traversal cache",
	{
		path:       "analysis/ir/dominance/postdominance.go",
		function:   "ID",
		expression: "r.g.ID()",
	}: "preserves opaque graph-instance identity through the reversed adapter",
}

type graphIDCallSite struct {
	path       string
	function   string
	expression string
}

type graphIDViolation struct {
	graphIDCallSite
	line int
}

func TestGraphInstanceIdentityDoesNotLeakIntoSemanticArtifacts(t *testing.T) {
	root := repoRoot(t)
	analysisRoot := filepath.Join(root, "analysis")
	var violations []graphIDViolation
	err := filepath.WalkDir(analysisRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			// Test fixtures may intentionally use raw graph identities to prove
			// instance separation. Only production projections are artifacts.
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		found, err := graphIDCallsInFile(path, rel)
		if err != nil {
			return err
		}
		for _, call := range found {
			if _, ok := reviewedRunLocalGraphIDCalls[call.graphIDCallSite]; !ok {
				violations = append(violations, call)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	sort.Slice(violations, func(i, j int) bool {
		if violations[i].path != violations[j].path {
			return violations[i].path < violations[j].path
		}
		if violations[i].line != violations[j].line {
			return violations[i].line < violations[j].line
		}
		return violations[i].expression < violations[j].expression
	})
	for _, violation := range violations {
		t.Errorf("%s:%d: %s contains unreviewed Graph.ID projection %s; replace it with a stable lexical/artifact identity or add a narrowly justified run-local allowlist entry",
			violation.path, violation.line, violation.function, violation.expression)
	}
}

func graphIDCallsInFile(path, rel string) ([]graphIDViolation, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", rel, err)
	}
	var out []graphIDViolation
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 0 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "ID" || !looksLikeGraphReceiver(selector.X) {
				return true
			}
			expression, err := formatExpression(fileSet, call)
			if err != nil {
				expression = "<unformattable Graph.ID call>"
			}
			out = append(out, graphIDViolation{
				graphIDCallSite: graphIDCallSite{path: rel, function: fn.Name.Name, expression: expression},
				line:            fileSet.Position(call.Pos()).Line,
			})
			return true
		})
	}
	return out, nil
}

func looksLikeGraphReceiver(expression ast.Expr) bool {
	switch receiver := expression.(type) {
	case *ast.Ident:
		return receiver.Name == "graph" || receiver.Name == "g"
	case *ast.SelectorExpr:
		return receiver.Sel.Name == "Graph" || receiver.Sel.Name == "graph" || receiver.Sel.Name == "g"
	case *ast.CallExpr:
		selector, ok := receiver.Fun.(*ast.SelectorExpr)
		return ok && len(receiver.Args) == 0 && selector.Sel.Name == "Graph"
	case *ast.ParenExpr:
		return looksLikeGraphReceiver(receiver.X)
	default:
		return false
	}
}

func formatExpression(fileSet *token.FileSet, expression ast.Expr) (string, error) {
	var out bytes.Buffer
	if err := format.Node(&out, fileSet, expression); err != nil {
		return "", err
	}
	return out.String(), nil
}
