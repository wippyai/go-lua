package summary

import (
	"testing"

	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestMergeEntryValuesWithAggregatePreservesExplicitSlots(t *testing.T) {
	fixed := EntryValues{
		0: product.FromType(typ.String),
	}
	aggregate := EntryValues{
		0: product.FromType(typ.Number),
		1: product.FromType(typ.Boolean),
	}

	got := mergeEntryValuesWithAggregate(fixed, aggregate)

	if !product.Equal(got[0], fixed[0]) {
		t.Fatalf("slot 0 = %s, want explicit fixed %s", got[0].ProjectValue(), fixed[0].ProjectValue())
	}
	if !product.Equal(got[1], aggregate[1]) {
		t.Fatalf("slot 1 = %s, want aggregate %s", got[1].ProjectValue(), aggregate[1].ProjectValue())
	}
}
