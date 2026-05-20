package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestExtendsRecord_NilTypes(t *testing.T) {
	if ExtendsRecord(nil, typ.String) {
		t.Error("nil a should not extend")
	}
	if ExtendsRecord(typ.String, nil) {
		t.Error("nil b should not extend")
	}
}

func TestExtendsRecord_NotRecord(t *testing.T) {
	if ExtendsRecord(typ.String, typ.String) {
		t.Error("non-record should not extend")
	}
}

func TestExtendsRecord_MapComponentConsistency(t *testing.T) {
	oldRec := typ.NewRecord().MapComponent(typ.String, typ.Number).Build()
	newRec := typ.NewRecord().Field("x", typ.Number).Build()
	if ExtendsRecord(newRec, oldRec) {
		t.Error("record without map component should not extend record with map component")
	}
}

func TestCollapseTableTopEvidence_AbsorbsPreciseTableMembers(t *testing.T) {
	tableTop := typ.NewInterface("table", nil)
	preciseRecord := typ.NewRecord().
		Field("name", typ.String).
		Field("tools", typ.NewArray(typ.String)).
		Build()
	preciseMap := typ.NewMap(typ.String, typ.Integer)
	evidence := typ.NewUnion(typ.NewOptional(tableTop), preciseRecord, preciseMap, typ.String)

	got := CollapseTableTopEvidence(evidence)
	want := typ.NewUnion(typ.NewOptional(tableTop), typ.String)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("expected table top to absorb precise table members as %v, got %v", want, got)
	}
}

func TestSelectTableUpperBound_AbsorbsTableUnion(t *testing.T) {
	tableTop := typ.NewOptional(typ.NewInterface("table", nil))
	strategySpec := typ.NewRecord().
		Field("kind", typ.LiteralString("strategy")).
		Field("tools", typ.NewTuple(typ.String, typ.String, typ.String)).
		Build()
	contextSpec := typ.NewRecord().
		Field("kind", typ.LiteralString("context")).
		Field("scope", typ.String).
		Build()
	nextHint := typ.NewUnion(strategySpec, contextSpec)

	got, ok := SelectTableUpperBound(tableTop, nextHint)
	if !ok || !typ.TypeEquals(got, tableTop) {
		t.Fatalf("expected table top upper bound %v, got %v ok=%v", tableTop, got, ok)
	}
}

func TestJoinMapRecordShape_PureMapComponentBecomesMap(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	canonical := typ.NewMap(typ.String, typ.NewArray(entry))
	recordView := typ.NewRecord().
		MapComponent(typ.NewUnion(typ.String, typ.False), typ.NewArray(entry)).
		SetOpen(true).
		Build()
	join := func(a, b typ.Type) typ.Type {
		if IsTruthyRefinement(a, b) {
			return a
		}
		if IsTruthyRefinement(b, a) {
			return b
		}
		return typ.JoinPreferNonSoft(a, b)
	}

	got, ok := JoinMapRecordShape(canonical, recordView, join)
	if !ok || !typ.TypeEquals(got, canonical) {
		t.Fatalf("expected canonical map %v, got %v ok=%v", canonical, got, ok)
	}
}

func TestRefineStructuralAnnotation_MapValueFromRecordEvidence(t *testing.T) {
	annotation := typ.NewMap(typ.String, typ.Any)
	evidence := typ.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Integer).
		Build()

	got, changed := RefineStructuralAnnotation(annotation, evidence, typ.JoinPreferNonSoft)
	want := typ.NewMap(typ.String, typ.JoinPreferNonSoft(typ.String, typ.Integer))
	if !changed || !typ.TypeEquals(got, want) {
		t.Fatalf("expected refined map annotation %v, got %v changed=%v", want, got, changed)
	}
}

func TestRefinesFalsyMapKey(t *testing.T) {
	candidate := typ.NewMap(typ.String, typ.Number)
	baseline := typ.NewMap(typ.NewUnion(typ.String, typ.False), typ.Number)

	ok, changed := RefinesFalsyMapKey(candidate, baseline)
	if !ok || !changed {
		t.Fatalf("expected truthy key refinement, got ok=%v changed=%v", ok, changed)
	}
}

func TestRefinesTableKeyByTruthiness_Map(t *testing.T) {
	candidate := typ.NewMap(typ.String, typ.Number)
	baseline := typ.NewMap(typ.NewUnion(typ.String, typ.False), typ.Number)

	if !RefinesTableKeyByTruthiness(candidate, baseline) {
		t.Fatalf("expected map key truthiness refinement")
	}
}

func TestRefinesTableKeyByTruthiness_RecordMapComponent(t *testing.T) {
	candidate := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.String, typ.Number).
		Build()
	baseline := typ.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.NewUnion(typ.String, typ.False), typ.Number).
		Build()

	if !RefinesTableKeyByTruthiness(candidate, baseline) {
		t.Fatalf("expected record map-key truthiness refinement")
	}
}

func TestRefinesTableKeyByTruthiness_RejectsValueChange(t *testing.T) {
	candidate := typ.NewMap(typ.String, typ.Integer)
	baseline := typ.NewMap(typ.NewUnion(typ.String, typ.False), typ.Number)

	if RefinesTableKeyByTruthiness(candidate, baseline) {
		t.Fatalf("value changes are not table-key truthiness refinements")
	}
}

func TestRefinesTableKeyByTruthiness_SplitsNilableUnion(t *testing.T) {
	candidate := typ.NewUnion(typ.Nil, typ.NewMap(typ.String, typ.Number))
	baseline := typ.NewUnion(typ.Nil, typ.NewMap(typ.NewUnion(typ.String, typ.False), typ.Number))

	if !RefinesTableKeyByTruthiness(candidate, baseline) {
		t.Fatalf("expected nilable map key truthiness refinement")
	}
}

func TestNestedNilOnlyRegression(t *testing.T) {
	candidate := typ.NewRecord().Field("value", typ.Nil).Build()
	baseline := typ.NewRecord().OptField("value", typ.String).Build()

	if !NestedNilOnlyRegression(candidate, baseline) {
		t.Fatalf("expected nested nil-only regression")
	}
}

func TestContainsNestedStructuralShape(t *testing.T) {
	shape := typ.NewMap(typ.String, typ.Any)
	growing := typ.NewMap(typ.String, typ.NewMap(typ.String, typ.Nil))

	if !ContainsNestedStructuralShape(growing, shape) {
		t.Fatalf("expected nested structural shape")
	}
}
