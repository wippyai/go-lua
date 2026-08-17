package flow

import "testing"

func TestComponentViewPublishesItsOwnContentIdentity(t *testing.T) {
	assembly := openProjectionAssembly(t, "component-view.lua")
	_, component, _, _, err := assembly.Take()
	if err != nil {
		t.Fatalf("Assembly.Take: %v", err)
	}
	view := component.View()
	if !view.ContentID().Available() || view.ContentID() != component.ContentID() {
		t.Fatalf("Component/View ContentID = %v/%v, want one sealed identity", component.ContentID(), view.ContentID())
	}
	if !view.Provenance().Flow.Available() || view.Provenance().Flow != view.ContentID() {
		t.Fatalf("Flow provenance = %#v, want the component identity", view.Provenance())
	}
	var unavailable View
	if unavailable.ContentID().Available() || unavailable.Authored().ContentID().Available() {
		t.Fatal("zero Flow View exposed an identity")
	}
}
