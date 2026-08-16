package role

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestNodeScopeAndTargetFamiliesRejectForeignOrdinals(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyTypeParam] = 1
	counts[keyspace.FamilyTypeField] = 1
	counts[keyspace.FamilyCell] = 1
	for _, test := range []struct {
		name                                string
		term                                keyspace.Term
		node, scope, ref, param, annotation bool
	}{
		{"primitive", keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1), true, false, false, false, true},
		{"alias", keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1), false, true, true, true, false},
		{"param", keyspace.MakeTerm(keyspace.FamilyTypeParam, 1), false, true, true, false, false},
		{"cell", keyspace.MakeTerm(keyspace.FamilyCell, 1), false, true, false, false, false},
		{"field", keyspace.MakeTerm(keyspace.FamilyTypeField, 1), false, false, false, false, true},
		{"foreign primitive ordinal", keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2), false, false, false, false, false},
		{"zero", 0, false, false, false, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := Node(counts, test.term); got != test.node {
				t.Fatalf("Node = %v, want %v", got, test.node)
			}
			if got := ScopeHandle(counts, test.term); got != test.scope {
				t.Fatalf("ScopeHandle = %v, want %v", got, test.scope)
			}
			if got := TypeReferenceTarget(counts, test.term); got != test.ref {
				t.Fatalf("TypeReferenceTarget = %v, want %v", got, test.ref)
			}
			if got := TypeParameterOwner(counts, test.term); got != test.param {
				t.Fatalf("TypeParameterOwner = %v, want %v", got, test.param)
			}
			if got := AnnotationTarget(counts, test.term); got != test.annotation {
				t.Fatalf("AnnotationTarget = %v, want %v", got, test.annotation)
			}
		})
	}
}

func TestRoleFamilyMatrixIsClosed(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		counts[family] = 1
		term := keyspace.MakeTerm(family, 1)
		if term == 0 {
			t.Fatalf("MakeTerm(%d, 1) returned zero", family)
		}
		if Node(counts, keyspace.MakeTerm(family, 2)) ||
			ScopeHandle(counts, keyspace.MakeTerm(family, 2)) ||
			TypeReferenceTarget(counts, keyspace.MakeTerm(family, 2)) ||
			TypeParameterOwner(counts, keyspace.MakeTerm(family, 2)) ||
			AnnotationTarget(counts, keyspace.MakeTerm(family, 2)) {
			t.Fatalf("family %d accepted an out-of-count ordinal", family)
		}
		counts[family] = 0
	}
}

func TestRoleFamilyOnlyMatrix(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		counts[family] = 1
		term := keyspace.MakeTerm(family, 1)
		if NodeFamily(family) != Node(counts, term) {
			t.Fatalf("NodeFamily(%d) disagrees with counted Node", family)
		}
		if ScopeHandleFamily(family) != ScopeHandle(counts, term) {
			t.Fatalf("ScopeHandleFamily(%d) disagrees with counted ScopeHandle", family)
		}
		if TypeReferenceTargetFamily(family) != TypeReferenceTarget(counts, term) {
			t.Fatalf("TypeReferenceTargetFamily(%d) disagrees with counted target", family)
		}
		if TypeParameterOwnerFamily(family) != TypeParameterOwner(counts, term) {
			t.Fatalf("TypeParameterOwnerFamily(%d) disagrees with counted owner", family)
		}
		if AnnotationTargetFamily(family) != AnnotationTarget(counts, term) {
			t.Fatalf("AnnotationTargetFamily(%d) disagrees with counted annotation target", family)
		}
		counts[family] = 0
	}
}
