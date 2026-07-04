package readmodel

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestCallArgumentExpectedHasObjectEntries(t *testing.T) {
	reader := typ.NewInterface("Reader", []typ.Method{
		{Name: "read", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	if callArgumentExpectedHasObjectEntries(reader) {
		t.Fatal("interface expected type must keep the whole object literal as the mismatch subject")
	}
	if !callArgumentExpectedHasObjectEntries(typetable.NewRecord().Field("name", typ.String).Build()) {
		t.Fatal("record expected type should allow object-entry mismatch subjects")
	}
	if !callArgumentExpectedHasObjectEntries(typeexpr.Optional(typ.NewArray(typ.String))) {
		t.Fatal("optional array expected type should allow object-entry mismatch subjects")
	}
	if !callArgumentExpectedHasObjectEntries(typeexpr.Union(reader, typetable.NewMap(typ.String, typ.Number))) {
		t.Fatal("union with an object-entry arm should allow object-entry mismatch subjects")
	}
}
