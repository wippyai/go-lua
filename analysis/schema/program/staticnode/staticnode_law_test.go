package staticnode

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func staticnodeLawID(t *testing.T, name string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("program-staticnode-law/"+name, nil)
	if !ok {
		t.Fatalf("derive %s", name)
	}
	return id
}

func staticnodeLawView(t *testing.T, publication Publication, catalog identity.ContentID) View {
	t.Helper()
	builder := snapshot.NewFrozen(catalog, identity.StoreID(1))
	for slot := uint32(0); slot < 58; slot++ {
		if slot >= 42 && slot <= 54 {
			continue
		}
		axis := snapshot.Axis[uint32, uint32]{SchemaID: catalog, Slot: slot}
		content := snapshot.Content[uint32, uint32]{
			Sequence:    []uint32{},
			Denominator: staticnodeLawID(t, fmt.Sprintf("filler-%d", slot)),
		}
		if err := snapshot.PutFrozenColumn(&builder, axis, content); err != nil {
			t.Fatalf("put filler slot %d: %v", slot, err)
		}
	}
	if !publication.Append(&builder, catalog) {
		t.Fatal("static publication append")
	}
	frozen, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal static publication: %v", err)
	}
	state, ok := programstate.New(frozen, catalog)
	if !ok {
		t.Fatal("open program state")
	}
	view, ok := NewView(state)
	if !ok {
		t.Fatal("open static view")
	}
	return view
}

func TestStaticFamiliesBindCanonicalSlots(t *testing.T) {
	bindings := []struct {
		name string
		got  programcatalog.Definition
		want programcatalog.Definition
	}{
		{"StaticTypeNodeFamily", StaticTypeNodeFamily().Definition(), programcatalog.StaticTypeNode()},
		{"StaticTypeNodeUnionMemberFamily", StaticTypeNodeUnionMemberFamily().Definition(), programcatalog.StaticTypeNodeUnionMember()},
		{"StaticTypeNodeIntersectionMemberFamily", StaticTypeNodeIntersectionMemberFamily().Definition(), programcatalog.StaticTypeNodeIntersectionMember()},
		{"StaticTypeNodeGenericArgumentFamily", StaticTypeNodeGenericArgumentFamily().Definition(), programcatalog.StaticTypeNodeGenericArgument()},
		{"StaticTypeNodeAliasParameterFamily", StaticTypeNodeAliasParameterFamily().Definition(), programcatalog.StaticTypeNodeAliasParameter()},
		{"StaticTypeNodeInterfaceExtendFamily", StaticTypeNodeInterfaceExtendFamily().Definition(), programcatalog.StaticTypeNodeInterfaceExtend()},
		{"StaticTypeNodeInterfaceMemberFamily", StaticTypeNodeInterfaceMemberFamily().Definition(), programcatalog.StaticTypeNodeInterfaceMember()},
		{"StaticTypeNodeTypeFunctionTypeParameterFamily", StaticTypeNodeTypeFunctionTypeParameterFamily().Definition(), programcatalog.StaticTypeNodeTypeFunctionTypeParameter()},
		{"StaticTypeNodeTypeFunctionParameterFamily", StaticTypeNodeTypeFunctionParameterFamily().Definition(), programcatalog.StaticTypeNodeTypeFunctionParameter()},
		{"StaticTypeNodeTypeFunctionReturnFamily", StaticTypeNodeTypeFunctionReturnFamily().Definition(), programcatalog.StaticTypeNodeTypeFunctionReturn()},
		{"StaticTypeNodeRecordFieldFamily", StaticTypeNodeRecordFieldFamily().Definition(), programcatalog.StaticTypeNodeRecordField()},
		{"StaticTypeNodeReferenceSourceKeyFamily", StaticTypeNodeReferenceSourceKeyFamily().Definition(), programcatalog.StaticTypeNodeReferenceSourceKey()},
		{"StaticTypeNodeReferenceCanonicalKeyFamily", StaticTypeNodeReferenceCanonicalKeyFamily().Definition(), programcatalog.StaticTypeNodeReferenceCanonicalKey()},
	}
	for _, binding := range bindings {
		if binding.got != binding.want {
			t.Errorf("%s bound slot/name %d/%s, want %d/%s", binding.name,
				binding.got.Slot(), binding.got.Name(), binding.want.Slot(), binding.want.Name())
		}
	}
	if got := programcatalog.StaticTypeNode().Slot(); got != 42 {
		t.Fatalf("catalog parent slot = %d", got)
	}
}

func TestStaticPublicationAppendsAllEmptyColumns(t *testing.T) {
	catalog := staticnodeLawID(t, "empty-catalog")
	view := staticnodeLawView(t, Publication{}, catalog)
	if !view.Available() {
		t.Fatal("empty static publication did not open a view")
	}
	checks := []func() (int, bool){
		view.StaticTypeNodeCount,
		view.StaticTypeNodeUnionMemberCount,
		view.StaticTypeNodeIntersectionMemberCount,
		view.StaticTypeNodeGenericArgumentCount,
		view.StaticTypeNodeAliasParameterCount,
		view.StaticTypeNodeInterfaceExtendCount,
		view.StaticTypeNodeInterfaceMemberCount,
		view.StaticTypeNodeTypeFunctionTypeParameterCount,
		view.StaticTypeNodeTypeFunctionParameterCount,
		view.StaticTypeNodeTypeFunctionReturnCount,
		view.StaticTypeNodeRecordFieldCount,
		view.StaticTypeNodeReferenceSourceKeyCount,
		view.StaticTypeNodeReferenceCanonicalKeyCount,
	}
	for index, check := range checks {
		if count, published := check(); !published || count != 0 {
			t.Errorf("family %d count/published = %d/%v", index, count, published)
		}
	}
}

func TestStaticTypeNodeChildrenPreservesCanonicalOrderAndReferenceStrictness(t *testing.T) {
	catalog := staticnodeLawID(t, "children-catalog")
	parentID := staticnodeLawID(t, "union-parent")
	firstID := staticnodeLawID(t, "union-first")
	secondID := staticnodeLawID(t, "union-second")
	parent, ok := NewStaticTypeNode(StaticTypeNodeSpec{
		ID: parentID, Owner: staticnodeLawID(t, "owner"), Kind: StaticNodeUnion,
		UnionOffset: 0, UnionCount: 2,
	})
	if !ok {
		t.Fatal("union parent")
	}
	first, ok := NewStaticTypeNodeUnionMember(parentID, firstID, 0)
	if !ok {
		t.Fatal("first union member")
	}
	second, ok := NewStaticTypeNodeUnionMember(parentID, secondID, 1)
	if !ok {
		t.Fatal("second union member")
	}
	view := staticnodeLawView(t, Publication{
		StaticTypeNodes:            []StaticTypeNode{parent},
		StaticTypeNodeUnionMembers: []StaticTypeNodeUnionMember{first, second},
	}, catalog)
	children, ok := view.StaticTypeNodeChildren(0, parent, true)
	if !ok || len(children) != 2 || children[0] != firstID || children[1] != secondID {
		t.Fatalf("children = %v, ok=%v", children, ok)
	}

	badTarget := staticnodeLawID(t, "unresolved-target")
	refID := staticnodeLawID(t, "unresolved-ref")
	ref, ok := NewStaticTypeNode(StaticTypeNodeSpec{
		ID: refID, Owner: staticnodeLawID(t, "ref-owner"), Kind: StaticNodeReference,
		Resolution: uint8(staticrefs.Unresolved), ReferenceTarget: badTarget,
	})
	if !ok {
		t.Fatal("unresolved reference")
	}
	refView := staticnodeLawView(t, Publication{StaticTypeNodes: []StaticTypeNode{ref}}, catalog)
	if _, ok := refView.StaticTypeNodeChildren(0, ref, true); ok {
		t.Fatal("strict unresolved reference accepted a target")
	}
	if children, ok := refView.StaticTypeNodeChildren(0, ref, false); !ok || len(children) != 1 || children[0] != badTarget {
		t.Fatalf("lenient unresolved children = %v, ok=%v", children, ok)
	}
}
