package diagnostics

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestReturnContractReportsLiteralMismatch(t *testing.T) {
	fn := mustFunctionExpr(t, `function f(): number return "hello" end`)
	result, err := program.RunFunction(fn, program.Config{
		Check: body.Config{
			Registry: standard.Registry(),
		},
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	diags := Produce(result.RootResult())
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
		if !strings.Contains(got[1].Message, "returned value 1 must satisfy declared return type number") {
			t.Fatalf("declared return evidence = %q, want return-slot contract wording", got[1].Message)
		}
	}
}

func TestReturnContractReportsCalledLocalFunctionLiteralMismatch(t *testing.T) {
	diags := runDiagnostics(t, `
local function parse_count(raw: string): number
    return "bad"
end

return parse_count("10")
`)
	var found bool
	for _, d := range diags {
		if d.Code != CodeReturnContractType {
			continue
		}
		found = true
		if !strings.Contains(d.Message, `returned value 1`) ||
			!strings.Contains(d.Message, `"bad"`) ||
			!strings.Contains(d.Message, "number") {
			t.Fatalf("return contract diagnostic = %#v, want literal string-to-number mismatch", d)
		}
		if got := d.Explanation.String(); !strings.Contains(got, `returned value 1 has literal value "bad"`) ||
			!strings.Contains(got, "returned value 1 must satisfy declared return type number") {
			t.Fatalf("return contract explanation = %q, want literal and declared-return evidence", got)
		}
	}
	if !found {
		t.Fatalf("diagnostics = %#v, want return contract for called local function body", diags)
	}
}

func TestReturnContractDeduplicatesRepeatedCalledLocalFunctionMismatch(t *testing.T) {
	diags := runDiagnostics(t, `
local function parse_count(raw: string): number
    return "bad"
end

local first = parse_count("10")
local second = parse_count("20")
`)
	var count int
	for _, d := range diags {
		if d.Code == CodeReturnContractType {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("diagnostics = %#v, want one deduplicated return contract", diags)
	}
}

func TestReturnContractAcceptsCalledGenericResultConstructor(t *testing.T) {
	diags := runDiagnostics(t, `
type Validation<T> = {ok: true, value: T} | {ok: false, error: string}

local function ok<T>(value: T): Validation<T>
    return {ok = true, value = value}
end

local function read_labels(value): Validation<{string}>
    if value == nil then
        return ok({} :: {string})
    end
    local labels: {string} = {"known"}
    return ok(labels)
end

return read_labels(nil)
`)
	for _, d := range diags {
		if d.Code == CodeReturnContractType {
			t.Fatalf("diagnostics = %#v, want generic result constructor accepted by enclosing return contract", diags)
		}
	}
}

func TestReturnContractReportsProjectedIndexOptional(t *testing.T) {
	src := strings.TrimLeft(`
local function pick(xs: {number}, i: integer): number
    return xs[i]
end
`, "\n")
	diags := runDiagnostics(t, src)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeReturnContractType ||
		!strings.Contains(d.Message, "cannot return xs[i] as returned value 1") ||
		!strings.Contains(d.Message, "may be nil") {
		t.Fatalf("diagnostic = %#v, want return contract optional index error", d)
	}
	if got := diags[0].Explanation.String(); !strings.Contains(got, "returned value 1 (xs[i]) can be number or nil here") ||
		!strings.Contains(got, "returned value 1 (xs[i]) is an indexed read that can miss or read nil") {
		t.Fatalf("explanation = %q, want path-specific optional-index return evidence", got)
	}
	rendered := diagnostic.Render(diags[0], diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"diagnostics_test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.return.contract]: cannot return xs[i] as returned value 1 because it may be nil
 --> diagnostics_test.lua:2:12
  |
2 |     return xs[i]
  |            ↑ returned value

because:
  1. proven: returned value 1 (xs[i]) can be number or nil here
  2. claimed: returned value 1 must satisfy declared return type number
 --> diagnostics_test.lua:1:48
  |
  |                                                ↓ declared return type
1 | local function pick(xs: {number}, i: integer): number
  3. missing proof: returned value 1 (xs[i]) is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared return type here

help: Guard ` + "`xs[i]`" + ` with a nil check, return a default value, or change the return type to accept nil.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
	}
}

func TestReturnContractReportsLocalFromOptionalIndexRead(t *testing.T) {
	diags := runDiagnostics(t, `
local function pick(xs: {number}, i: integer): number
    local value = xs[i]
    return value
end
`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeReturnContractType ||
		!strings.Contains(d.Message, "cannot return value as returned value 1") ||
		!strings.Contains(d.Message, "may be nil") {
		t.Fatalf("diagnostic = %#v, want return contract optional-local error", d)
	}
	if got := diags[0].Explanation.String(); !strings.Contains(got, "returned value 1 (value) can be number or nil here") ||
		!strings.Contains(got, "returned value 1 must satisfy declared return type number") {
		t.Fatalf("explanation = %q, want optional local evidence and declared return evidence", got)
	}
}

func TestReturnContractReportsFlowBackedReturnMismatches(t *testing.T) {
	cases := []struct {
		name string
		src  string
		got  string
	}{
		{
			name: "annotated parameter",
			src: `
				local function f(x: string): number
					return x
				end
			`,
			got: "string",
		},
		{
			name: "parameter field",
			src: `
				type User = {id: string}
				local function f(u: User): number
					return u.id
				end
			`,
			got: "string",
		},
		{
			name: "inferred local",
			src: `
				local function f(): number
					local x = "bad"
					return x
				end
			`,
			got: "\"bad\"",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			diags := runDiagnostics(t, tc.src)
			if len(diags) != 1 {
				t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
			}
			if d := diags[0]; d.Code != CodeReturnContractType ||
				!strings.Contains(d.Message, tc.got) ||
				!strings.Contains(d.Message, "number") {
				t.Fatalf("diagnostic = %#v, want return contract %s-to-number mismatch", d, tc.got)
			}
		})
	}
}

func TestReturnContractAcceptsGuardedOptionalIdentifierReturn(t *testing.T) {
	diags := runDiagnostics(t, `
		local function f(x: string?): string
			if x == nil then
				return ""
			end
			return x
		end
	`)
	if len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none after nil guard", diags)
	}
}

func TestReturnContractDoesNotTrustCastEscape(t *testing.T) {
	diags := runDiagnostics(t, `local function f(): number return "no" as any end`)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want 1: %#v", len(diags), diags)
	}
	if d := diags[0]; d.Code != CodeReturnContractType {
		t.Fatalf("diagnostic = %#v, want return contract error", d)
	}
	if got := diags[0].Explanation.String(); !strings.Contains(got, "user asserted any") ||
		!strings.Contains(got, "returned value 1 comes from any/unknown") ||
		!strings.Contains(got, "no proof on this path shows returned value 1 satisfies the declared return type") {
		t.Fatalf("explanation = %q, want explicit-any claim and missing-proof evidence", got)
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

func TestReturnContractReportsCastArrayIndexWithoutLengthProof(t *testing.T) {
	src := strings.TrimLeft(`
local function f(v: any): number
    return (v :: {number})[1]
end
`, "\n")
	diags := runDiagnostics(t, src)
	if len(diags) != 1 {
		t.Fatalf("diagnostics = %d, want cast-index return contract error: %#v", len(diags), diags)
	}
	d := diags[0]
	if d.Code != CodeReturnContractType || d.Severity != diagnostic.SeverityError {
		t.Fatalf("diagnostic code/severity = %s/%s, want return contract error", d.Code, d.Severity)
	}
	if !strings.Contains(d.Message, "cannot return v[1] as returned value 1") ||
		!strings.Contains(d.Message, "may be nil") {
		t.Fatalf("message = %q, want optional array index return mismatch", d.Message)
	}
	explanation := d.Explanation.String()
	if !strings.Contains(explanation, "returned value 1 (v[1]) is an indexed read that can miss or read nil") ||
		!strings.Contains(explanation, "no proof shows the selected slot satisfies the declared return type here") {
		t.Fatalf("explanation = %q, want indexed-read missing-proof evidence", explanation)
	}
	rendered := diagnostic.Render(d, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"diagnostics_test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.return.contract]: cannot return v[1] as returned value 1 because it may be nil
 --> diagnostics_test.lua:2:12
  |
2 |     return (v :: {number})[1]
  |            ↑ returned value

because:
  1. proven: returned value 1 (v[1]) can be number or nil here
  2. claimed: returned value 1 must satisfy declared return type number
 --> diagnostics_test.lua:1:27
  |
  |                           ↓ declared return type
1 | local function f(v: any): number
  3. missing proof: returned value 1 (v[1]) is an indexed read that can miss or read nil; no proof shows the selected slot satisfies the declared return type here

help: Guard ` + "`v[1]`" + ` with a nil check, return a default value, or change the return type to accept nil.`
	if rendered != want {
		t.Fatalf("rendered diagnostic mismatch (-want +got):\n%s", renderLineDiff(want, rendered))
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
