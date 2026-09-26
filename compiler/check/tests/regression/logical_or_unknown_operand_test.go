package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
)

func exportedFieldReturn(t *testing.T, source, field string) typ.Type {
	t.Helper()
	exported := testutil.CheckAndExport(source, "or_mod", testutil.WithStdlib())
	if exported.HasError() {
		t.Fatalf("unexpected errors: %v", testutil.ErrorMessages(exported.Errors))
	}
	rec, ok := exported.Manifest.Export.(*typ.Record)
	if !ok {
		t.Fatalf("export = %T (%v), want record", exported.Manifest.Export, exported.Manifest.Export)
	}
	f := rec.GetField(field)
	if f == nil {
		t.Fatalf("export has no field %q: %v", field, rec)
	}
	fn, ok := f.Type.(*typ.Function)
	if !ok || len(fn.Returns) == 0 {
		t.Fatalf("field %q = %v, want function with a return", field, f.Type)
	}
	return fn.Returns[0]
}

// A dynamic operand of `or` may be any truthy value, so the expression keeps
// the operand's top type instead of narrowing to the fallback's type.
func TestLogicalOr_DynamicOperandDominatesFallback(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   typ.Type
	}{
		{"unknown param", `
local M = {}
function M.f(r: unknown)
	return r or {}
end
return M
`, typ.Unknown},
		{"unknown local binding", `
local M = {}
function M.f(r: unknown)
	local out = r or {}
	return out
end
return M
`, typ.Unknown},
		{"unknown receiver method result", `
local M = {}
function M.f(c)
	local r = c:execute()
	return r or {}
end
return M
`, typ.Any},
		{"pcall result", `
local M = {}
function M.f(fn)
	local ok, r = pcall(fn)
	return r or {}
end
return M
`, typ.Any},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := exportedFieldReturn(t, tc.source, "f"); !typ.TypeEquals(got, tc.want) {
				t.Fatalf("return = %v, want %v", got, tc.want)
			}
		})
	}
}
