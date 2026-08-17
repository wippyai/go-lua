package accessgeometry

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func accessGeometryTestID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

func accessGeometryTestResult() *Result {
	return &Result{
		sourceID:      accessGeometryTestID(1),
		flowID:        accessGeometryTestID(2),
		staticID:      accessGeometryTestID(3),
		moduleID:      accessGeometryTestID(4),
		tableFields:   tableFieldProjection{keys: []keyspace.Key{0, 7, 0}},
		exactLenses:   exactLensProjection{keys: []keyspace.Key{0, 8}},
		dynamicLenses: dynamicLensProjection{keys: []keyspace.Key{0, 0}},
		indexAccesses: indexProjection{
			accesses: []indexAccess{
				{Read: keyspace.MakeTerm(keyspace.FamilyRead, 1), Base: keyspace.MakeTerm(keyspace.FamilyNil, 1), KeyTerm: keyspace.MakeTerm(keyspace.FamilyKey, 1), Position: -1, Lens: keyspace.MakeTerm(keyspace.FamilyLensExact, 1)},
				{Write: keyspace.MakeTerm(keyspace.FamilyWrite, 2), Base: keyspace.MakeTerm(keyspace.FamilyNil, 2), KeyTerm: keyspace.MakeTerm(keyspace.FamilyNil, 2), Values: keyspace.MakeTerm(keyspace.FamilyValues, 1), Position: 1, Lens: keyspace.MakeTerm(keyspace.FamilyLensKey, 1)},
			},
			reads:      []uint32{0, 1},
			writes:     []uint32{0, 0, 2},
			readCount:  1,
			writeCount: 1,
		},
	}
}

func TestAccessGeometryProvenanceFenceAndDenominators(t *testing.T) {
	result := accessGeometryTestResult()
	sourceID, flowID, staticID, moduleID := result.sourceID, result.flowID, result.staticID, result.moduleID
	if !Matches(result, sourceID, flowID, staticID, moduleID) {
		t.Fatal("matching owner quartet was rejected")
	}
	if Matches(result, sourceID, flowID, staticID, identity.ContentID{}) ||
		Matches(result, sourceID, flowID, staticID, accessGeometryTestID(9)) {
		t.Fatal("foreign or unavailable provenance matched")
	}
	if result.TableFields().Count() != 2 || result.ExactLenses().Count() != 1 || result.DynamicLenses().Count() != 1 {
		t.Fatal("typed views did not retain their exact authored denominators")
	}
	if _, ok := result.TableFields().At(2); ok {
		t.Fatal("TableField At escaped its denominator")
	}
}

func TestAccessGeometryZeroKeysAndTypedIndexGetSetViews(t *testing.T) {
	result := accessGeometryTestResult()
	fields, exact, dynamic := result.TableFields(), result.ExactLenses(), result.DynamicLenses()
	if key, ok := fields.Get(keyspace.MakeTerm(keyspace.FamilyTableField, 1)); !ok || key != 7 {
		t.Fatalf("TableField normalized key = %d/%v, want 7/true", key, ok)
	}
	if key, ok := fields.Get(keyspace.MakeTerm(keyspace.FamilyTableField, 2)); !ok || key != 0 {
		t.Fatalf("dynamic TableField key = %d/%v, want zero/true", key, ok)
	}
	if key, ok := exact.Get(keyspace.MakeTerm(keyspace.FamilyLensExact, 1)); !ok || key != 8 {
		t.Fatalf("ExactLens normalized key = %d/%v, want 8/true", key, ok)
	}
	if key, ok := dynamic.Get(keyspace.MakeTerm(keyspace.FamilyLensKey, 1)); !ok || key != 0 {
		t.Fatalf("DynamicLens key = %d/%v, want zero/true", key, ok)
	}

	accesses := result.IndexAccesses()
	reads, writes := accesses.Reads(), accesses.Writes()
	if reads.Count() != 1 || writes.Count() != 1 {
		t.Fatalf("IndexAccess denominators = %d reads, %d writes; want 1, 1", reads.Count(), writes.Count())
	}
	if term, ok := reads.At(0); !ok || term != keyspace.MakeTerm(keyspace.FamilyRead, 1) || !reads.Contains(term) {
		t.Fatalf("candidate Read At/Contains = %v/%v", term, ok)
	}
	if term, ok := writes.At(0); !ok || term != keyspace.MakeTerm(keyspace.FamilyWrite, 2) || !writes.Contains(term) {
		t.Fatalf("candidate Write At/Contains = %v/%v", term, ok)
	}
	base, keyTerm, lens, ok := reads.Get(keyspace.MakeTerm(keyspace.FamilyRead, 1))
	if !ok || base != keyspace.MakeTerm(keyspace.FamilyNil, 1) || keyTerm != keyspace.MakeTerm(keyspace.FamilyKey, 1) || lens != keyspace.MakeTerm(keyspace.FamilyLensExact, 1) {
		t.Fatalf("IndexGet row = %v,%v,%v,%v", base, keyTerm, lens, ok)
	}
	if exactKey, ok := exact.Get(lens); !ok || exactKey != 8 {
		t.Fatalf("IndexGet exact key plane = %d/%v", exactKey, ok)
	}
	base, keyTerm, values, position, lens, ok := writes.Get(keyspace.MakeTerm(keyspace.FamilyWrite, 2))
	if !ok || base != keyspace.MakeTerm(keyspace.FamilyNil, 2) || keyTerm != keyspace.MakeTerm(keyspace.FamilyNil, 2) || values != keyspace.MakeTerm(keyspace.FamilyValues, 1) || position != 1 || lens != keyspace.MakeTerm(keyspace.FamilyLensKey, 1) {
		t.Fatalf("IndexSet row = %v,%v,%v,%d,%v,%v", base, keyTerm, values, position, lens, ok)
	}
	if slot, ok := reads.Slot(keyspace.MakeTerm(keyspace.FamilyRead, 1)); !ok || slot != 0 {
		t.Fatalf("Read candidate slot = %d/%v, want 0/true", slot, ok)
	}
	if slot, ok := writes.Slot(keyspace.MakeTerm(keyspace.FamilyWrite, 2)); !ok || slot != 0 {
		t.Fatalf("Write candidate slot = %d/%v, want 0/true", slot, ok)
	}
}

func TestAccessGeometrySparseCandidateOrdinalsRemainDistinctFromEnumeration(t *testing.T) {
	result := accessGeometryTestResult()
	result.indexAccesses.reads = []uint32{0, 1, 0, 2}
	result.indexAccesses.writes = []uint32{0, 0, 3, 0, 4}
	result.indexAccesses.accesses = []indexAccess{
		result.indexAccesses.accesses[0],
		{Read: keyspace.MakeTerm(keyspace.FamilyRead, 3), Base: keyspace.MakeTerm(keyspace.FamilyNil, 3), KeyTerm: keyspace.MakeTerm(keyspace.FamilyNil, 3), Position: -1, Lens: keyspace.MakeTerm(keyspace.FamilyLensKey, 1)},
		result.indexAccesses.accesses[1],
		{Write: keyspace.MakeTerm(keyspace.FamilyWrite, 4), Base: keyspace.MakeTerm(keyspace.FamilyNil, 4), KeyTerm: keyspace.MakeTerm(keyspace.FamilyNil, 4), Values: keyspace.MakeTerm(keyspace.FamilyValues, 2), Position: 0, Lens: keyspace.MakeTerm(keyspace.FamilyLensKey, 1)},
	}
	result.indexAccesses.readCount = 2
	result.indexAccesses.writeCount = 2
	reads, writes := result.IndexAccesses().Reads(), result.IndexAccesses().Writes()
	if term, ok := reads.At(0); !ok || term != keyspace.MakeTerm(keyspace.FamilyRead, 1) {
		t.Fatalf("first dense Read At = %v/%v", term, ok)
	}
	if term, ok := reads.At(1); !ok || term != keyspace.MakeTerm(keyspace.FamilyRead, 3) {
		t.Fatalf("second dense Read At = %v/%v", term, ok)
	}
	if term, ok := writes.At(0); !ok || term != keyspace.MakeTerm(keyspace.FamilyWrite, 2) {
		t.Fatalf("first dense Write At = %v/%v", term, ok)
	}
	if term, ok := writes.At(1); !ok || term != keyspace.MakeTerm(keyspace.FamilyWrite, 4) {
		t.Fatalf("second dense Write At = %v/%v", term, ok)
	}
	if slot, ok := reads.Slot(keyspace.MakeTerm(keyspace.FamilyRead, 2)); ok || slot != 0 {
		t.Fatalf("absent Read sparse slot = %d/%v", slot, ok)
	}
	if slot, ok := reads.Slot(keyspace.MakeTerm(keyspace.FamilyRead, 3)); !ok || slot != 1 {
		t.Fatalf("second Read sparse slot = %d/%v, want 1/true", slot, ok)
	}
	if slot, ok := writes.Slot(keyspace.MakeTerm(keyspace.FamilyWrite, 3)); ok || slot != 0 {
		t.Fatalf("absent Write sparse slot = %d/%v", slot, ok)
	}
	if slot, ok := writes.Slot(keyspace.MakeTerm(keyspace.FamilyWrite, 4)); !ok || slot != 1 {
		t.Fatalf("second Write sparse slot = %d/%v, want 1/true", slot, ok)
	}
}

func TestAccessGeometryQueriesFailClosedForMalformedPlanes(t *testing.T) {
	var unavailable *Result
	if _, ok := unavailable.TableFields().Get(keyspace.MakeTerm(keyspace.FamilyTableField, 1)); ok {
		t.Fatal("nil Result exposed a TableField")
	}
	if _, ok := unavailable.ExactLenses().Get(keyspace.MakeTerm(keyspace.FamilyLensExact, 1)); ok {
		t.Fatal("nil Result exposed an ExactLens")
	}
	if _, ok := unavailable.DynamicLenses().Get(keyspace.MakeTerm(keyspace.FamilyLensKey, 1)); ok {
		t.Fatal("nil Result exposed a DynamicLens")
	}
	if unavailable.IndexAccesses().Reads().Count() != 0 || unavailable.IndexAccesses().Writes().Count() != 0 {
		t.Fatal("nil Result exposed indexed-access denominators")
	}
	result := accessGeometryTestResult()
	result.indexAccesses.accesses[0].Lens = keyspace.MakeTerm(keyspace.FamilyCell, 1)
	if _, _, _, ok := result.IndexAccesses().Reads().Get(keyspace.MakeTerm(keyspace.FamilyRead, 1)); ok {
		t.Fatal("malformed IndexGet Lens was queryable")
	}
	result = accessGeometryTestResult()
	result.indexAccesses.accesses[1].KeyTerm = 0
	if _, _, _, _, _, ok := result.IndexAccesses().Writes().Get(keyspace.MakeTerm(keyspace.FamilyWrite, 2)); ok {
		t.Fatal("malformed IndexSet raw key term was queryable")
	}
	result = accessGeometryTestResult()
	if _, ok := result.TableFields().Get(keyspace.MakeTerm(keyspace.FamilyLensExact, 1)); ok {
		t.Fatal("wrong-family TableField query succeeded")
	}
	if _, ok := result.DynamicLenses().Get(keyspace.MakeTerm(keyspace.FamilyLensKey, 2)); ok {
		t.Fatal("out-of-denominator DynamicLens query succeeded")
	}
}

func TestAccessGeometryQueriesScaleWithDensePlanes(t *testing.T) {
	const members = 10000
	result := &Result{
		sourceID:      accessGeometryTestID(1),
		flowID:        accessGeometryTestID(2),
		staticID:      accessGeometryTestID(3),
		moduleID:      accessGeometryTestID(4),
		tableFields:   tableFieldProjection{keys: make([]keyspace.Key, members+1)},
		exactLenses:   exactLensProjection{keys: make([]keyspace.Key, members+1)},
		dynamicLenses: dynamicLensProjection{keys: make([]keyspace.Key, members+1)},
	}
	for index := 1; index <= members; index++ {
		result.tableFields.keys[index] = keyspace.Key(index)
		result.exactLenses.keys[index] = keyspace.Key(index)
	}
	fields, exact, dynamic := result.TableFields(), result.ExactLenses(), result.DynamicLenses()
	if fields.Count() != members || exact.Count() != members || dynamic.Count() != members {
		t.Fatal("dense scaling denominator changed")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		if key, ok := fields.Get(keyspace.MakeTerm(keyspace.FamilyTableField, members)); !ok || key != members {
			t.Fatal("scaled TableField query failed")
		}
		if key, ok := exact.Get(keyspace.MakeTerm(keyspace.FamilyLensExact, members)); !ok || key != members {
			t.Fatal("scaled ExactLens query failed")
		}
		if key, ok := dynamic.Get(keyspace.MakeTerm(keyspace.FamilyLensKey, members)); !ok || key != 0 {
			t.Fatal("scaled DynamicLens query failed")
		}
	}); allocations != 0 {
		t.Fatalf("dense queries allocated %v objects per run", allocations)
	}
}
