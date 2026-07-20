package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestInstantiateFormalInputProductUsesCompleteRegisteredProduct(t *testing.T) {
	reg := standard.Registry()
	actual := product.Set(reg, product.Top(), runtimekind.Key,
		runtimekind.Join(runtimekind.Singleton(runtimekind.String), runtimekind.Singleton(runtimekind.Number)))
	constraint := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	constraint = product.Set(reg, constraint, runtimekind.Key, runtimekind.Singleton(runtimekind.String))

	got, err := instantiateFormalInputProduct(reg, actual, constraint)
	if err != nil {
		t.Fatal(err)
	}
	if value := product.PresenceOf(got); !presence.Equal(value, presence.Present()) {
		t.Fatalf("presence = %s, want present", value)
	}
	if value := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(value, runtimekind.Singleton(runtimekind.String)) {
		t.Fatalf("runtime kind = %s, want string", value)
	}

	identity, err := instantiateFormalInputProduct(reg, actual, product.Bottom(reg))
	if err != nil {
		t.Fatal(err)
	}
	if !product.Equal(reg, identity, actual) {
		t.Fatal("Bottom symbolic constraint was not the input identity")
	}
}
