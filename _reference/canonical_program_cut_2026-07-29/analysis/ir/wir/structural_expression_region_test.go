package wir

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestStructuralExpressionOwnerRejectsAliasSpellingsDeterministically(t *testing.T) {
	canonical := StructuralExpressionOwner{HasTemp: true, Temp: 7}
	aliases := []StructuralExpressionOwner{
		{HasTemp: true, Temp: 7, Point: 9},
		{HasTemp: false, Temp: 7, Point: 9},
	}
	want := StructuralExpressionRegion{
		Guard: 2, TrueTarget: 3, FalseTarget: 4, Join: 4, RHSOnTrue: true,
		OwnedRHSPoints: []cfg.Point{3},
	}
	other := StructuralExpressionRegion{
		Guard: 5, TrueTarget: 6, FalseTarget: 7, Join: 7, RHSOnTrue: true,
		OwnedRHSPoints: []cfg.Point{6},
	}

	build := func(reverse bool) *Body {
		body := &Body{}
		if reverse {
			body.SetStructuralExpressionRegion(canonical, want)
		}
		for _, alias := range aliases {
			body.SetStructuralExpressionRegion(alias, other)
		}
		if !reverse {
			body.SetStructuralExpressionRegion(canonical, want)
		}
		return body
	}

	for _, body := range []*Body{build(false), build(true)} {
		got, ok := body.StructuralExpressionRegion(canonical)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("canonical region = %#v/%v, want %#v", got, ok, want)
		}
		count := 0
		body.ForEachStructuralExpressionRegion(func(owner StructuralExpressionOwner, _ StructuralExpressionRegion) bool {
			count++
			if owner != canonical {
				t.Fatalf("retained noncanonical owner %#v", owner)
			}
			return true
		})
		if count != 1 {
			t.Fatalf("retained owner count = %d, want 1", count)
		}
	}
}
