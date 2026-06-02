package captured

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFromParentFactsAtPoint_NilParentFacts(t *testing.T) {
	result := FromParentFactsAtPoint(nil, nil, 0, nil, PathProjection{})
	if result != nil {
		t.Errorf("expected nil for nil parent facts, got %v", result)
	}
}

func TestFromParentFactsAtPoint_NilChildGraph(t *testing.T) {
	result := FromParentFactsAtPoint(nil, nil, 1, nil, PathProjection{})
	if result != nil {
		t.Errorf("expected nil for nil child graph, got %v", result)
	}
}

func TestFromParentFactsAtPoint_ZeroPoint(t *testing.T) {
	result := FromParentFactsAtPoint(nil, nil, 0, nil, PathProjection{})
	if result != nil {
		t.Errorf("expected nil for zero def point, got %v", result)
	}
}

func TestCapturedTypeAtPointMergesPathSensitiveRecordField(t *testing.T) {
	const sym cfg.SymbolID = 7
	const point cfg.Point = 3
	base := typ.NewRecord().
		Field("suites_hierarchy", typ.NewRecord().Build()).
		Field("name", typ.String).
		Build()
	suite := typ.NewAlias("Suite", typ.NewRecord().Field("name", typ.String).Build())
	wantSuites := typ.NewArray(suite)

	facts := fixedFacts{declared: map[cfg.SymbolID]typ.Type{sym: base}}
	root := constraint.Path{Symbol: sym}
	got := capturedTypeAtPoint(facts, point, sym, PathProjection{
		Children: projectionSurface{
			childTypes: func(p cfg.Point, path constraint.Path) []flow.PathFact {
				if p == point && path.Equal(root) {
					return []flow.PathFact{{Path: root.Field("suites_hierarchy"), Type: wantSuites}}
				}
				return nil
			},
		},
	})

	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("capturedTypeAtPoint = %T, want record", got)
	}
	field := rec.GetField("suites_hierarchy")
	if field == nil {
		t.Fatalf("capturedTypeAtPoint dropped suites_hierarchy field: %v", rec)
	}
	if !typ.TypeEquals(field.Type, wantSuites) {
		t.Fatalf("suites_hierarchy = %v, want %v", field.Type, wantSuites)
	}
	if name := rec.GetField("name"); name == nil || !typ.TypeEquals(name.Type, typ.String) {
		t.Fatalf("capturedTypeAtPoint lost unrelated field: %v", rec)
	}
}

func TestProjectPathFacts_UsesParentIdentityWhenRecursingIntoChildFacts(t *testing.T) {
	const sym cfg.SymbolID = 38
	const point cfg.Point = 6
	root := constraint.Path{Root: "class_mt", Symbol: sym}
	index := root.Field("__index")
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	base := typ.NewRecord().Field("__index", typ.NewRecord().Build()).Build()

	got := projectPathFacts(point, root, base, PathProjection{
		Children: projectionSurface{
			childTypes: func(p cfg.Point, path constraint.Path) []flow.PathFact {
				if p != point {
					return nil
				}
				if path.Equal(root) {
					return []flow.PathFact{{
						Path: constraint.Path{Root: "$sym38", Symbol: sym}.Field("__index"),
						Type: typ.NewRecord().Build(),
					}}
				}
				if path.Equal(index) {
					return []flow.PathFact{{Path: index.Field("is_empty"), Type: method}}
				}
				return nil
			},
		},
	}, nil, true)

	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("projectPathFacts = %T, want record", got)
	}
	indexField := rec.GetField("__index")
	if indexField == nil {
		t.Fatalf("projectPathFacts dropped __index: %v", rec)
	}
	indexRec, ok := indexField.Type.(*typ.Record)
	if !ok {
		t.Fatalf("__index = %T, want record", indexField.Type)
	}
	methodField := indexRec.GetField("is_empty")
	if methodField == nil || !typ.TypeEquals(methodField.Type, method) {
		t.Fatalf("__index.is_empty = %v, want %v", methodField, method)
	}
}

func TestProjectPathFacts_UsesVersionedChildFactIdentityForRecursion(t *testing.T) {
	const sym cfg.SymbolID = 38
	const version = 12
	const point cfg.Point = 6
	root := constraint.Path{Symbol: sym}
	index := constraint.Path{Symbol: sym, Version: version}.Field("__index")
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	base := typ.NewRecord().Field("__index", typ.NewRecord().Build()).Build()

	got := projectPathFacts(point, root, base, PathProjection{
		Children: projectionSurface{
			childTypes: func(p cfg.Point, path constraint.Path) []flow.PathFact {
				if p != point {
					return nil
				}
				if path.Equal(root) {
					return []flow.PathFact{{
						Path: index,
						Type: typ.NewRecord().Build(),
					}}
				}
				if path.Equal(index) {
					return []flow.PathFact{{Path: index.Field("is_empty"), Type: method}}
				}
				return nil
			},
		},
	}, nil, true)

	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("projectPathFacts = %T, want record", got)
	}
	indexField := rec.GetField("__index")
	if indexField == nil {
		t.Fatalf("projectPathFacts dropped __index: %v", rec)
	}
	indexRec, ok := indexField.Type.(*typ.Record)
	if !ok {
		t.Fatalf("__index = %T, want record", indexField.Type)
	}
	methodField := indexRec.GetField("is_empty")
	if methodField == nil || !typ.TypeEquals(methodField.Type, method) {
		t.Fatalf("__index.is_empty = %v, want %v", methodField, method)
	}
}

func TestProjectPathFacts_AppliesDirectPathFactOnceForUnion(t *testing.T) {
	const sym cfg.SymbolID = 8
	const point cfg.Point = 4
	path := constraint.Path{Symbol: sym}.Field("suite")
	base := typ.NewUnion(
		typ.NewRecord().Field("name", typ.String).Build(),
		typ.NewRecord().Field("full_path", typ.String).Build(),
	)
	direct := typ.NewUnion(
		typ.NewRecord().Field("name", typ.String).Field("full_path", typ.String).Build(),
		typ.NewRecord().Field("name", typ.String).Field("parent", typ.String).Build(),
	)
	calls := 0

	got := projectPathFacts(point, path, base, PathProjection{
		Paths: projectionSurface{
			pathType: func(p cfg.Point, q constraint.Path) typ.Type {
				if p == point && q.Equal(path) {
					calls++
					return direct
				}
				return nil
			},
		},
	}, nil, true)

	if calls != 1 {
		t.Fatalf("direct path fact calls = %d, want 1", calls)
	}
	if !typ.TypeEquals(got, direct) {
		t.Fatalf("projectPathFacts = %v, want direct path fact %v", got, direct)
	}
}

func TestProjectPathFacts_ReconcilesDirectInitWitnessUnionWithDeclaredCapture(t *testing.T) {
	const sym cfg.SymbolID = 10
	const point cfg.Point = 7
	root := constraint.Path{Symbol: sym}
	declared := typ.NewRecord().
		Field("run_with", typ.Func().Param("self", typ.Any).Param("db", typ.String).Returns(typ.Any).Build()).
		Build()
	direct := typ.NewUnion(declared, typ.NewRecord().Build())

	got := projectPathFacts(point, root, declared, PathProjection{
		Paths: projectionSurface{
			pathType: func(p cfg.Point, path constraint.Path) typ.Type {
				if p == point && path.Equal(root) {
					return direct
				}
				return nil
			},
		},
	}, nil, true)

	if !typ.TypeEquals(got, declared) {
		t.Fatalf("projectPathFacts = %v, want declared capture contract %v", got, declared)
	}
}

func TestProjectPathFacts_UsesFiniteChildFactsInsteadOfDerivedRecursivePaths(t *testing.T) {
	const sym cfg.SymbolID = 9
	const point cfg.Point = 5
	root := constraint.Path{Symbol: sym}
	parent := root.Field("parent")
	base := typ.NewRecord().
		Field("name", typ.String).
		Field("parent", typ.NewRecord().
			Field("name", typ.String).
			Field("parent", typ.NewRecord().Field("name", typ.String).Build()).
			Build()).
		Build()
	parentType := typ.NewRecord().
		Field("name", typ.String).
		Field("parent", base).
		Build()
	parentCalls := 0

	got := projectPathFacts(point, root, base, PathProjection{
		Children: projectionSurface{
			childTypes: func(p cfg.Point, path constraint.Path) []flow.PathFact {
				if p != point {
					return nil
				}
				if path.Equal(root) {
					return []flow.PathFact{{Path: parent, Type: parentType}}
				}
				if path.Equal(parent) {
					parentCalls++
				}
				return nil
			},
		},
	}, nil, true)

	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("projectPathFacts = %T, want record", got)
	}
	field := rec.GetField("parent")
	if field == nil {
		t.Fatalf("projectPathFacts dropped parent field: %v", rec)
	}
	if !typ.TypeEquals(field.Type, parentType) {
		t.Fatalf("parent = %v, want direct child fact %v", field.Type, parentType)
	}
	if parentCalls != 1 {
		t.Fatalf("parent child fact probes = %d, want 1 finite probe", parentCalls)
	}
}

type fixedFacts struct {
	declared map[cfg.SymbolID]typ.Type
}

func (f fixedFacts) DeclaredAt(_ cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if t := f.declared[sym]; t != nil {
		return flow.TypedValue{Type: t, State: flow.StateResolved}
	}
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

func (f fixedFacts) RefinedAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{State: flow.StateUnknown}
}

func (f fixedFacts) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	return f.DeclaredAt(p, sym)
}

func (f fixedFacts) IsAnnotated(cfg.SymbolID) bool { return false }

type projectionSurface struct {
	pathType   func(cfg.Point, constraint.Path) typ.Type
	childTypes func(cfg.Point, constraint.Path) []flow.PathFact
}

func (p projectionSurface) ObservePath(q flow.PathObservationQuery) flow.PathObservation {
	if p.pathType == nil {
		return flow.PathObservation{}
	}
	t := p.pathType(q.Point, q.Path)
	if typ.IsAbsentOrUnknown(t) {
		return flow.PathObservation{}
	}
	return flow.PathObservation{
		Type:   t,
		State:  flow.StateResolved,
		Source: flow.PathObservationFactProjection,
	}
}

func (p projectionSurface) ObserveChildPaths(q flow.PathChildQuery) []flow.PathFact {
	if p.childTypes == nil {
		return nil
	}
	return p.childTypes(q.Point, q.Path)
}
