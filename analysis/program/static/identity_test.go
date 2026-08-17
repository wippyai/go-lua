package static

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"testing"
)

func TestStaticIdentityCodecsAreStableAndOwnerScoped(t *testing.T) {
	owner := identity.ContentID{0: 1}
	otherOwner := identity.ContentID{0: 2}
	term := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	otherTerm := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	ref, ok := (View{component: component}).StaticTypes().Ref(term)
	if !ok {
		t.Fatal("StaticTypes.Ref rejected the identity-law type term")
	}

	occur, occurOK := OccurrenceID(owner, 1, term)
	if !occurOK {
		t.Fatalf("OccurrenceID = %x/%v, want available identity", occur, occurOK)
	}
	if again, againOK := OccurrenceID(owner, 1, term); !againOK || again != occur {
		t.Fatal("OccurrenceID was not deterministic")
	}
	if changed, changedOK := OccurrenceID(owner, 1, otherTerm); !changedOK || changed == occur {
		t.Fatal("OccurrenceID ignored its authored term")
	}
	if changed, changedOK := OccurrenceID(otherOwner, 1, term); !changedOK || changed == occur {
		t.Fatal("OccurrenceID ignored its owner")
	}

	typeID, typeOK := TypeReferenceID(owner, ref)
	expressionID, expressionOK := ExpressionID(owner, ref)
	inputID, inputOK := InputID(owner, 2, term, 3)
	scopeID, scopeOK := ScopeID(owner, term)
	if !typeOK || !expressionOK || !inputOK || !scopeOK {
		t.Fatalf("static identity availability = type=%v expression=%v input=%v scope=%v", typeOK, expressionOK, inputOK, scopeOK)
	}
	if typeID == expressionID || typeID == inputID || typeID == scopeID || expressionID == inputID || expressionID == scopeID || inputID == scopeID {
		t.Fatal("Static identity domains collided")
	}
	if again, againOK := TypeReferenceID(owner, ref); !againOK || again != typeID {
		t.Fatal("TypeReferenceID was not deterministic")
	}
	if again, againOK := ExpressionID(owner, ref); !againOK || again != expressionID {
		t.Fatal("ExpressionID was not deterministic")
	}
	if changed, changedOK := InputID(owner, 2, term, 4); !changedOK || changed == inputID {
		t.Fatal("InputID ignored its dense index")
	}
	if changed, changedOK := ScopeID(owner, otherTerm); !changedOK || changed == scopeID {
		t.Fatal("ScopeID ignored its authored scope")
	}
}

func TestStaticIdentityCodecsRejectUnavailableInputs(t *testing.T) {
	term := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	var zero identity.ContentID
	if _, ok := OccurrenceID(zero, 1, term); ok {
		t.Fatal("OccurrenceID accepted an unavailable owner")
	}
	if _, ok := OccurrenceID(identity.ContentID{0: 1}, 0, term); ok {
		t.Fatal("OccurrenceID accepted an invalid family")
	}
	if _, ok := OccurrenceID(identity.ContentID{0: 1}, 1, 0); ok {
		t.Fatal("OccurrenceID accepted an invalid term")
	}
	if _, ok := TypeReferenceID(identity.ContentID{0: 1}, StaticTypeRef{}); ok {
		t.Fatal("TypeReferenceID accepted an unavailable reference")
	}
	if _, ok := ExpressionID(identity.ContentID{0: 1}, StaticTypeRef{}); ok {
		t.Fatal("ExpressionID accepted an unavailable reference")
	}
	if _, ok := InputID(zero, 1, term, 0); ok {
		t.Fatal("InputID accepted an unavailable owner")
	}
	if _, ok := InputID(identity.ContentID{0: 1}, 1, 0, 0); ok {
		t.Fatal("InputID accepted an invalid source")
	}
	if _, ok := ScopeID(zero, term); ok {
		t.Fatal("ScopeID accepted an unavailable owner")
	}
	if _, ok := ScopeID(identity.ContentID{0: 1}, 0); ok {
		t.Fatal("ScopeID accepted an invalid scope")
	}
}
