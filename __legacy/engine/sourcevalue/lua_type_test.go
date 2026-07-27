package sourcevalue

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestLuaTypeNameValueRuntimeKindMatrix(t *testing.T) {
	reg := standard.Registry()
	if got, ok := LuaTypeNameValue(reg, typevalue.NewCache(), product.Bottom(reg)); !ok || !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("LuaTypeNameValue(Bottom) = %#v/%v, want exact Bottom", got, ok)
	}
	for _, tag := range []runtimekind.Tag{
		runtimekind.Nil, runtimekind.Boolean, runtimekind.Number, runtimekind.String,
		runtimekind.Table, runtimekind.Function, runtimekind.Thread, runtimekind.Userdata,
	} {
		t.Run(tag.String(), func(t *testing.T) {
			arg := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(tag))
			got, ok := LuaTypeNameValue(reg, typevalue.NewCache(), arg)
			literal, exact := typevalue.StringLiteralOf(reg, got)
			if !ok || !exact || literal != tag.String() {
				t.Fatalf("LuaTypeNameValue(%s) = %q/%v/%v", tag, literal, ok, exact)
			}
		})
	}

	top, ok := LuaTypeNameValue(reg, typevalue.NewCache(), product.Top())
	topType, exact := typevalue.TypeOf(reg, top)
	if !ok || !exact || !typ.TypeEquals(topType, typ.String) {
		t.Fatalf("LuaTypeNameValue(Top) type = %v/%v/%v, want string", topType, ok, exact)
	}
}

func TestLuaTypeNameValueRecoversKindFromWitness(t *testing.T) {
	reg := standard.Registry()
	arg := typevalue.WithWitness(reg, product.Top(), typ.LiteralString("value"))
	got, ok := LuaTypeNameValue(reg, typevalue.NewCache(), arg)
	literal, exact := typevalue.StringLiteralOf(reg, got)
	if !ok || !exact || literal != "string" {
		t.Fatalf("witness recovery = %q/%v/%v", literal, ok, exact)
	}
}
