package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestLocalAssignmentGenericSourceValueMorePreciseStripsLoweredNil(t *testing.T) {
	reg := standard.Registry()
	cache := typevalue.NewCache()
	result := &Result{registry: reg, typeValues: cache}

	lowered := cache.FromTypeWithWitness(reg, typ.MaterializeOptional(typ.String))
	generic := cache.FromTypeWithWitness(reg, typ.String)

	if !result.LocalAssignmentGenericSourceValueMorePrecise(lowered, generic) {
		t.Fatal("generic string source should replace lowered optional string")
	}
	if got := result.PreferredLocalAssignmentSourceValue(lowered, generic); !product.Equal(reg, got, generic) {
		t.Fatalf("PreferredLocalAssignmentSourceValue returned lowered value, want generic")
	}
}

func TestLocalAssignmentGenericSourceValueMorePreciseRejectsUnrelatedType(t *testing.T) {
	reg := standard.Registry()
	cache := typevalue.NewCache()
	result := &Result{registry: reg, typeValues: cache}

	lowered := cache.FromTypeWithWitness(reg, typ.MaterializeOptional(typ.String))
	generic := cache.FromTypeWithWitness(reg, typ.Number)

	if result.LocalAssignmentGenericSourceValueMorePrecise(lowered, generic) {
		t.Fatal("generic number source should not replace lowered optional string")
	}
	if got := result.PreferredLocalAssignmentSourceValue(lowered, generic); !product.Equal(reg, got, lowered) {
		t.Fatalf("PreferredLocalAssignmentSourceValue returned generic value, want lowered")
	}
}
