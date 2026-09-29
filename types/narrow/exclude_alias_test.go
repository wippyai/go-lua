package narrow

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExcludeKindNarrowsAliasUnionMembers(t *testing.T) {
	rec := typ.NewRecord().Field("id", typ.String).Build()
	spec := typ.NewAlias("Spec", typ.NewUnion(typ.String, rec))
	got := ExcludeKind(typ.NewUnion(typ.NewAlias("Name", typ.String), spec), kind.String)
	if !typ.TypeEquals(got, rec) {
		t.Fatalf("excluding string from alias members = %v, want %v", got, rec)
	}
}
