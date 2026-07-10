package runtimekindof

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestRestrictTypeToRuntimeKindNarrowsAliasedUnion(t *testing.T) {
	record := typetable.NewRecord().Field("id", typ.String).Build()
	alias := typ.NewAlias("AgentRef", typeexpr.Union(typ.String, record))

	got, changed := RestrictTypeToRuntimeKind(alias, runtimekind.Singleton(runtimekind.Table))

	if !changed || !typ.TypeEquals(got, record) {
		t.Fatalf("RestrictTypeToRuntimeKind(alias, table) = %v changed=%v, want %v/true", got, changed, record)
	}
}
