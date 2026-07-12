package sourcevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestStaticIndexValueMatchesCanonicalFullProductProjection(t *testing.T) {
	reg := standard.Registry()
	cache := typevalue.NewCache()
	record := typetable.NewRecord().Field("target_name", typ.String).Build()
	owner := cache.FromTypeWithWitness(reg, record)
	owner = product.WithPresence(reg, owner, presence.Present())
	owner = product.Set(reg, owner, evidence.Key, evidence.ExplicitTop())
	key := typevalue.LiteralString(reg, "target_name")
	want, ok := cache.RuntimeIndex(reg, owner, key)
	if !ok {
		t.Fatal("canonical RuntimeIndex rejected fixture")
	}
	want = InheritTopOriginEvidence(reg, want, owner)
	got, ok := StaticIndexValue(reg, cache, owner, key)
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("static index = %#v/%v, want canonical full product %#v", got, ok, want)
	}
}

func TestStaticIndexValueRejectsContextualOwnersAndKeys(t *testing.T) {
	reg := standard.Registry()
	record := typetable.NewRecord().Field("target_name", typ.String).Build()
	base := typevalue.WithWitness(reg, typevalue.FromType(reg, record), record)
	base = product.WithPresence(reg, base, presence.Present())
	key := typevalue.LiteralString(reg, "target_name")

	tests := []struct {
		name  string
		owner product.Value
		key   product.Value
	}{
		{name: "identity-singleton", owner: product.Set(reg, base, identity.Key, identity.Singleton(identity.ID{Kind: "lua.table", Site: "test", Index: 1})), key: key},
		{name: "optional-owner", owner: product.WithPresence(reg, base, presence.Maybe()), key: key},
		{name: "broad-key", owner: base, key: typevalue.FromType(reg, typ.String)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got, ok := StaticIndexValue(reg, nil, test.owner, test.key); ok || !product.Equal(reg, got, product.Value{}) {
				t.Fatalf("contextual projection accepted: %#v/%v", got, ok)
			}
		})
	}
}
