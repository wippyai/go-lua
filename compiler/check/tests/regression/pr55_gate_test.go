package regression

import (
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
	"testing"
)

func TestPR55AnnotatedGuardedWrapper(t *testing.T) {
	mod := testutil.CheckAndExport(`
local M = {}
type DB = {x: number}
type Provider = {get: () -> (DB?, string?)}
local provider = {} :: Provider
local function get_db(): (DB?, string?)
 local db, err = provider.get()
 if err then return nil, "failed" end
 if not db then return nil, "no db" end
 return db, nil
end
M.get_db = get_db
return M
`, "store", testutil.WithStdlib())
	if mod.HasError() {
		t.Fatal(mod.Errors)
	}
	for _, reassign := range []int{0, 1, 2} {
		source := `local store = require("store")
local get_db: () -> ({x:number}?,string?) = store.get_db
`
		if reassign == 1 {
			source += `get_db = function() return nil, nil end
`
		}
		source += `local function run()
local db, err = get_db()
if err then return end
local x: number = db.x
end
`
		if reassign == 2 {
			source += `get_db = function() return nil, nil end
`
		}
		source += "run()\n"
		result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("store", mod))
		if result.HasError() != (reassign != 0) {
			t.Fatalf("reassigned=%v: %v", reassign, result.Errors)
		}
	}
}

func TestPR55BuiltinIteratorValuePresence(t *testing.T) {
	consts := io.NewManifest("consts")
	consts.SetExport(typ.NewRecord().Field("CATEGORIES", typ.NewTuple(typ.Nil, typ.Nil, typ.Nil)).Build())
	assertions := io.NewManifest("test")
	assertions.SetExport(typ.NewRecord().Field("not_nil", typ.Func().Param("value", typ.Any).OptParam("message", typ.String).Returns(typ.Any).Spec(contract.NewSpec().WithEnsures(constraint.NotNil{Path: constraint.ParamPath(0)})).Build()).Build())
	result := testutil.Check(`
local consts = require("consts")
local test = require("test")
local function run()
 local by_path: {[string]: any} = {}
 local function list(): {{[string]: any}} return {} end
 for _, a in ipairs(list()) do by_path[tostring(a.path)] = a end
 for _, path in ipairs(consts.CATEGORIES) do
  test.not_nil(by_path[path], "category " .. path)
 end
end
return run
`, testutil.WithStdlib(), testutil.WithManifest("consts", consts), testutil.WithManifest("test", assertions))
	if result.HasError() {
		t.Fatal(result.Errors)
	}
}

func TestPR55CastReturnRelation(t *testing.T) {
	for _, bad := range []bool{false, true} {
		failure := `return nil, "failed"`
		if bad {
			failure = `return nil, nil`
		}
		mod := testutil.CheckAndExport(`
local module = {}
function module.get(n: number)
 if n == 0 then `+failure+` end
 return {x=1}, nil
end
return module
`, "module", testutil.WithStdlib())
		if mod.HasError() {
			t.Fatal(mod.Errors)
		}
		result := testutil.Check(`
local module = require("module")
type View = {get: (number) -> ({x:number}?,string?)}
local view = module :: View
local v, err = view.get(0)
if err then return end
local x: number = v.x
`, testutil.WithStdlib(), testutil.WithModule("module", mod))
		if result.HasError() != bad {
			t.Fatalf("bad=%v: %v", bad, result.Errors)
		}
	}
}
