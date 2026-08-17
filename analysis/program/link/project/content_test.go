package project

import "testing"

func TestProjectRelationDigestsAreCanonicalAcrossInputOrder(t *testing.T) {
	first := projectProgram(t, `return 1`)
	second := projectProgram(t, `return 2`)
	left := projectDraft(t, []Module{{Name: "first", Program: first}, {Name: "second", Program: second}})
	right := projectDraft(t, []Module{{Name: "second", Program: second}, {Name: "first", Program: first}})
	leftComponent, err := left.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	rightComponent, err := right.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if leftComponent.Cold().ContentID() != rightComponent.Cold().ContentID() {
		t.Fatal("Project contentID changed with module input order")
	}
	leftMountID, leftOK := leftComponent.MountRelationID()
	rightMountID, rightOK := rightComponent.MountRelationID()
	if !leftOK || !rightOK || leftMountID != rightMountID {
		t.Fatal("Project mount relation changed with module input order")
	}
}
