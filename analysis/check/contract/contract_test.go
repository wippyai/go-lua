package contract

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestFromFunctionTypeClassifiesRootTopAsOptionalNonExplicit(t *testing.T) {
	fn := typ.Func().
		Param("raw", typ.Any).
		Param("name", typ.String).
		Returns(typ.Unknown).
		Build()

	c := FromFunctionType(fn)
	raw, ok := c.ParamAt(0)
	if !ok {
		t.Fatal("ParamAt(0) returned false")
	}
	if raw.Explicit || !raw.Optional {
		t.Fatalf("raw param = %+v, want non-explicit optional top boundary", raw)
	}
	name, ok := c.ParamAt(1)
	if !ok {
		t.Fatal("ParamAt(1) returned false")
	}
	if !name.Explicit || name.Optional {
		t.Fatalf("name param = %+v, want explicit required string", name)
	}
	if got := c.RequiredArity(); got != 2 {
		t.Fatalf("RequiredArity = %d, want 2", got)
	}
	if got := c.ParamCount(); got != 2 {
		t.Fatalf("ParamCount = %d, want fixed parameter count 2", got)
	}
	ret, ok := c.ResultAt(0)
	if !ok {
		t.Fatal("ResultAt(0) returned false")
	}
	if ret.Explicit {
		t.Fatalf("unknown return = %+v, want non-explicit", ret)
	}
}

func TestFromFunctionTypeKeepsNestedAnyShapeExplicit(t *testing.T) {
	mapAny := typetable.NewMap(typ.String, typ.Any)
	fn := typ.Func().
		Param("headers", mapAny).
		Build()

	c := FromFunctionType(fn)
	param, ok := c.ParamAt(0)
	if !ok {
		t.Fatal("ParamAt(0) returned false")
	}
	if !param.Explicit || param.Optional {
		t.Fatalf("headers param = %+v, want explicit required map shape", param)
	}
	if param.Type != mapAny {
		t.Fatalf("param type = %v, want original map type", param.Type)
	}
}

func TestParamAcceptedTypeMaterializesOptionality(t *testing.T) {
	fn := typ.Func().
		OptParam("value", typ.String).
		Build()

	c := FromFunctionType(fn)
	param, ok := c.ParamAt(0)
	if !ok {
		t.Fatal("ParamAt(0) returned false")
	}
	if !typ.TypeEquals(param.AcceptedType(), typeexpr.Optional(typ.String)) {
		t.Fatalf("AcceptedType = %s, want string?", param.AcceptedType())
	}
}

func TestFromFunctionTypeVariadicExtendsParamLookup(t *testing.T) {
	fn := typ.Func().
		Param("prefix", typ.String).
		Variadic(typ.Number).
		Build()

	c := FromFunctionType(fn)
	if !c.HasVararg() {
		t.Fatal("HasVararg = false, want true")
	}
	param, ok := c.ParamAt(3)
	if !ok {
		t.Fatal("ParamAt(3) returned false for variadic contract")
	}
	if param.Type != typ.Number || !param.Explicit || param.Optional {
		t.Fatalf("variadic param = %+v, want explicit required number", param)
	}
}
