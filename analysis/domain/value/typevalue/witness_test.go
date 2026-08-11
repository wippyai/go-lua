package typevalue

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestWitnessOfRequiresExplicitTypeWitness(t *testing.T) {
	reg := registry.Registry()
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.Function))

	if got, ok := TypeOf(reg, value); !ok || !typ.TypeEquals(got, typ.Func().Variadic(typ.Any).Returns(typ.Any).Build()) {
		t.Fatalf("TypeOf(runtime function) = %v/%v, want conservative callable", got, ok)
	}
	if got, ok := WitnessOf(reg, value); ok || got != nil {
		t.Fatalf("WitnessOf(runtime function) = %v/%v, want no explicit witness", got, ok)
	}
	if HasWitness(reg, value) {
		t.Fatal("HasWitness(runtime function) = true, want false")
	}
}

func TestWitnessOfReturnsExplicitTypeWitness(t *testing.T) {
	reg := registry.Registry()
	value := WithWitness(reg, FromType(reg, typ.String), typ.String)

	got, ok := WitnessOf(reg, value)
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("WitnessOf(witnessed string) = %v/%v, want string", got, ok)
	}
	if !HasWitness(reg, value) {
		t.Fatal("HasWitness(witnessed string) = false, want true")
	}
}

func TestStringLiteralOfRequiresExplicitStringLiteralWitness(t *testing.T) {
	reg := registry.Registry()
	value := WithWitness(reg, FromType(reg, typ.LiteralString("#")), typ.LiteralString("#"))

	got, ok := StringLiteralOf(reg, value)
	if !ok || got != "#" {
		t.Fatalf("StringLiteralOf(witnessed #) = %q/%v, want #/true", got, ok)
	}

	plain := WithWitness(reg, FromType(reg, typ.String), typ.String)
	if got, ok := StringLiteralOf(reg, plain); ok || got != "" {
		t.Fatalf("StringLiteralOf(string) = %q/%v, want empty/false", got, ok)
	}

	runtimeOnly := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	runtimeOnly = product.Set(reg, runtimeOnly, runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	if got, ok := StringLiteralOf(reg, runtimeOnly); ok || got != "" {
		t.Fatalf("StringLiteralOf(runtime string) = %q/%v, want empty/false", got, ok)
	}
}
