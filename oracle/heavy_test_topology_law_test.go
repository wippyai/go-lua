package oracle

// This file is the misuse-pattern law. A -test.run pattern whose first slash
// element is empty (a subtest pattern such as "/placement/foo$") matches every
// top-level test's name, because an empty regexp matches anything. A top-level
// test that never calls t.Run has no deeper pattern element to be filtered by,
// so its whole body runs regardless of what the rest of the pattern names.
//
// The other laws in this package close that hole per file: a test that loops
// the corpus harness or spawns a `go test` subprocess is decomposed into one
// subtest per independent fixture where the property allows it, or wrapped in
// one named subtest where the property is a single cross-fixture judgment that
// cannot be split. This law is the topology's own regression gate: it reads
// every top-level oracle test's source and asserts that the set reaching such
// work outside a per-fixture subtest is exactly the frozen list below. A new
// top-level test that reaches the corpus harness through a loop, or spawns a
// subprocess, fails here until its body sits behind a subtest.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// heavyTopologyLeaves are the operations the misuse-pattern defect actually
// pays for: one full seal-compile-solve pass through the corpus harness, one
// prefix compile-and-solve, or one `go test` subprocess. A single call to one
// of these, outside a loop, costs one fixture and is not what this law guards;
// only a call reached through a for/range statement counts as iterating.
var heavyTopologyLeaves = map[string]struct{}{
	"corpusHarnessExecuteDetached":        {},
	"corpusHarnessExecute":                {},
	"corpusHarnessExecuteWithPlanCleanup": {},
	"corpusHarnessFixtureRun":             {},
	"analyzeEdgeMatrixPrefix":             {},
}

// heavyTopologyParents is the closed set of top-level oracle tests whose own
// body, or a helper it calls without crossing a per-fixture subtest, loops the
// corpus harness or spawns a `go test` subprocess to answer one cross-fixture
// judgment. Each one gates its whole body behind exactly one subtest named
// "law", so a pattern that does not name that subtest touches none of the
// work below it.
//
// A top-level test is not listed here merely for calling a corpus-harness
// primitive: TestRegionAscentAdmitsAMovedHeadImage and its kind loop a few
// named fixtures with one subtest per fixture, which this law reads as safe on
// its own terms and does not require naming. What earns a listing is reaching
// the loop without that per-fixture boundary in between.
var heavyTopologyParents = map[string]struct{}{
	"TestKnownRedsManifestIsExactOverItsCoveredSets":       {},
	"TestCorpusFixtureAnalysisIsIndependentOfCompileOrder": {},
	"TestCanonicalFrozenCorpusCensusHighWaterMark":         {},
	"TestCorpusCensusHighWaterMarkSample":                  {},
	"TestCanonicalCorpusAcceptanceHighWaterMark":           {},
	"TestSequentialCorpusFixturesRetainNoHeapAfterClose":   {},
}

// heavyTopologyFunc is one package-level function's static shape: whether its
// own body reaches a heavy leaf outside a per-fixture subtest, and which other
// package-level functions it calls the same way. A call made only from inside
// a per-fixture subtest is not recorded at all - whatever it reaches is
// already behind that subtest's own name.
type heavyTopologyFunc struct {
	name      string
	isTest    bool
	direct    bool
	callsSame []string
}

// TestHeavyCorpusWorkSitsBehindExactlyOneDocumentedSubtest is the law.
func TestHeavyCorpusWorkSitsBehindExactlyOneDocumentedSubtest(t *testing.T) {
	repository := architectureBatteryRepositoryRoot(t)
	oracleDir := filepath.Join(repository, "oracle")
	matches, err := filepath.Glob(filepath.Join(oracleDir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("oracle/*.go matched no source file")
	}

	funcs := make(map[string]*heavyTopologyFunc)
	for _, path := range matches {
		if !strings.HasSuffix(path, "_test.go") {
			continue
		}
		if heavyTopologyHasBuildTag(t, path) {
			// A build-tag-gated probe is not part of the default `go test
			// ./oracle` binary the misuse pattern actually runs against.
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for _, decl := range parsed.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn || fn.Recv != nil || fn.Body == nil {
				continue
			}
			funcs[fn.Name.Name] = heavyTopologyAnalyze(fn)
		}
	}

	memo := make(map[string]bool)
	visiting := make(map[string]bool)
	var reaches func(name string) bool
	reaches = func(name string) bool {
		if value, done := memo[name]; done {
			return value
		}
		if visiting[name] {
			// A cycle among package-level test helpers names nothing heavy on
			// its own; break it rather than loop forever.
			return false
		}
		visiting[name] = true
		defer delete(visiting, name)
		fn, known := funcs[name]
		if !known {
			return false
		}
		found := fn.direct
		for _, callee := range fn.callsSame {
			if reaches(callee) {
				found = true
			}
		}
		memo[name] = found
		return found
	}

	found := make(map[string]struct{})
	for name, fn := range funcs {
		if fn.isTest && reaches(name) {
			found[name] = struct{}{}
		}
	}

	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, listed := heavyTopologyParents[name]; !listed {
			t.Errorf("%s loops the corpus harness or spawns a subprocess outside a per-fixture subtest, and is not in heavyTopologyParents; "+
				"wrap its body in one subtest (or split it into one subtest per independent fixture) and add it to the list if the judgment cannot be split", name)
		}
	}
	for name := range heavyTopologyParents {
		fn, exists := funcs[name]
		if !exists || !fn.isTest {
			t.Errorf("heavyTopologyParents names %s, which is not a top-level oracle test", name)
			continue
		}
		if _, stillFound := found[name]; !stillFound {
			t.Errorf("%s no longer loops the corpus harness or spawns a subprocess outside a per-fixture subtest; remove it from heavyTopologyParents", name)
		}
	}
}

// heavyTopologyHasBuildTag reports whether a source file opens with a
// //go:build (or the legacy // +build) constraint, which the parser does not
// retain and the misuse pattern's default `go test ./oracle` binary does not
// compile.
func heavyTopologyHasBuildTag(t *testing.T, path string) bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments|parser.PackageClauseOnly)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	for _, group := range parsed.Comments {
		for _, comment := range group.List {
			text := comment.Text
			if strings.HasPrefix(text, "//go:build") || strings.HasPrefix(text, "// +build") {
				return true
			}
		}
	}
	return false
}

// heavyTopologyAnalyze walks one function body with the stack ast.Inspect
// leaves on a nil visit, which is what lets a heavy call be classified against
// its actual lexical ancestors: the nearest enclosing t.Run literal, if any,
// and whether that literal's own call site sits inside a loop.
func heavyTopologyAnalyze(fn *ast.FuncDecl) *heavyTopologyFunc {
	result := &heavyTopologyFunc{name: fn.Name.Name, isTest: heavyTopologyIsTestFunc(fn)}

	runLiterals := make(map[*ast.FuncLit]bool) // literal -> its Run call site is inside a loop
	var stack []ast.Node

	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		stack = append(stack, node)
		if call, isCall := node.(*ast.CallExpr); isCall {
			if literal, isRun := heavyTopologyRunLiteral(call); isRun {
				runLiterals[literal] = heavyTopologyStackHasLoop(stack)
			}
		}
		return true
	})

	seenCallee := make(map[string]bool)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		stack = append(stack, node)
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		if literal, fine := heavyTopologyNearestRunLiteral(stack, runLiterals); literal != nil && fine {
			return true // reached only from inside a per-fixture subtest: excluded entirely
		}
		if callee, isRunByName := heavyTopologyRunNamedCallee(call); isRunByName {
			// A subtest registered as t.Run(name, someFunc) delegates to a
			// separate declaration rather than a literal nested in this one,
			// so its guard is read off this call site directly: inside a loop
			// is the same per-fixture shape a literal would give it.
			if !heavyTopologyStackHasLoop(stack) && !seenCallee[callee] {
				seenCallee[callee] = true
				result.callsSame = append(result.callsSame, callee)
			}
			return true
		}
		name, isSubprocess := heavyTopologyCalleeName(call)
		if name == "" {
			return true
		}
		if _, isLeaf := heavyTopologyLeaves[name]; isLeaf {
			if isSubprocess || heavyTopologyStackHasLoop(stack) {
				result.direct = true
			}
			return true
		}
		if isSubprocess {
			result.direct = true
			return true
		}
		if !seenCallee[name] {
			seenCallee[name] = true
			result.callsSame = append(result.callsSame, name)
		}
		return true
	})

	return result
}

// heavyTopologyRunLiteral reports whether a call is `<expr>.Run(<name>,
// <func literal>)`, the shape every subtest in this package is registered
// through, and returns the literal that becomes its subtest body.
func heavyTopologyRunLiteral(call *ast.CallExpr) (*ast.FuncLit, bool) {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "Run" || len(call.Args) != 2 {
		return nil, false
	}
	literal, isLiteral := call.Args[1].(*ast.FuncLit)
	if !isLiteral {
		return nil, false
	}
	return literal, true
}

// heavyTopologyRunNamedCallee reports whether a call is `<expr>.Run(<name>,
// <identifier>)`, the shape a subtest takes when it is registered against a
// separately declared function rather than an inline literal.
func heavyTopologyRunNamedCallee(call *ast.CallExpr) (string, bool) {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector || selector.Sel.Name != "Run" || len(call.Args) != 2 {
		return "", false
	}
	ident, isIdent := call.Args[1].(*ast.Ident)
	if !isIdent {
		return "", false
	}
	return ident.Name, true
}

// heavyTopologyNearestRunLiteral finds the innermost Run literal enclosing the
// node currently on top of the stack, and whether that literal's own call site
// was inside a loop (a per-fixture subtest) rather than called once (a single
// named law).
func heavyTopologyNearestRunLiteral(stack []ast.Node, runLiterals map[*ast.FuncLit]bool) (*ast.FuncLit, bool) {
	for index := len(stack) - 1; index >= 0; index-- {
		literal, isLiteral := stack[index].(*ast.FuncLit)
		if !isLiteral {
			continue
		}
		if fine, tracked := runLiterals[literal]; tracked {
			return literal, fine
		}
	}
	return nil, false
}

// heavyTopologyStackHasLoop reports whether any ancestor on the stack is a
// for or range statement.
func heavyTopologyStackHasLoop(stack []ast.Node) bool {
	for _, node := range stack {
		switch node.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			return true
		}
	}
	return false
}

// heavyTopologyCalleeName resolves a call's target to the plain package-local
// name this law tracks: an unqualified identifier for a call within the
// package, or the literal "exec.Command" for a subprocess spawn, which counts
// as heavy regardless of looping since one `go test` invocation is
// multi-second on its own.
func heavyTopologyCalleeName(call *ast.CallExpr) (name string, isSubprocess bool) {
	switch callee := call.Fun.(type) {
	case *ast.Ident:
		return callee.Name, false
	case *ast.SelectorExpr:
		if base, isIdent := callee.X.(*ast.Ident); isIdent && base.Name == "exec" && callee.Sel.Name == "Command" {
			return "exec.Command", true
		}
	}
	return "", false
}

// heavyTopologyIsTestFunc reports whether fn is a top-level Go test function:
// exported name starting with Test, taking exactly one *testing.T parameter.
func heavyTopologyIsTestFunc(fn *ast.FuncDecl) bool {
	if !strings.HasPrefix(fn.Name.Name, "Test") {
		return false
	}
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	field := fn.Type.Params.List[0]
	if len(field.Names) != 1 {
		return false
	}
	star, isStar := field.Type.(*ast.StarExpr)
	if !isStar {
		return false
	}
	selector, isSelector := star.X.(*ast.SelectorExpr)
	if !isSelector {
		return false
	}
	base, isIdent := selector.X.(*ast.Ident)
	return isIdent && base.Name == "testing" && selector.Sel.Name == "T"
}
