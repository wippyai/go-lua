package regression

import (
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
	"testing"
)

// Minimal shape from /home/wolfy-j/kickside/crm/src/resolution/aliases.lua:89-96.
func TestGuardedQueryRowsExistenceIsBoolean(t *testing.T) {
	dbType := typ.NewInterface("DB", []typ.Method{{Name: "query", Type: typ.Func().Param("self", typ.Self).Param("sql", typ.String).Returns(typ.NewArray(typ.String), typ.NewOptional(typ.LuaError)).Build()}})
	sql := io.NewManifest("sql")
	sql.SetExport(typ.NewInterface("sql", []typ.Method{{Name: "get", Type: typ.Func().Returns(dbType).Build()}}))
	result := testutil.Check(`
local sql = require("sql")
local function record_exists(db: any, crm_id: string, record_id: string): (boolean, string?)
    local rows, qerr = db:query("SELECT record_id")
    if qerr then return false, tostring(qerr) end
    return rows and rows[1] ~= nil, nil
end
record_exists(sql.get(), "crm", "record")
`, testutil.WithStdlib(), testutil.WithManifest("sql", sql))
	if result.HasError() {
		t.Fatalf("guarded query existence must be boolean: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestGuardedQueryRowsDeclaredOptionalStillRequiresGuard(t *testing.T) {
	result := testutil.Check(`
local function record_exists(rows: {string}?): boolean
    return rows and rows[1] ~= nil
end
record_exists(nil)
`, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("a declared optional row set still needs a presence proof")
	}
}
