package regression

import (
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
	"testing"
)

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
