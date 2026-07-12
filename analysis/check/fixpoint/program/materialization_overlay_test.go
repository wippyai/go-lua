package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestMaterializedProofChangesReportExactPresentationAndSemanticKeys(t *testing.T) {
	reg := standard.Registry()
	buildRecursive := func() typ.Type {
		return typ.NewRecursive("ProofDelta", func(self typ.Type) typ.Type {
			return typetable.NewRecord().OptField("next", self).Build()
		})
	}
	currentSpelling := typevalue.NewCache().FromTypeWithWitness(reg, buildRecursive())
	projectedSpelling := typevalue.NewCache().FromTypeWithWitness(reg, buildRecursive())
	if product.Equal(reg, currentSpelling, projectedSpelling) ||
		!product.LessOrEq(reg, currentSpelling, projectedSpelling) ||
		!product.LessOrEq(reg, projectedSpelling, currentSpelling) {
		t.Fatal("test requires distinct lattice-equivalent recursive spellings")
	}

	presentationKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(6101)))
	semanticKey := summary.DefaultSummaryKey(ref.FromSymbol(symbol.ID(6102)))
	presentationResult := &body.Result{}
	semanticResult := &body.Result{}
	base := summary.NewSnapshot(reg,
		summary.EntrySummary{Key: presentationKey, Summary: summary.Summary{Returns: []product.Value{currentSpelling}}},
		summary.EntrySummary{Key: semanticKey, Summary: summary.Summary{Returns: []product.Value{product.Top()}}},
	)
	materialized := materializedProgram{
		resultKey: map[*body.Result]summary.SummaryKey{
			presentationResult: presentationKey,
			semanticResult:     semanticKey,
		},
		projections: &resultSummaryProjectionCache{entries: map[*body.Result]summary.Summary{
			presentationResult: {Returns: []product.Value{projectedSpelling}},
			semanticResult:     {Returns: []product.Value{typevalue.FromType(reg, typ.String)}},
		}},
	}

	_, changes := snapshotWithMaterializedSummaryProofChanges(reg, base, materialized, true)
	if _, ok := changes.Presentation[presentationKey]; !ok {
		t.Fatal("equivalent spelling rewrite missing from presentation changes")
	}
	if _, ok := changes.Semantic[presentationKey]; ok {
		t.Fatal("equivalent spelling rewrite incorrectly reported as semantic change")
	}
	if _, ok := changes.Presentation[semanticKey]; !ok {
		t.Fatal("semantic refinement missing from presentation changes")
	}
	if _, ok := changes.Semantic[semanticKey]; !ok {
		t.Fatal("semantic refinement missing from semantic changes")
	}
}

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
