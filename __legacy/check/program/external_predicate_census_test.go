package program_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/parse"
)

func runExternalPredicateCensusChunk(t *testing.T, src string) {
	t.Helper()
	stmts, err := parse.ParseString(src, "external_predicate_census_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	if _, err := program.RunChunk(stmts, program.Config{Check: body.Config{
		Registry: standard.Registry(),
		Signatures: signaturelookup.Source{
			IncludeStdlib: true,
		},
	}}); err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
}

// Distilled from app.api:cli (wippy-golua-seam/tests/app, format_bytes): a
// comparison predicate whose operand is an arithmetic expression.
func TestExternalCensusUnsupportedPredicateProducer(t *testing.T) {
	runExternalPredicateCensusChunk(t, `
local n = 1
return n < 2 * 3
`)
}

// Distilled from app.test.process:cancel_receiver_worker
// (wippy-golua-seam/tests/app): a runtime cast over a call result of an
// any-typed callee.
func TestExternalCensusRefinementOutsideCertifiedPredicate(t *testing.T) {
	runExternalPredicateCensusChunk(t, `
local g: any = nil
local pid = g() :: string
return pid
`)
}

// Distilled from app.lib:sigv4 (wippy-golua-seam/tests/app, sign): an or
// fallback whose left operand is a dynamic map index.
func TestExternalCensusStructuralBranchConditionNotExactLeftOperand(t *testing.T) {
	runExternalPredicateCensusChunk(t, `
local values: {[string]: string} = {}
local k = "a"
return values[k] or ""
`)
}

// Distilled from wippy.test:runner (framework/src/test, run_tests): an or
// fallback whose left operand is a field read through a cast-to-any of a
// generic-for loop variable.
func TestExternalCensusValueSourceOperand(t *testing.T) {
	runExternalPredicateCensusChunk(t, `
local function f(tests: any): ()
	for _, entry in ipairs(tests) do
		local meta = (entry :: any).meta or {}
		tostring(meta)
	end
end
return { f = f }
`)
}

func TestExternalCensusLogicalPredicateRHSIsAValueProducer(t *testing.T) {
	for _, src := range []string{
		`
local function key(prefix: string, suffix: string?): string
	return prefix .. (suffix and (":" .. suffix) or "")
end
return { key = key }
`,
		`
local function label(value: string?): string
	return value or ("value:" .. value)
end
return { label = label }
`,
		`
local function label(value: string?): string
	return value and ("value:" .. value) or ""
end
return { label = label }
`,
	} {
		runExternalPredicateCensusChunk(t, src)
	}
}
