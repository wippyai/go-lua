package regression

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestZZFixtureDump dumps positioned diagnostics for a fixture main.lua under the
// canonical flow, to pinpoint which expect-error lines lost/gained an error.
// Diagnostic only. Set ZZFIX to the main.lua path.
func TestZZFixtureDump(t *testing.T) {
	path := os.Getenv("ZZFIX")
	if path == "" {
		t.Skip("set ZZFIX")
	}
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res := testutil.Check(string(src), testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	lines := make([]string, 0, len(res.Diagnostics))
	for _, d := range res.Diagnostics {
		lines = append(lines, fmt.Sprintf("L%d:%d [%s] %s", d.Position.Line, d.Position.Column, d.Code.Name(), d.Message))
	}
	sort.Strings(lines)
	for _, l := range lines {
		t.Log(l)
	}
}

// zzprecision_probe maps exactly which gradual-value narrowing forms the canonical
// flow already supports and which it drops, so the cluster-#1 fix targets the real
// gap rather than a guess. Diagnostic only; excluded from the oracle. Run with:
//
//	WIPPY_FLOW=canonical go test ./compiler/check/tests/regression/ -run ZZPrecisionProbe -v
func TestZZPrecisionProbe(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"P1_type_narrows_bare_any_symbol", `
local function run(value: any)
  if type(value) == "number" then
    local n: number = value
    return n
  end
  return 0
end`},
		{"P2_type_narrows_fieldpath_on_any", `
local function run(block: any)
  if type(block.text) == "string" then
    local s: string = block.text
    return s
  end
  return ""
end`},
		{"P3_type_fieldpath_feeds_callarg", `
local function parse(text: string?) return text end
local function run(block: any)
  if type(block.text) == "string" then
    return parse(block.text)
  end
  return nil
end`},
		{"P4_user_predicate_narrows_symbol", `
local function is_num(v) return type(v) == "number" end
local function run(value: any)
  if is_num(value) then
    local n: number = value
    return n
  end
  return 0
end`},
		{"P5_type_narrows_bare_symbol_feeds_callarg", `
local function parse(n: number) return n end
local function run(value: any)
  if type(value) == "number" then
    return parse(value)
  end
  return 0
end`},
		{"P6_presence_narrows_optional_symbol", `
local function run(value: string?)
  if value ~= nil then
    local s: string = value
    return s
  end
  return ""
end`},
		{"P7_scalar_narrowed_record_into_generic_return", `
type V<T> = {ok: true, value: T} | {ok: false}
type C = {id: string, n: number}
local function ok<T>(value: T): V<T> return {ok = true, value = value} end
local function dec(raw: any): V<C>
  if type(raw.id) ~= "string" then return {ok = false} end
  if type(raw.n) ~= "number" then return {ok = false} end
  return ok({id = raw.id, n = raw.n})
end
return dec`},
		{"P8_scalar_narrowed_record_direct_return", `
type C = {id: string, n: number}
local function dec(raw: any): C
  if type(raw.id) ~= "string" then error("x") end
  if type(raw.n) ~= "number" then error("x") end
  return {id = raw.id, n = raw.n}
end
return dec`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := testutil.Check(tc.src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
			if len(res.Errors) == 0 {
				t.Logf("PROBE %s: NO ERROR (narrowing works)", tc.name)
			} else {
				t.Logf("PROBE %s: ERRORS = %v", tc.name, testutil.ErrorMessages(res.Diagnostics))
			}
		})
	}
}
