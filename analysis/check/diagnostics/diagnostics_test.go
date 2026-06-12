package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestAnnotationAssignabilityReportsLiteralMismatch(t *testing.T) {
	diags := runDiagnostics(t, `local x: number = "no"`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot assign") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Explanation.Evidence()) < 2 {
		t.Fatalf("explanation evidence = %#v, want source and annotation evidence", d.Explanation.Evidence())
	}
}

func TestAnnotationAssignabilityAcceptsSubtypeLiteral(t *testing.T) {
	diags := runDiagnostics(t, `local x: number = 42`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestAnnotationAssignabilityDoesNotTrustCastEscape(t *testing.T) {
	diags := runDiagnostics(t, `local x: number = "no" as any`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if got := diags[0].Explanation.String(); !strings.Contains(got, "source expression") {
		t.Fatalf("explanation = %q, want source evidence", got)
	}
}

func TestAnnotationAssignabilitySkipsNonLiteralSources(t *testing.T) {
	diags := runDiagnostics(t, `
		local y = value
		local x: number = y
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for non-literal source", diags)
	}
}

func TestMemberCallReportsMissingMethodAfterDiscriminantNarrowing(t *testing.T) {
	diags := runDiagnostics(t, `
		type Dog = {kind: "dog", bark: () -> ()}
		type Cat = {kind: "cat", meow: () -> ()}
		type Animal = Dog | Cat

		local function speak(a: Animal)
			if a.kind == "dog" then
				a.meow()
			end
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeMissingMember || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "meow") || !strings.Contains(d.Message, "dog") {
		t.Fatalf("message = %q, want missing meow on narrowed dog variant", d.Message)
	}
	if len(d.Explanation.Evidence()) == 0 || d.Explanation.String() == "" {
		t.Fatalf("explanation = %#v, want non-empty evidence", d.Explanation)
	}
}

func TestMemberCallAcceptsMatchingDiscriminantMethod(t *testing.T) {
	diags := runDiagnostics(t, `
		type Dog = {kind: "dog", bark: () -> ()}
		type Cat = {kind: "cat", meow: () -> ()}
		type Animal = Dog | Cat

		local function speak(a: Animal)
			if a.kind == "dog" then
				a.bark()
			else
				a.meow()
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestMemberCallSkipsUnreachableDiscriminantBranch(t *testing.T) {
	diags := runDiagnostics(t, `
		type Dog = {kind: "dog", bark: () -> ()}

		local function speak(a: Dog)
			if a.kind == "cat" then
				a.meow()
			end
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for unreachable branch body", diags)
	}
}

func TestMemberCallSkipsUnnarrowedPrimitiveMethod(t *testing.T) {
	diags := runDiagnostics(t, `
		local value: string = "abc"
		value.upper()
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want no discriminant-member diagnostic without narrowing proof", diags)
	}
}

func TestDirectCallReportsNonCallableTarget(t *testing.T) {
	diags := runDiagnostics(t, `
		local x: number = 42
		x()
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallNotCallable || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "not callable") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Explanation.Evidence()) == 0 {
		t.Fatalf("explanation evidence = %#v, want non-empty", d.Explanation.Evidence())
	}
}

func TestDirectCallReportsTooFewArgs(t *testing.T) {
	diags := runDiagnostics(t, `
		local function add(a: number, b: number): number
			return a
		end
		add(1)
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallTooFewArgs || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "expects 2 arguments") || !strings.Contains(d.Message, "got 1") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Explanation.Evidence()) < 2 {
		t.Fatalf("explanation evidence = %#v, want call and declaration evidence", d.Explanation.Evidence())
	}
}

func TestDirectCallReportsWrongArgumentType(t *testing.T) {
	diags := runDiagnostics(t, `
		local function add(a: number, b: number): number
			return a
		end
		add(1, "wrong")
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "argument 2") || !strings.Contains(d.Message, `"wrong"`) || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	if len(d.Explanation.Evidence()) < 2 {
		t.Fatalf("explanation evidence = %#v, want call and parameter evidence", d.Explanation.Evidence())
	}
}

func TestDirectCallSkipsGenericIdentityArgumentClaims(t *testing.T) {
	diags := runDiagnostics(t, `
		local function identity<T>(x: T): T
			return x
		end
		local n: number = identity(42)
		local s: string = identity("hello")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for generic identity call", diags)
	}
}

func TestDirectCallAcceptsTypedOptionalParam(t *testing.T) {
	diags := runDiagnostics(t, `
		local function log(msg: string, level: string?)
		end
		log("hello")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for typed optional param", diags)
	}
}

func TestDirectCallAcceptsUntypedDefaultOptional(t *testing.T) {
	diags := runDiagnostics(t, `
		local function greet(name, greeting)
			local message = greeting or "Hello"
			return message
		end
		greet("World")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for untyped default optional", diags)
	}
}

func TestDirectCallAcceptsMultipleOrDefaults(t *testing.T) {
	diags := runDiagnostics(t, `
		local function pick(a, b, c)
			local left = b or "left"
			local right = c or "right"
			return left, right
		end
		pick("head")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for trailing defaults", diags)
	}
}

func TestDirectCallAcceptsExplicitNilCheckOptional(t *testing.T) {
	diags := runDiagnostics(t, `
		local function maybe(value: string?)
			if value == nil then
				return
			end
			return value
		end
		maybe()
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for optional nil-checked param", diags)
	}
}

func TestReturnContractReportsLiteralMismatch(t *testing.T) {
	fn := mustFunctionExpr(t, `function f(): number return "hello" end`)
	result, err := check.CheckFunction(fn, check.Config{Registry: product.DefaultRegistry()})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	diags := Produce(result, Config{Registry: product.DefaultRegistry()})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeReturnContractType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "returned value") || !strings.Contains(d.Message, "hello") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	wantReturn := fn.Stmts[0].(*ast.ReturnStmt)
	if got := d.Explanation.Evidence(); len(got) != 2 {
		t.Fatalf("explanation evidence = %#v, want 2 items", got)
	} else {
		if got[0].Span != ast.SpanOf(wantReturn.Exprs[0]) {
			t.Fatalf("returned value evidence span = %#v, want %#v", got[0].Span, ast.SpanOf(wantReturn.Exprs[0]))
		}
		if got[1].Span != ast.SpanOf(fn.ReturnTypes[0]) {
			t.Fatalf("declared return evidence span = %#v, want %#v", got[1].Span, ast.SpanOf(fn.ReturnTypes[0]))
		}
	}
}

func TestReturnContractSkipsOptionalUnknownAndGenericReturns(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{name: "optional nil", src: `local function f(): number? return nil end`},
		{name: "unknown", src: `local function f(): unknown return "hello" end`},
		{name: "any", src: `local function f(): any return "hello" end`},
		{name: "generic", src: `local function id<T>(x: T): T return "hello" end`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			diags := runDiagnostics(t, tc.src)
			if len(diags) != 0 {
				t.Fatalf("diagnostics = %#v, want none", diags)
			}
		})
	}
}

func TestDirectCallResultAssignmentReportsAnnotatedLocalMismatch(t *testing.T) {
	src := `
local function add(a: number, b: number): number
	return a + b
end
local x: string = add(1, 2)
`
	diags := runDiagnostics(t, src)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallResultAssignment || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "call result") || !strings.Contains(d.Message, "string") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
	stmts := mustStmts(t, src)
	assign := stmts[1].(*ast.LocalAssignStmt)
	call := assign.Exprs[0].(*ast.FuncCallExpr)
	if got := d.Explanation.Evidence(); len(got) != 2 {
		t.Fatalf("explanation evidence = %#v, want 2 items", got)
	} else {
		if got[0].Span != ast.SpanOf(call) {
			t.Fatalf("call evidence span = %#v, want %#v", got[0].Span, ast.SpanOf(call))
		}
		if got[1].Span != ast.SpanOf(assign.Types[0]) {
			t.Fatalf("declared type evidence span = %#v, want %#v", got[1].Span, ast.SpanOf(assign.Types[0]))
		}
	}
}

func TestDirectCallResultAssignmentSkipsGenericReturnContracts(t *testing.T) {
	diags := runDiagnostics(t, `
		local function id<T>(x: T): T
			return x
		end
		local s: string = id("hello")
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for generic return contract", diags)
	}
}

func TestPrecheckReportsStructuralBreakAndGoto(t *testing.T) {
	cases := []struct {
		name string
		src  string
		code diagnostic.Code
	}{
		{name: "break outside loop", src: `break`, code: CodeBreakOutsideLoop},
		{name: "break in nested function", src: `
			while true do
				local f = function() break end
			end
		`, code: CodeBreakOutsideLoop},
		{name: "goto missing label", src: `goto missing`, code: CodeGotoUndefinedLabel},
		{name: "duplicate label", src: "::dup::\n::dup::", code: CodeDuplicateLabel},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			stmts := mustStmts(t, tc.src)
			diags := Precheck(stmts, Config{Registry: product.DefaultRegistry()})
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
			}
			if diags[0].Code != tc.code {
				t.Fatalf("diagnostic code = %s, want %s", diags[0].Code, tc.code)
			}
		})
	}
}

func TestPrecheckAllowsForwardGotoAcrossNestedBlocks(t *testing.T) {
	cases := []string{
		"goto target\n do\n  local x = 1\n end\n::target::",
		"if true then\n  local x = 1\n end\n goto target\n::target::",
	}
	for _, src := range cases {
		stmts := mustStmts(t, src)
		diags := Precheck(stmts, Config{Registry: product.DefaultRegistry()})
		if len(diags) != 0 {
			t.Fatalf("diagnostics = %#v, want none", diags)
		}
	}
}

func runDiagnostics(t *testing.T, src string) []diagnostic.Diagnostic {
	t.Helper()
	stmts, err := parse.ParseString(src, "diagnostics_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	result, err := check.CheckChunk(stmts, check.Config{Registry: product.DefaultRegistry()})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	return Produce(result, Config{Registry: product.DefaultRegistry()})
}

func mustStmts(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "diagnostics_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return stmts
}

func mustFunctionExpr(t *testing.T, src string) *ast.FunctionExpr {
	t.Helper()
	stmts := mustStmts(t, src)
	if len(stmts) != 1 {
		t.Fatalf("stmts = %d, want 1", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt = %T, want *ast.FuncDefStmt with function", stmts[0])
	}
	return def.Func
}
