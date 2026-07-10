package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestInstantiateCallFunctionTypeUsesSolvedArgumentType(t *testing.T) {
	result, err := CheckFunction(parseFunction(t, `
function run(): ()
	local out = id("ready")
end`), Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	point, ok := onlyCallPoint(t, result)
	if !ok {
		t.Fatal("call point not found")
	}
	site, ok := result.CallSiteView(point)
	if !ok {
		t.Fatal("call site not found")
	}
	tp := typ.NewTypeParam("T", nil)
	fn := typ.Func().TypeParamRef(tp).Param("value", tp).Returns(tp).Build()
	got := result.InstantiateCallFunctionType(point, site, fn)
	if got.Type == nil || len(got.Type.Returns) != 1 || !typ.TypeEquals(got.Type.Returns[0], typ.LiteralString("ready")) {
		t.Fatalf("instantiated returns = %v, want literal string", got.Type)
	}
}
