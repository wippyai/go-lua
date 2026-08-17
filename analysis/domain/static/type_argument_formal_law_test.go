package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// A type-argument node is admitted as an open node: its free formals belong to
// the boundary that binds them, not to the node. Openness is not an exemption
// from the declaration law, so an unlawful generic graph carries no identity
// even while it is open.
func TestTypeArgumentFormalIdentityRejectsUnlawfulOpenNode(t *testing.T) {
	first := mutualGenericPairForTypeArgumentTest()
	if !typ.ContainsTypeParam(first) {
		t.Fatal("fixture is not the open shape the law admits at its boundary")
	}
	if _, ok := typeArgumentFormalIdentity(first); ok {
		t.Fatal("a mutual generic group carrying formals was issued a type-argument identity")
	}
}

// The same boundary must keep issuing identity for the open nodes it exists to
// carry: a bare formal and a lawful declaration that binds one.
func TestTypeArgumentFormalIdentityIssuesLawfulOpenNode(t *testing.T) {
	formal := typ.NewTypeParam("T", nil)
	declaration := typ.NewGeneric("Box", []*typ.TypeParam{formal}, typ.NewArray(formal))
	for _, value := range []typ.Type{formal, declaration, typ.String} {
		if _, ok := typeArgumentFormalIdentity(value); !ok {
			t.Fatalf("lawful node %v was issued no type-argument identity", value)
		}
	}
}

// mutualGenericPairForTypeArgumentTest builds two generic declarations that
// reach each other productively. The group needs a second binder vocabulary to
// be represented, so it is exactly what the recurrence law rejects.
func mutualGenericPairForTypeArgumentTest() *typ.Generic {
	firstParam := typ.NewTypeParam("T", nil)
	secondParam := typ.NewTypeParam("U", nil)
	first := typ.NewGeneric("First", []*typ.TypeParam{firstParam}, nil)
	second := typ.NewGeneric("Second", []*typ.TypeParam{secondParam}, nil)
	first.SetBody(typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "next", Type: typ.Instantiate(second, firstParam)}}}))
	second.SetBody(typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{{Name: "next", Type: typ.Instantiate(first, secondParam)}}}))
	return first
}
