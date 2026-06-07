package summary

import (
	"testing"

	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestMergeEntryValuesWithFixedPreservesExplicitSlots(t *testing.T) {
	fixed := EntryValues{
		0: product.FromType(typ.String),
	}
	fallback := EntryValues{
		0: product.FromType(typ.Number),
		1: product.FromType(typ.Boolean),
	}

	got := mergeEntryValuesWithFixed(fixed, fallback)

	if !product.Equal(got[0], fixed[0]) {
		t.Fatalf("slot 0 = %s, want explicit fixed %s", got[0].ProjectValue(), fixed[0].ProjectValue())
	}
	if !product.Equal(got[1], fallback[1]) {
		t.Fatalf("slot 1 = %s, want fallback %s", got[1].ProjectValue(), fallback[1].ProjectValue())
	}
}
