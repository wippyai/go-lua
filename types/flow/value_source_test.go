package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestValueSourceFallbackPublication(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	sym := setupSymbol(g, "value", []cfg.Point{c.Entry()})
	setVersion(g, c.Entry(), sym, cfg.Version{Root: "value", Symbol: sym, ID: 1})
	inputs := newInputs(g)
	inputs.DeclaredTypes[sym] = typ.Unknown
	s := Solve(inputs, testResolver())
	t.Run("flow unknown replaces extracted concrete", func(t *testing.T) {
		got := s.valueTypeAt(c.Entry(), ValueSource{ValuePath: constraint.Path{Root: "value", Symbol: sym}, ValueType: typ.Integer})
		if !typ.IsUnknown(got) {
			t.Fatalf("got %v, want flow unknown", got)
		}
	})
	t.Run("no flow evidence uses fallback", func(t *testing.T) {
		got := s.valueTypeAt(c.Entry(), ValueSource{ValueType: typ.String})
		if !typ.TypeEquals(got, typ.String) {
			t.Fatalf("got %v, want string", got)
		}
	})
	t.Run("fallback cannot publish nested inference holes", func(t *testing.T) {
		fallback := typ.NewRecord().Field("a", typ.NewArray(typ.NewUnion(typ.Integer, typ.Unresolved))).Build()
		got := s.valueTypeAt(c.Entry(), ValueSource{ValueType: fallback})
		if !typ.IsFinal(got) {
			t.Fatalf("published pending type %v", got)
		}
	})
}

func TestValueSourceOptionalMapElement(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	sym := setupSymbol(g, "messages", []cfg.Point{c.Entry()})
	setVersion(g, c.Entry(), sym, cfg.Version{Root: "messages", Symbol: sym, ID: 1})
	inputs := newInputs(g)
	inputs.Decomposer = testMapElementDecomposer{}
	row := typ.NewMap(typ.String, typ.Any)
	inputs.DeclaredTypes[sym] = typ.NewOptional(typ.NewArray(row))
	s := Solve(inputs, testResolver())
	got := s.valueTypeAt(c.Entry(), ValueSource{MapElementSource: &MapElementSource{MapPath: constraint.Path{Root: "messages", Symbol: sym}}})
	if !typ.TypeEquals(got, row) {
		t.Fatalf("got %v, want %v", got, row)
	}
}

func TestValueSourcePreservesSequenceMetadata(t *testing.T) {
	c := cfg.New()
	s := Solve(newInputs(newMockSSAGraph(c)), testResolver())
	for _, fallback := range []typ.Type{
		typ.NewInferredArray(typ.Unresolved).WithExplicitNilWrite(),
		typ.NewInferredMap(typ.String, typ.Unresolved).WithExplicitNilWrite(),
	} {
		got := s.valueTypeAt(c.Entry(), ValueSource{ValueType: fallback, ValueElements: []ValueSource{{ValueType: typ.String}}})
		switch shape := got.(type) {
		case *typ.Array:
			if !shape.InferredPresence || !shape.ExplicitNilWrite {
				t.Fatalf("lost array metadata: %#v", shape)
			}
		case *typ.Map:
			if !shape.InferredPresence || !shape.ExplicitNilWrite {
				t.Fatalf("lost map metadata: %#v", shape)
			}
		default:
			t.Fatalf("changed shape: %T", got)
		}
	}
}

func TestValueSourceDependenciesIncludeParentChanges(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	sym := setupSymbol(g, "record", []cfg.Point{c.Entry()})
	setVersion(g, c.Entry(), sym, cfg.Version{Root: "record", Symbol: sym, ID: 1})
	value := ValueSource{ValueFields: []ValueFieldSource{{Name: "a", ValueSource: ValueSource{ValuePath: constraint.Path{Root: "record", Symbol: sym, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "value"}}}}}}}
	for _, kind := range []string{"index", "insert", "send"} {
		t.Run(kind, func(t *testing.T) {
			inputs := newInputs(g)
			switch kind {
			case "index":
				inputs.IndexerAssignments = []IndexerAssignment{{Point: c.Entry(), ValueSource: value}}
			case "insert":
				inputs.TableMutatorAssignments = []TableMutatorAssignment{{Point: c.Entry(), ValueSource: value}}
			case "send":
				inputs.ContainerMutatorAssignments = []ContainerMutatorAssignment{{Point: c.Entry(), ValueSource: value}}
			}
			s := Solve(inputs, testResolver())
			deps := s.buildAssignmentDependencies()
			if len(deps[symbolDependencyKey(sym)]) == 0 {
				t.Fatal("field read does not depend on changes to its parent")
			}
		})
	}
}

func TestValueSourcePendingEvidenceDefersPublication(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	sym := setupSymbol(g, "value", []cfg.Point{c.Entry()})
	setVersion(g, c.Entry(), sym, cfg.Version{Root: "value", Symbol: sym, ID: 1})
	s := Solve(newInputs(g), testResolver())
	// Transfer queries run before the finished solution enables its query cache.
	s.queryCacheEnabled = false
	path := constraint.Path{Root: "value", Symbol: sym}
	key := string(s.pkResolver.KeyAt(c.Entry(), path))
	source := ValueSource{ValuePath: path, ValueType: typ.Integer}
	s.setValue(key, typ.Unresolved)
	if got := s.valueTypeAt(c.Entry(), source); got != nil {
		t.Fatalf("published %v before evidence is ready", got)
	}
	s.setValue(key, typ.String)
	if got := s.valueTypeAt(c.Entry(), source); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("got %v after evidence settles, want string", got)
	}
}

func TestValueSourceWaitsForDynamicKeyEvidence(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	tableSym := setupSymbol(g, "input", []cfg.Point{c.Entry()})
	keySym := setupSymbol(g, "key", []cfg.Point{c.Entry()})
	for _, sym := range []cfg.SymbolID{tableSym, keySym} {
		setVersion(g, c.Entry(), sym, cfg.Version{Symbol: sym, ID: 1})
	}
	inputs := newInputs(g)
	inputs.Decomposer = testMapElementDecomposer{}
	inputs.DeclaredTypes[tableSym] = typ.NewRecord().Field("count", typ.Integer).Field("title", typ.String).SetComplete(true).Build()
	s := Solve(inputs, testResolver())
	s.queryCacheEnabled = false
	keyPath := constraint.Path{Root: "key", Symbol: keySym}
	key := string(s.pkResolver.KeyAt(c.Entry(), keyPath))
	source := ValueSource{MapElementSource: &MapElementSource{MapPath: constraint.Path{Root: "input", Symbol: tableSym}, KeySymbol: keySym, KeyVar: "key"}, ValueType: typ.Any}
	s.setValue(key, typ.Unresolved)
	if got := s.valueTypeAt(c.Entry(), source); got != nil {
		t.Fatalf("published %v before the key settles", got)
	}
	s.setValue(key, typ.LiteralString("count"))
	if got := s.valueTypeAt(c.Entry(), source); !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("got %v, want integer field", got)
	}
}

func TestValueSourceWaitsForDynamicMapEvidence(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	sym := setupSymbol(g, "input", []cfg.Point{c.Entry()})
	setVersion(g, c.Entry(), sym, cfg.Version{Symbol: sym, ID: 1})
	inputs := newInputs(g)
	inputs.Decomposer = testMapElementDecomposer{}
	s := Solve(inputs, testResolver())
	s.queryCacheEnabled = false
	path := constraint.Path{Root: "input", Symbol: sym}
	key := string(s.pkResolver.KeyAt(c.Entry(), path))
	source := ValueSource{MapElementSource: &MapElementSource{MapPath: path}, ValueType: typ.Any}
	s.setValue(key, typ.Unresolved)
	if got := s.valueTypeAt(c.Entry(), source); got != nil {
		t.Fatalf("published %v before the map settles", got)
	}
	s.setValue(key, typ.NewArray(typ.String))
	if got := s.valueTypeAt(c.Entry(), source); !typ.TypeEquals(got, typ.String) {
		t.Fatalf("got %v, want string element", got)
	}
}
