package static

import (
	"testing"

	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestSemanticSourceFragmentPublishesCanonicalZeroDenominator(t *testing.T) {
	component := staticContentComponent(t, Input{})
	fragment, err := SemanticSourceFragment(component.View())
	if err != nil {
		t.Fatalf("SemanticSourceFragment(empty) error = %v", err)
	}
	wantFacets := []semanticsource.Facet{
		0,
		semanticsource.FacetProgramStaticFunctionContract,
		semanticsource.FacetProgramStaticCallTypeArguments,
		semanticsource.FacetProgramStaticCellDeclaredType,
		semanticsource.FacetProgramStaticClaimTarget,
		semanticsource.FacetProgramStaticTypeValueTarget,
		semanticsource.FacetProgramStaticTypeof,
		semanticsource.FacetProgramStaticAnnotation,
		semanticsource.FacetProgramStaticPublication,
		semanticsource.FacetProgramStaticTypeRef,
	}
	if len(fragment) != len(wantFacets) {
		t.Fatalf("fragment rows = %d, want %d", len(fragment), len(wantFacets))
	}
	for index, publication := range fragment {
		token := publication.Definition().Token()
		if token.Origin() != semanticsource.OriginProgramStatic || token.Facet() != wantFacets[index] {
			t.Fatalf("fragment[%d] token = (%v,%v), want (%v,%v)", index, token.Origin(), token.Facet(), semanticsource.OriginProgramStatic, wantFacets[index])
		}
		if publication.Count() != 0 {
			t.Fatalf("fragment[%d] count = %d, want zero", index, publication.Count())
		}
	}
}

func TestSemanticSourceFragmentUsesTypedFacetCardinalities(t *testing.T) {
	component := staticContentComponent(t, contractsFixture(t))
	fragment, err := SemanticSourceFragment(component.View())
	if err != nil {
		t.Fatalf("SemanticSourceFragment(contracts) error = %v", err)
	}
	want := []int{6, 1, 2, 0, 0, 0, 0, 0, 0, 0}
	if len(fragment) != len(want) {
		t.Fatalf("fragment rows = %d, want %d", len(fragment), len(want))
	}
	for index, publication := range fragment {
		if got := publication.Count(); got != want[index] {
			t.Fatalf("fragment[%d] count = %d, want %d", index, got, want[index])
		}
	}
}

func TestSemanticSourceFragmentRejectsUnavailableViews(t *testing.T) {
	if _, err := SemanticSourceFragment(View{}); err == nil {
		t.Fatal("SemanticSourceFragment(zero View) succeeded")
	}

	draft := emptyDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	view := finalizer.View()
	if _, err := finalizer.Commit(CommitInput{}); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if _, err := SemanticSourceFragment(view); err == nil {
		t.Fatal("SemanticSourceFragment(expired View) succeeded")
	}
}

func TestSemanticSourceFragmentRejectsTamperedOwnerProjection(t *testing.T) {
	component := contentReferencesComponent(t)
	component.references.rows = nil
	if publications, err := SemanticSourceFragment(component.View()); err == nil || publications != nil {
		t.Fatalf("tampered Static View result = %#v/%v, want nil/error", publications, err)
	}
}
