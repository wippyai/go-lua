package typefacts

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

type functionTypeMap map[cfg.SymbolID]typ.Type

func (m functionTypeMap) lookup(sym cfg.SymbolID) typ.Type {
	return m[sym]
}

func TestTypeFactsDeclaredAtPrefersAnnotatedDeclaration(t *testing.T) {
	const sym cfg.SymbolID = 1
	facts := New(Config{
		Declared:      flow.DeclaredTypes{sym: typ.String},
		FunctionType:  functionTypeMap{sym: typ.Number}.lookup,
		Literals:      flow.DeclaredTypes{sym: typ.Boolean},
		AnnotatedVars: map[cfg.SymbolID]bool{sym: true},
	})

	got := facts.DeclaredAt(0, sym)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, typ.String) {
		t.Fatalf("DeclaredAt annotated symbol = %v/%v, want string/resolved", got.Type, got.State)
	}
}

func TestTypeFactsSeparatesBindingValueFromDeclaration(t *testing.T) {
	const sym cfg.SymbolID = 2
	fn := typ.Func().Returns(typ.String).Build()
	facts := New(Config{
		Declared:     flow.DeclaredTypes{sym: typ.Number},
		FunctionType: functionTypeMap{sym: fn}.lookup,
	})

	declared := facts.DeclaredAt(0, sym)
	if declared.State != flow.StateResolved || !typ.TypeEquals(declared.Type, typ.Number) {
		t.Fatalf("DeclaredAt function symbol = %v/%v, want declared number/resolved", declared.Type, declared.State)
	}

	binding := facts.BindingValueAt(0, sym)
	if binding.State != flow.StateResolved || !typ.TypeEquals(binding.Type, fn) {
		t.Fatalf("BindingValueAt function symbol = %v/%v, want canonical function fact", binding.Type, binding.State)
	}

	effective := facts.EffectiveTypeAt(0, sym)
	if effective.State != flow.StateResolved || !typ.TypeEquals(effective.Type, fn) {
		t.Fatalf("EffectiveTypeAt function symbol = %v/%v, want canonical function fact", effective.Type, effective.State)
	}
}

func TestTypeFactsDeclaredAtUsesLiteralLast(t *testing.T) {
	const sym cfg.SymbolID = 3
	facts := New(Config{
		Literals: flow.DeclaredTypes{sym: typ.Boolean},
	})

	got := facts.DeclaredAt(0, sym)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, typ.Boolean) {
		t.Fatalf("DeclaredAt literal-only symbol = %v/%v, want boolean/resolved", got.Type, got.State)
	}
}

func TestTypeFactsDeclaredAtUnknownIsUnknownState(t *testing.T) {
	const sym cfg.SymbolID = 4
	facts := New(Config{
		Declared: flow.DeclaredTypes{sym: typ.Unknown},
	})

	got := facts.DeclaredAt(0, sym)
	if got.State != flow.StateUnknown || !typ.TypeEquals(got.Type, typ.Unknown) {
		t.Fatalf("DeclaredAt unknown = %v/%v, want unknown/unknown", got.Type, got.State)
	}
}

func TestSelectEffectiveKeepsKnownDeclarationOverRefinedUnknown(t *testing.T) {
	declared := typ.NewMap(typ.String, typ.Any)

	got := SelectEffective(
		flow.TypedValue{Type: declared, State: flow.StateResolved},
		flow.TypedValue{Type: typ.Unknown, State: flow.StateResolved},
		false,
	)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, declared) {
		t.Fatalf("SelectEffective refined unknown = %v/%v, want declared %v/resolved", got.Type, got.State, declared)
	}
}

func TestTypeFactsEffectiveTypeAtProtectsAnnotatedDeclaration(t *testing.T) {
	const sym cfg.SymbolID = 6

	graph, assign := newTypeFactsLinearGraph(sym)
	solution := flow.Solve(&flow.Inputs{
		Graph:         graph,
		DeclaredTypes: flow.DeclaredTypes{sym: typ.String},
		AnnotatedVars: map[cfg.SymbolID]bool{sym: true},
		Assignments: []flow.UnifiedAssignment{{
			Point:      assign,
			TargetPath: constraint.Path{Symbol: sym},
			Type:       typ.Number,
		}},
		TypeKeys: map[uint64]typ.Type{},
	}, typeFactsTestResolver())

	facts := New(Config{
		Declared:      flow.DeclaredTypes{sym: typ.String},
		AnnotatedVars: map[cfg.SymbolID]bool{sym: true},
		Solution:      solution,
	})

	got := facts.EffectiveTypeAt(graph.Exit(), sym)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, typ.String) {
		t.Fatalf("EffectiveTypeAt annotated mismatch = %v/%v, want declared string/resolved", got.Type, got.State)
	}
}

func TestSelectEffectiveAnnotatedAnyAdoptsProvenRefinement(t *testing.T) {
	refined := typ.NewRecord().Field("kind", typ.LiteralString("event")).Build()

	got := SelectEffective(
		flow.TypedValue{Type: typ.Any, State: flow.StateResolved},
		flow.TypedValue{Type: refined, State: flow.StateResolved},
		true,
	)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, refined) {
		t.Fatalf("SelectEffective annotated any refinement = %v/%v, want %v/resolved", got.Type, got.State, refined)
	}
}

func TestSelectEffectiveAnnotatedConcreteAdoptsSubtypeRefinement(t *testing.T) {
	refined := typ.LiteralString("ready")

	got := SelectEffective(
		flow.TypedValue{Type: typ.String, State: flow.StateResolved},
		flow.TypedValue{Type: refined, State: flow.StateResolved},
		true,
	)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, refined) {
		t.Fatalf("SelectEffective annotated string refinement = %v/%v, want %v/resolved", got.Type, got.State, refined)
	}
}

func TestSelectEffectiveAnnotatedSoftContainerAdoptsPrecisionRefinement(t *testing.T) {
	entry := typ.NewRecord().Field("id", typ.String).Build()
	declared := typ.NewMap(typ.String, typ.NewArray(typ.Any))
	refined := typ.NewRecursive("Flow", func(self typ.Type) typ.Type {
		return typ.NewMap(typ.String, typ.NewArray(entry))
	})

	got := SelectEffective(
		flow.TypedValue{Type: declared, State: flow.StateResolved},
		flow.TypedValue{Type: refined, State: flow.StateResolved},
		true,
	)
	want := typ.NewMap(typ.String, typ.NewArray(entry))
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, want) {
		t.Fatalf("SelectEffective annotated soft container = %v/%v, want %v/resolved", got.Type, got.State, want)
	}
}

func TestSelectEffectiveAnnotatedMapKeepsMapComponentWhenFlowObservationIsRecordFields(t *testing.T) {
	declared := typ.NewMap(typ.String, typ.NewArray(typ.Any))
	refined := typ.NewRecord().
		Field("a", typ.NewTuple(typ.LiteralInt(1))).
		Field("b", typ.NewTuple(typ.LiteralInt(2))).
		SetOpen(true).
		Build()

	got := SelectEffective(
		flow.TypedValue{Type: declared, State: flow.StateResolved},
		flow.TypedValue{Type: refined, State: flow.StateResolved},
		true,
	)
	want := typ.NewMap(typ.String, typ.NewUnion(typ.NewTuple(typ.LiteralInt(1)), typ.NewTuple(typ.LiteralInt(2))))
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, want) {
		t.Fatalf("SelectEffective annotated map with field observation = %v/%v, want %v/resolved", got.Type, got.State, want)
	}
}

func TestSelectEffectiveAnnotatedRecordUsesDeclaredContractOverInitWitnessUnion(t *testing.T) {
	declared := typ.NewRecord().
		Field("run_with", typ.Func().Param("self", typ.Any).Param("db", typ.String).Returns(typ.Any).Build()).
		Build()
	refined := typ.NewUnion(declared, typ.NewRecord().Build())

	got := SelectEffective(
		flow.TypedValue{Type: declared, State: flow.StateResolved},
		flow.TypedValue{Type: refined, State: flow.StateResolved},
		true,
	)
	if got.State != flow.StateResolved || !typ.TypeEquals(got.Type, declared) {
		t.Fatalf("SelectEffective annotated record witness union = %v/%v, want declared %v/resolved", got.Type, got.State, declared)
	}
}

type typeFactsVersionedGraph struct {
	*cfg.CFG
	versions map[cfg.Point]map[cfg.SymbolID]cfg.Version
	decls    map[cfg.SymbolID]cfg.Point
}

func newTypeFactsLinearGraph(sym cfg.SymbolID) (*typeFactsVersionedGraph, cfg.Point) {
	g := cfg.New()
	assign := g.AddNode(cfg.NodeAssign, sym, "")
	g.AddEdge(g.Entry(), assign, true)
	g.AddEdge(assign, g.Exit(), true)

	out := &typeFactsVersionedGraph{
		CFG:      g,
		versions: map[cfg.Point]map[cfg.SymbolID]cfg.Version{},
		decls:    map[cfg.SymbolID]cfg.Point{sym: g.Entry()},
	}
	out.setVersion(g.Entry(), sym, cfg.Version{Root: "v", Symbol: sym, ID: 1})
	out.setVersion(assign, sym, cfg.Version{Root: "v", Symbol: sym, ID: 2})
	out.setVersion(g.Exit(), sym, cfg.Version{Root: "v", Symbol: sym, ID: 2})
	return out, assign
}

func (g *typeFactsVersionedGraph) setVersion(p cfg.Point, sym cfg.SymbolID, ver cfg.Version) {
	if g.versions[p] == nil {
		g.versions[p] = map[cfg.SymbolID]cfg.Version{}
	}
	g.versions[p][sym] = ver
}

func (g *typeFactsVersionedGraph) VisibleVersion(p cfg.Point, sym cfg.SymbolID) cfg.Version {
	if bySym := g.versions[p]; bySym != nil {
		return bySym[sym]
	}
	return cfg.Version{}
}

func (g *typeFactsVersionedGraph) AllVisibleVersions(p cfg.Point) map[cfg.SymbolID]cfg.Version {
	return g.versions[p]
}

func (g *typeFactsVersionedGraph) PhiNodes() []cfg.PhiNode {
	return nil
}

func (g *typeFactsVersionedGraph) SymbolAt(cfg.Point, string) (cfg.SymbolID, bool) {
	return 0, false
}

func (g *typeFactsVersionedGraph) AllSymbolsAt(cfg.Point) map[string]cfg.SymbolID {
	return nil
}

func (g *typeFactsVersionedGraph) DeclarationPoint(sym cfg.SymbolID) (cfg.Point, bool) {
	p, ok := g.decls[sym]
	return p, ok
}

func (g *typeFactsVersionedGraph) NameOf(cfg.SymbolID) string {
	return ""
}

func (g *typeFactsVersionedGraph) SymbolKind(cfg.SymbolID) (cfg.SymbolKind, bool) {
	return cfg.SymbolUnknown, false
}

func (g *typeFactsVersionedGraph) ParamNames() []string {
	return nil
}

func (g *typeFactsVersionedGraph) ParamSymbols() []cfg.SymbolID {
	return nil
}

func (g *typeFactsVersionedGraph) ParamDeclPoints() []cfg.Point {
	return nil
}

func typeFactsTestResolver() narrow.Resolver {
	return &core.FuncResolver{
		FieldFunc: core.Field,
		IndexFunc: core.Index,
	}
}
