package testfixture

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	schemaalgebra "github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

func TestFixtureSealsSelectGroupAndMergeRoots(t *testing.T) {
	fixture := New(t, 0x71)

	selectNode, ok := fixture.SelectNode()
	if !ok || !selectNode.Available() || selectNode.Kind() != schemaalgebra.KindSelect {
		t.Fatal("missing sealed Select root")
	}
	selectBinding, ok := selectNode.Select()
	if !ok || !selectBinding.Available() || len(selectNode.Children()) != 1 {
		t.Fatal("Select root did not redeem its sealed binding and child")
	}

	groupNode, ok := fixture.GroupNode()
	if !ok || !groupNode.Available() || groupNode.Kind() != schemaalgebra.KindGroup {
		t.Fatal("missing sealed Group root")
	}
	groupBinding, ok := groupNode.Group()
	if !ok || !groupBinding.Available() || !groupBinding.Key().Equal(fixture.LayoutLeftKey()) || len(groupNode.Children()) != 1 {
		t.Fatal("Group root did not redeem its sealed key binding and child")
	}
	groupRange, ok := groupBinding.Range()
	if !ok || !groupRange.Available() || groupRange.Kind() != schemaalgebra.KindGroup {
		t.Fatal("Group root did not issue its producer range")
	}

	mergeNode, ok := fixture.MergeNode()
	if !ok || !mergeNode.Available() || mergeNode.Kind() != schemaalgebra.KindMerge {
		t.Fatal("missing sealed Merge root")
	}
	mergeBinding, ok := mergeNode.Merge()
	if !ok || !mergeBinding.Available() || !mergeBinding.Key().Equal(fixture.LayoutLeftKey()) || len(mergeNode.Children()) != 2 {
		t.Fatal("Merge root did not redeem its sealed key binding and children")
	}
	lookupOnly := false
	for _, layout := range fixture.Mounted().Arrangement().Layouts() {
		if layout.CoordinateClass() == arrangement.CoordinateClassLookupOnly {
			lookupOnly = true
			break
		}
	}
	if !lookupOnly {
		t.Fatal("Merge did not seal a lookup-only physical coordinate")
	}
	mergeRange, ok := mergeBinding.Range()
	if !ok || !mergeRange.Available() || mergeRange.Kind() != schemaalgebra.KindMerge {
		t.Fatal("Merge root did not issue its producer range")
	}

	if selectNode.Digest() == groupNode.Digest() || selectNode.Digest() == mergeNode.Digest() || groupNode.Digest() == mergeNode.Digest() {
		t.Fatal("sealed operator roots share a structural identity")
	}
}
