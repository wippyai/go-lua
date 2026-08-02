package identityvalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/registry"
)

func TestExactIDReadsOnlySingletonIdentity(t *testing.T) {
	reg := registry.Registry()
	id := identity.LuaFunction(17)
	value := WithExact(reg, product.Top(), id)

	got, ok := ExactID(reg, value)
	if !ok || got != id {
		t.Fatalf("ExactID(witnessed identity) = %v/%v, want %v/true", got, ok, id)
	}
	if !HasExact(reg, value) {
		t.Fatal("HasExact(witnessed identity) = false, want true")
	}

	if got, ok := ExactID(reg, product.Top()); ok || got != (identity.ID{}) {
		t.Fatalf("ExactID(top) = %v/%v, want zero/false", got, ok)
	}
	if got, ok := ExactID(nil, value); ok || got != (identity.ID{}) {
		t.Fatalf("ExactID(nil registry) = %v/%v, want zero/false", got, ok)
	}
	withoutIdentity := axis.NewRegistry().Freeze()
	if got, ok := ExactID(withoutIdentity, product.Bottom(withoutIdentity)); ok || got != (identity.ID{}) {
		t.Fatalf("ExactID(registry without identity) = %v/%v, want zero/false", got, ok)
	}
}

func TestPresentMaterializesPresentIdentityValue(t *testing.T) {
	reg := registry.Registry()
	id := identity.LuaFunction(23)
	value := Present(reg, id)

	if !presence.Equal(product.PresenceOf(value), presence.Present()) {
		t.Fatalf("Present identity presence = %v, want present", product.PresenceOf(value))
	}
	got, ok := ExactID(reg, value)
	if !ok || got != id {
		t.Fatalf("Present identity ExactID = %v/%v, want %v/true", got, ok, id)
	}
}
