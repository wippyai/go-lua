package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TestZZLookupExportProbe types the lookup-table-cast modules through the
// canonical flow and dumps the exported status_codes field type, to confirm
// whether the annotated {[number]: string} is lost to an inferred literal-keyed
// record. Read-only diagnostic probe.
func TestZZLookupExportProbe(t *testing.T) {
	dir := "testdata/fixtures/realworld/lookup-table-cast"
	constSrc := readFixtureFile(dir, "constants.lua")
	mapperSrc := readFixtureFile(dir, "mapper.lua")

	constMod := testutil.CheckAndExport(constSrc, "constants")
	mapperMod := testutil.CheckAndExport(mapperSrc, "mapper", testutil.WithModule("constants", constMod))

	exp := mapperMod.Manifest.Export
	t.Logf("mapper export raw kind=%v: %s", exp.Kind(), exp.String())

	rec, ok := unwrap.Alias(exp).(*typ.Record)
	if !ok {
		t.Fatalf("export not a record: %T", unwrap.Alias(exp))
	}
	for _, f := range rec.Fields {
		t.Logf("  field %-20s : kind=%v %s", f.Name, f.Type.Kind(), f.Type.String())
	}
	if sc := rec.GetField("status_codes"); sc != nil {
		st := unwrap.Alias(sc.Type)
		t.Logf("status_codes underlying kind=%v: %s", st.Kind(), st.String())
		if r2, ok := st.(*typ.Record); ok {
			t.Logf("  status_codes is Record: fields=%d hasMap=%v mapKey=%v mapVal=%v",
				len(r2.Fields), r2.HasMapComponent(), keyStr(r2.MapKey), keyStr(r2.MapValue))
			if r2.MapKey != nil {
				t.Logf("  mapKey kind=%v", r2.MapKey.Kind())
			}
		}
		if m2, ok := st.(*typ.Map); ok {
			t.Logf("  status_codes is Map: key=%s val=%s", m2.Key.String(), m2.Value.String())
		}
	}
}

func keyStr(t typ.Type) string {
	if t == nil {
		return "<nil>"
	}
	return t.String()
}
