package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCertifiedContextProjectionRequiresEntryRootFact(t *testing.T) {
	reg := standard.Registry()
	value := typevalue.LiteralString(reg, "entry")
	result := transformer.SpecializationResult{PreservedParams: []uint32{0}}
	certificate := &relationContextEntryCertificate{
		params:          []relationContextEntryParam{{value: value}},
		rootRefinements: []bool{false},
	}
	omitted, ok := projectCertifiedContextSummary(reg, result, certificate)
	if !ok || len(omitted.NormalReturnFacts.PathRefinements) != 0 {
		t.Fatalf("value-only projection = %#v/%v, want no invented root fact", omitted.NormalReturnFacts.PathRefinements, ok)
	}
	certificate.rootRefinements[0] = true
	projected, ok := projectCertifiedContextSummary(reg, result, certificate)
	if !ok || len(projected.NormalReturnFacts.PathRefinements) != 1 {
		t.Fatalf("certified projection = %#v/%v, want one root fact", projected.NormalReturnFacts.PathRefinements, ok)
	}
	fact := projected.NormalReturnFacts.PathRefinements[0]
	if fact.Path.PlaceholderIndex() != 0 || !product.Equal(reg, fact.Value, product.ProjectBoundary(reg, value)) {
		t.Fatalf("certified root fact = %#v", fact)
	}
}

func TestCertifiedContextProjectionOmitsTopAndRejectsShapeDrift(t *testing.T) {
	reg := standard.Registry()
	result := transformer.SpecializationResult{PreservedParams: []uint32{0}}
	certificate := &relationContextEntryCertificate{
		params:          []relationContextEntryParam{{value: product.Top()}},
		rootRefinements: []bool{true},
	}
	projected, ok := projectCertifiedContextSummary(reg, result, certificate)
	if !ok || len(projected.NormalReturnFacts.PathRefinements) != 0 {
		t.Fatalf("top projection = %#v/%v, want omission", projected.NormalReturnFacts.PathRefinements, ok)
	}
	certificate.rootRefinements = nil
	if _, ok := projectCertifiedContextSummary(reg, result, certificate); ok {
		t.Fatal("certificate shape drift was accepted")
	}
}
