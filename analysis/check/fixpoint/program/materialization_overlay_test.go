package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestMaterializedParamObligationsJoinAlternatingProjections(t *testing.T) {
	reg := standard.Registry()
	stringOrNumber := typevalue.FromType(reg, typ.MaterializeUnion([]typ.Type{typ.String, typ.Number}))
	stringOrBoolean := typevalue.FromType(reg, typ.MaterializeUnion([]typ.Type{typ.String, typ.Boolean}))
	numberOrBoolean := typevalue.FromType(reg, typ.MaterializeUnion([]typ.Type{typ.Number, typ.Boolean}))
	first := []product.Value{stringOrNumber, numberOrBoolean}
	second := []product.Value{stringOrBoolean, stringOrNumber}

	joined, changed := overlayMaterializedParamObligations(reg, first, second)
	if !changed {
		t.Fatal("incomparable projection did not advance the summary join")
	}
	want := []product.Value{
		product.Meet(reg, stringOrNumber, stringOrBoolean),
		product.Meet(reg, numberOrBoolean, stringOrNumber),
	}
	if !paramObligationsEqual(reg, joined, want) {
		t.Fatalf("joined obligations = %#v, want per-slot meet %#v", joined, want)
	}

	joinedSummary := summary.Summary{ParamObligations: joined}
	for name, input := range map[string][]product.Value{"first": first, "second": second} {
		if !summary.LessOrEq(reg, summary.Summary{ParamObligations: input}, joinedSummary) {
			t.Fatalf("%s projection is not below joined summary", name)
		}
		if next, changed := overlayMaterializedParamObligations(reg, joined, input); changed || !paramObligationsEqual(reg, next, joined) {
			t.Fatalf("%s projection changed converged obligations: changed=%v next=%#v", name, changed, next)
		}
	}
}

func TestMaterializedParamObligationsRejectCombinedBottom(t *testing.T) {
	reg := standard.Registry()
	current := []product.Value{typevalue.FromType(reg, typ.String)}
	projected := []product.Value{typevalue.FromType(reg, typ.Number)}
	next, changed := overlayMaterializedParamObligations(reg, current, projected)
	if changed || !paramObligationsEqual(reg, next, current) {
		t.Fatalf("incomparable Meet changed obligations to Bottom: changed=%v next=%#v", changed, next)
	}
}

func TestMaterializedParamObligationsRejectProjectedBottom(t *testing.T) {
	reg := standard.Registry()
	current := []product.Value{typevalue.FromType(reg, typ.String)}
	next, changed := overlayMaterializedParamObligations(reg, current, []product.Value{product.Bottom(reg)})
	if changed || !paramObligationsEqual(reg, next, current) {
		t.Fatalf("Bottom projection changed obligations: changed=%v next=%#v", changed, next)
	}
}
