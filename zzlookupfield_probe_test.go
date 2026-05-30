package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TestZZLookupFieldProbe dumps the inferred field type of an annotated map
// assigned into a module record field, for int vs string keys, to confirm
// whether the literal-keyed map-component is what reaches the index read.
func TestZZLookupFieldProbe(t *testing.T) {
	srcInt := `
type StatusCodeMap = {[number]: string}
local M = {}
local status_codes: StatusCodeMap = {}
status_codes[400] = "invalid_request"
status_codes[401] = "authentication"
M.status_codes = status_codes
return M
`
	srcStr := `
type ErrorTypeMap = {[string]: string}
local M = {}
local error_types: ErrorTypeMap = {}
error_types["a"] = "invalid_request"
error_types["b"] = "authentication"
M.error_types = error_types
return M
`
	opt := testutil.WithCheckOption(check.WithCanonicalFlow())
	dump := func(label, src, field string) {
		mod := testutil.CheckAndExport(src, "z_"+label, opt)
		exp := unwrap.Alias(mod.Manifest.Export)
		rec, ok := exp.(*typ.Record)
		if !ok {
			t.Logf("%s: export not record: %T", label, exp)
			return
		}
		f := rec.GetField(field)
		if f == nil {
			t.Logf("%s: no field %s", label, field)
			return
		}
		ft := unwrap.Alias(f.Type)
		t.Logf("%s field %s: kind=%v %s", label, field, ft.Kind(), ft.String())
		if r2, ok := ft.(*typ.Record); ok {
			t.Logf("   Record fields=%d hasMap=%v mapKey=%s(kind=%v) mapVal=%s",
				len(r2.Fields), r2.HasMapComponent(), keyStr(r2.MapKey), kindOf(r2.MapKey), keyStr(r2.MapValue))
		}
		if m2, ok := ft.(*typ.Map); ok {
			t.Logf("   Map key=%s(kind=%v) val=%s", m2.Key.String(), m2.Key.Kind(), m2.Value.String())
		}
	}
	dump("int", srcInt, "status_codes")
	dump("str", srcStr, "error_types")
}

func kindOf(t typ.Type) string {
	if t == nil {
		return "<nil>"
	}
	return t.Kind().String()
}
