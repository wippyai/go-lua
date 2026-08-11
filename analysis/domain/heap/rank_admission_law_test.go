package heap

import "testing"

// TestHeapSealAdmitsARepresentableThreeObjectRank proves the cold admission
// boundary consumed by the Mu scheduler.  A sealed allocation value can carry
// One plus Many (Recent and Summary): the maximal three Object positions.  If
// Seal succeeds, rank construction is total and that concrete fixed-coordinate
// score fits the stored representation proof.
func TestHeapSealAdmitsARepresentableThreeObjectRank(t *testing.T) {
	schema, key, _ := heapLatticeFixture(t)
	object := heapLatticeObject(t, schema, ShapeEligible, FrozenMutable, noneContainment(t, schema))
	one, oneOK := schema.One(key, object)
	many, manyOK := schema.Many(key, object, object)
	value, valueOK := schema.Relation(key, one, many)
	if !oneOK || !manyOK || !valueOK {
		t.Fatal("three-object allocation value")
	}
	if schema.owner.fixedObjectRankBound == 0 || schema.owner.maxObjectRankSum == 0 {
		t.Fatal("Seal omitted fixed-coordinate rank representation proof")
	}
	score, scoreOK := valueObjectWidenScore(value)
	if !scoreOK || score > schema.owner.maxObjectRankSum {
		t.Fatal("sealed three-object score exceeds its admission witness")
	}
	rank, rankOK := NewWidenRank(schema)
	if !rankOK || rank.maxObjectSum != schema.owner.maxObjectRankSum {
		t.Fatal("a sealed schema must construct its rank directly from the admission witness")
	}
}

// TestHeapRankAdmissionRejectsRepresentationOverflow distinguishes a cold
// schema rejection from a solver cap: no Value is widened and no precision is
// discarded.  A schema whose fixed CellState score cannot be represented is
// simply not a legal finite carrier.
func TestHeapRankAdmissionRejectsRepresentationOverflow(t *testing.T) {
	owner := &schema{presentPotential: ^uint64(0), referenceCount: 0}
	if owner.sealWidenRankBounds() {
		t.Fatal("unrepresentable fixed-coordinate rank was admitted")
	}
}
