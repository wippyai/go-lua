package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
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

func TestAnnotationAssignabilityReportsArrayLiteralElementMismatch(t *testing.T) {
	diags := runDiagnostics(t, `local arr: {number} = {1, "two", 3}`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, `"two"`) || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestAnnotationAssignabilityAcceptsHomogeneousArrayLiteral(t *testing.T) {
	diags := runDiagnostics(t, `local arr: {number} = {1, 2, 3}`)
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

func TestAnnotationAssignabilityReportsScalarOperatorRHS(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{name: "arithmetic", src: `local bad: string = 1 + 2`},
		{name: "relational", src: `local bad: string = 1 < 2`},
		{name: "concat", src: `local bad: number = "a" .. "b"`},
		{name: "logical", src: `local bad: number = true and false`},
		{name: "unary minus", src: `local bad: string = -1`},
		{name: "unary not", src: `local bad: number = not false`},
		{name: "unary len", src: `local bad: string = #"abc"`},
		{name: "unary bitnot", src: `local bad: string = ~1`},
		{name: "cast wrapper", src: `local bad: string = (1 + 2) as number`},
		{name: "non-nil wrapper", src: `local bad: string = (1 + 2)!`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			diags := runDiagnostics(t, tc.src)
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
			}
			d := diags[0]
			if d.Code != CodeAssignmentType || d.Severity != diagnostic.SeverityError {
				t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
			}
			if !strings.Contains(d.Message, "cannot assign") {
				t.Fatalf("message = %q, want assignment mismatch", d.Message)
			}
		})
	}
}

func TestAnnotationAssignabilityPreservesGradualUntypedDynamicMapWrite(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(raw, key: string)
			local map: {[string]: string} = {}
			map[key] = raw
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for unannotated gradual source", diags)
	}
}

func TestAnnotationAssignabilitySkipsUnannotatedIdentifierSources(t *testing.T) {
	diags := runDiagnostics(t, `
		local y = value
		local x: number = y
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for unannotated identifier source", diags)
	}
}

func TestAnnotationAssignabilitySkipsAnnotatedIdentifierWithoutPointProof(t *testing.T) {
	diags := runDiagnostics(t, `
		local x: string? = value
		local s: string = x
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none without point-local source proof", diags)
	}
}

func TestAnnotationAssignabilityReportsMaybeParameterWithoutNarrowing(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(x: string?)
			local y: string = x
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string?") {
		t.Fatalf("diagnostic = %#v, want optional parameter assignment error", d)
	}
}

func TestAnnotationAssignabilityUsesSolvedTypeTestState(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(x: string | number)
			if type(x) == "string" then
				local n: number = x
			else
				local s: string = x
			end
		end
	`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	for _, d := range diags {
		if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign") {
			t.Fatalf("diagnostic = %#v, want assignment mismatch", d)
		}
	}
}

func TestAnnotationAssignabilityUsesTypeIsWrapperErrorBranchState(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {x: number, y: number}
		local function isPoint(x)
			return Point:is(x)
		end
		function validate(data: any)
			local val, err = isPoint(data)
			if err ~= nil then
				local p: Point = val
			end
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "nil") {
		t.Fatalf("diagnostic = %#v, want nil-to-Point assignment error", d)
	}
}

func TestAnnotationAssignabilityUsesDeclaredLocalValueForTypeTestState(t *testing.T) {
	diags := runDiagnostics(t, `
		local y: string | number = 42
		if type(y) == "string" then
			local n: number = y
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string") || !strings.Contains(d.Message, "number") {
		t.Fatalf("diagnostic = %#v, want string-to-number assignment mismatch", d)
	}
}

func TestAnnotationAssignabilityUsesSolvedTypeNotState(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(x: string | number)
			if type(x) ~= "string" then
				local s: string = x
			else
				local n: number = x
			end
		end
	`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	for _, d := range diags {
		if d.Code != CodeAssignmentType || !strings.Contains(d.Message, "cannot assign") {
			t.Fatalf("diagnostic = %#v, want assignment mismatch", d)
		}
	}
}

func TestAnnotationAssignabilityAcceptsAssertedMaybeParameter(t *testing.T) {
	diags := runDiagnosticsWithSignatures(t, `
		function f(x: string?)
			assert(x)
			local y: string = x
		end
	`, signaturelookup.Source{IncludeStdlib: true})
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after assert", diags)
	}
}

func TestAnnotationAssignabilitySkipsRootLiteralIndexProjection(t *testing.T) {
	diags := runDiagnostics(t, `
		local xs: {number} = {1, 2}
		local x: number = xs[1]
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestAnnotationAssignabilityReportsNestedOptionalIndexProjection(t *testing.T) {
	diags := runDiagnostics(t, `
		type Response = {
			result: {
				data: {
					departments: {string}?,
				},
			},
		}
		local response: Response = {
			result = {
				data = {
					departments = {"engineering"},
				},
			},
		}
		local first: string = response.result.data.departments[1]
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string?") {
		t.Fatalf("diagnostic = %#v, want optional nested index assignment error", d)
	}
}

func TestAnnotationAssignabilityReportsNestedOptionalIndexAfterGuardCalls(t *testing.T) {
	diags := runDiagnostics(t, `
		type Response = {
			result: {
				data: {
					departments: {string}?,
				},
			},
		}
		local response: Response = {
			result = {
				data = {
					departments = {"engineering"},
				},
			},
		}
		test.not_nil(response.result.data.departments, "departments required")
		test.eq(type(response.result.data.departments), "table", "departments should be a table")
		local count: number = #response.result.data.departments
		local first: string = response.result.data.departments[1]
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "string?") {
		t.Fatalf("diagnostic = %#v, want optional nested index assignment error", d)
	}
}

func TestAnnotationAssignabilityReportsMissingRequiredField(t *testing.T) {
	diags := runDiagnostics(t, `
		type Point = {x: number, y: number}
		local p: Point = {x = 10}
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeAssignmentType || !strings.Contains(d.Message, "y") {
		t.Fatalf("diagnostic = %#v, want missing required field y", d)
	}
}

func TestLexicalTypeShadowingResolvesNearestVisibleAlias(t *testing.T) {
	diags := runDiagnostics(t, `
		type Value = number
		local a: Value = 10
		if true then
			type Value = string
			local b: Value = "hello"
		end
		local c: Value = 20
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestUnresolvedLexicalTypeReferences(t *testing.T) {
	cases := []struct {
		name string
		src  string
		line int
	}{
		{
			name: "used before definition",
			src: `
local p: Point = {x = 10, y = 20}
type Point = {x: number, y: number}
`,
			line: 2,
		},
		{
			name: "not visible outside block",
			src: `
if true then
	type LocalPoint = {x: number, y: number}
end
local p: LocalPoint = {x = 1, y = 2}
`,
			line: 5,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			diags := runDiagnostics(t, tc.src)
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
			}
			d := diags[0]
			if d.Code != CodeUnresolvedTypeReference || d.Severity != diagnostic.SeverityError {
				t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
			}
			if d.Position.Line != tc.line {
				t.Fatalf("diagnostic line = %d, want %d", d.Position.Line, tc.line)
			}
			if !strings.Contains(d.Message, "unknown type") {
				t.Fatalf("message = %q", d.Message)
			}
		})
	}
}

func TestUnresolvedValueReferencesReportsImplicitGlobalReads(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		local x = missing + known
		missing = 42
		print(known)
	`, []string{"known", "print"})
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeUnresolvedValueReference || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if d.Position.Line != 2 || !strings.Contains(d.Message, "missing") {
		t.Fatalf("diagnostic = %#v, want unresolved read of missing on line 2", d)
	}
}

func TestUnresolvedValueReferencesReportsNestedReads(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		local t = {[key] = {value = source}}
		sink[t[other]] = value
	`, []string{"sink", "value"})
	if len(diags) != 3 {
		t.Fatalf("diagnostics = %d, want 3: %#v", len(diags), diags)
	}
	for _, d := range diags {
		if d.Code != CodeUnresolvedValueReference {
			t.Fatalf("diagnostic code = %s, want %s: %#v", d.Code, CodeUnresolvedValueReference, diags)
		}
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

func TestMemberCallReportsUnionReceiverMissingMethod(t *testing.T) {
	diags := runDiagnostics(t, `
		function f(x: string | number)
			x:upper()
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeMissingMember || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "upper") || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q, want missing upper on string|number receiver", d.Message)
	}
	if len(d.Explanation.Evidence()) == 0 {
		t.Fatalf("explanation evidence = %#v, want non-empty", d.Explanation.Evidence())
	}
}

func TestMemberCallAcceptsUnionReceiverWhenAllAlternativesCallable(t *testing.T) {
	diags := runDiagnostics(t, `
		type Left = {run: () -> string}
		type Right = {run: () -> number}

		function f(x: Left | Right)
			x:run()
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

func TestNumericForReportsStringInit(t *testing.T) {
	diags := runDiagnostics(t, `
		for i = "one", 10 do
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeNumericForOperand || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "initial value") || !strings.Contains(d.Message, `"one"`) {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestNumericForReportsStringLimitAndStep(t *testing.T) {
	diags := runDiagnostics(t, `
		for i = 1, "ten", "one" do
		end
	`)
	if len(diags) != 2 {
		t.Fatalf("diagnostics = %d, want 2: %#v", len(diags), diags)
	}
	if diags[0].Code != CodeNumericForOperand || !strings.Contains(diags[0].Message, "limit") {
		t.Fatalf("first diagnostic = %#v, want limit numeric-for operand", diags[0])
	}
	if diags[1].Code != CodeNumericForOperand || !strings.Contains(diags[1].Message, "step") {
		t.Fatalf("second diagnostic = %#v, want step numeric-for operand", diags[1])
	}
}

func TestNumericForAcceptsNumbersAndDefaultStep(t *testing.T) {
	diags := runDiagnostics(t, `
		for i = 1, 10 do
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
}

func TestNumericForSkipsUnknownAndPartlyNumericUnion(t *testing.T) {
	diags := runDiagnostics(t, `
		local mixed: number | string = value
		for i = value, mixed do
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for unknown and partly numeric union", diags)
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

func TestDirectCallReportsWrongArgumentTypeInNestedReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		local function add(a: number, b: number): number
			return a + b
		end
		local function f(): number
			return add("bad", 2)
		end
		local x = f()
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeDirectCallArgType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "argument 1") || !strings.Contains(d.Message, `"bad"`) || !strings.Contains(d.Message, "number") {
		t.Fatalf("message = %q", d.Message)
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

func TestAnnotationAssignabilityUsesBoundaryProofAfterAssignedTypeCast(t *testing.T) {
	diags := runDiagnosticsWithGlobals(t, `
		type Point = {x: number, y: number}
		local function validate(data: any)
			Point(data)
			local p: {x: number, y: number} = data
			return p
		end
		local function validate_assign(data: any)
			local v = Point(data)
			local p: {x: number, y: number} = data
			return p
		end
		local function expect_point(x)
			return Point(x)
		end
		local function validate_wrapped(data: any)
			expect_point(data)
			local p: {x: number, y: number} = data
			return p
		end
		return validate, validate_assign, validate_wrapped
	`, nil)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after type-cast postcondition", diags)
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
	result, err := check.CheckFunction(fn, check.Config{
		Registry: standard.Registry(),
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	diags := Produce(result)
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

func TestReturnContractReportsProjectedIndexOptional(t *testing.T) {
	diags := runDiagnostics(t, `
		local function pick(xs: {number}, i: integer): number
			return xs[i]
		end
	`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeReturnContractType || !strings.Contains(d.Message, "number?") {
		t.Fatalf("diagnostic = %#v, want return contract optional index error", d)
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

func TestReturnContractReportsGenericDirectCallConcreteMismatch(t *testing.T) {
	src := `
		type Box<T> = {value: T}
		type StringBox = {value: string}
		local function make<T>(value: T): Box<T>
			return {value = value}
		end
		local function build(): StringBox
			return make(true)
		end
	`
	diags := runDiagnostics(t, src)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeReturnContractType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic = %#v, want return contract error", d)
	}
}

func TestReturnContractSkipsUninferredGenericDirectCallReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		type User = {id: string}
		type Result<T> = {ok: true, value: T} | {ok: false, error: string}
		local function invalid<T>(message: string): Result<T>
			return {ok = false, error = message}
		end
		local function decode(): Result<User>
			return invalid("id")
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none for uninferred generic return", diags)
	}
}

func runDiagnostics(t *testing.T, src string) []diagnostic.Diagnostic {
	t.Helper()
	return runDiagnosticsWithGlobals(t, src, []string{"test", "type", "value"})
}

func runDiagnosticsWithGlobals(t *testing.T, src string, globals []string) []diagnostic.Diagnostic {
	t.Helper()
	return runDiagnosticsFull(t, src, globals, signaturelookup.Source{})
}

func runDiagnosticsWithSignatures(t *testing.T, src string, signatures signaturelookup.Source) []diagnostic.Diagnostic {
	t.Helper()
	return runDiagnosticsFull(t, src, []string{"test", "type", "value"}, signatures)
}

func runDiagnosticsFull(t *testing.T, src string, globals []string, signatures signaturelookup.Source) []diagnostic.Diagnostic {
	t.Helper()
	stmts, err := parse.ParseString(src, "diagnostics_test.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	reg := standard.Registry()
	result, err := check.CheckChunk(stmts, check.Config{
		Registry:   reg,
		Globals:    globals,
		Signatures: signatures,
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	return Produce(result)
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
