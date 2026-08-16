package source

import "testing"

// The issuance is a commit-only capability: copying its outer pointer cannot
// create a second consume, and a View from another completed commit is never
// interchangeable even when source shape is equal.
func TestSemanticPathIssuanceIsSharedOneShotAndExactViewFenced(t *testing.T) {
	input, index := sourceFixture(1)
	draft, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	component, issuance, err := finalizer.CommitWithSemanticPathIssuance(ownedIndex(draft, index))
	if err != nil || component == nil || issuance == nil {
		t.Fatalf("CommitWithSemanticPathIssuance = %v/%v/%v", component, issuance, err)
	}
	copy := issuance
	if !issuance.ConsumeSemanticPathIssuance(component.View()) {
		t.Fatal("exact committed View did not consume issuance")
	}
	if copy.ConsumeSemanticPathIssuance(component.View()) {
		t.Fatal("copied issuance consumed twice")
	}

	second, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	secondFinalizer, err := second.Finalizer()
	if err != nil {
		t.Fatal(err)
	}
	secondComponent, secondIssuance, err := secondFinalizer.CommitWithSemanticPathIssuance(ownedIndex(second, index))
	if err != nil || secondComponent == nil || secondIssuance == nil {
		t.Fatal("second issuance unavailable")
	}
	if secondIssuance.ConsumeSemanticPathIssuance(component.View()) {
		t.Fatal("foreign completed View consumed issuance")
	}
	if secondIssuance.ConsumeSemanticPathIssuance(secondComponent.View()) {
		t.Fatal("foreign consume left issuance live for exact retry")
	}
}
