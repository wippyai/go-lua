package projection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestValueTypeWithPresenceAppliesValuePresence(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	value := product.WithPresence(reg, typeValues.FromType(reg, typ.String), presence.Maybe())

	got, ok := ValueTypeWithPresence(reg, typeValues, value)
	if !ok || !typ.TypeEquals(got, typ.MaterializeOptional(typ.String)) {
		t.Fatalf("ValueTypeWithPresence = %v/%v, want optional string", got, ok)
	}
}

func TestValueTypeWithPresenceRejectsMissingContext(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	value := typeValues.FromType(reg, typ.String)

	if got, ok := ValueTypeWithPresence(nil, typeValues, value); ok || got != nil {
		t.Fatalf("ValueTypeWithPresence(nil registry) = %v/%v, want no type", got, ok)
	}
	if got, ok := ValueTypeWithPresence(reg, nil, value); ok || got != nil {
		t.Fatalf("ValueTypeWithPresence(nil cache) = %v/%v, want no type", got, ok)
	}
}
