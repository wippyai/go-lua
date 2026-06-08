package canonical

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/functionsymbols"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestPathProjectorCallableRefPreservesMapReadOptionality(t *testing.T) {
	t.Parallel()

	point := cfg.Point(1)
	sym := cfg.SymbolID(10)
	ref := summary.FuncRef{GraphID: 101}
	handlerSig := typ.Func().Param("state", typ.String).Build()
	rootType := typ.NewRecord().
		Field("handlers", typ.NewMap(typ.String, handlerSig)).
		Build()
	path := constraint.NewPath(sym, "app").Field("handlers").IndexStr("missing")

	projector := pathProjector{
		state: state.FunctionState{
			InPoints: map[cfg.Point]flow.PointState{
				point: {
					Env: map[flow.ValueKey]product.AbstractValue{
						flow.SymbolValueKey(sym): product.FromType(rootType),
					},
					FunctionRefs: flow.WithFunctionRefPath(nil, path, flow.FunctionRefSetOf(canonref.ToFlow(ref))),
				},
			},
		},
		callables: testCallableProjector(ref, handlerSig),
	}

	got := projector.RefinedPathAt(point, path)
	if got.State != flow.StateResolved {
		t.Fatalf("RefinedPathAt state = %v, want resolved", got.State)
	}
	inner, optional := typ.SplitNilableFieldType(got.Type)
	if !optional {
		t.Fatalf("RefinedPathAt type = %v, want optional callable", got.Type)
	}
	if unwrap.Function(inner) == nil {
		t.Fatalf("RefinedPathAt inner = %v, want function", inner)
	}
}

func TestPathProjectorCallableRefRefinesMustPresentPath(t *testing.T) {
	t.Parallel()

	point := cfg.Point(1)
	sym := cfg.SymbolID(11)
	ref := summary.FuncRef{GraphID: 102}
	declaredSig := typ.Func().Returns(typ.Unknown).Build()
	solvedSig := typ.Func().Returns(typ.String).Build()
	path := constraint.NewPath(sym, "app").Field("run")

	projector := pathProjector{
		state: state.FunctionState{
			InPoints: map[cfg.Point]flow.PointState{
				point: {
					Env: map[flow.ValueKey]product.AbstractValue{
						flow.SymbolValueKey(sym): product.FromType(typ.NewRecord().Field("run", declaredSig).Build()),
					},
					FunctionRefs: flow.WithFunctionRefPath(nil, path, flow.FunctionRefSetOf(canonref.ToFlow(ref))),
				},
			},
		},
		callables: testCallableProjector(ref, solvedSig),
	}

	got := projector.RefinedPathAt(point, path)
	fn := unwrap.Function(got.Type)
	if got.State != flow.StateResolved || fn == nil {
		t.Fatalf("RefinedPathAt = (%v, %v), want resolved function", got.Type, got.State)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("returns = %#v, want solved [string]", fn.Returns)
	}
}

func TestObservePathCallableRefPreservesDeclaredMapReadOptionalityWithoutProductValue(t *testing.T) {
	t.Parallel()

	point := cfg.Point(1)
	sym := cfg.SymbolID(12)
	ref := summary.FuncRef{GraphID: 103}
	handlerSig := typ.Func().Returns(typ.String).Build()
	rootType := typ.NewRecord().
		Field("handlers", typ.NewMap(typ.String, handlerSig)).
		Build()
	path := constraint.NewPath(sym, "app").Field("handlers").IndexStr("missing")
	fs := state.FunctionState{
		InPoints: map[cfg.Point]flow.PointState{
			point: {
				FunctionRefs: flow.WithFunctionRefPath(nil, path, flow.FunctionRefSetOf(canonref.ToFlow(ref))),
			},
		},
	}
	facts := &canonicalFacts{
		state:    fs,
		declared: map[cfg.SymbolID]typ.Type{sym: rootType},
		annotate: flow.AnnotatedSymbolsFromMap(map[cfg.SymbolID]bool{sym: true}),
		paths:    newPathProjector(fs, canonicalTestSymbolSet(), testCallableProjector(ref, handlerSig)),
	}

	got := facts.ObservePath(flow.PathObservationQuery{Point: point, Path: path})
	if !got.Resolved() {
		t.Fatalf("ObservePath = %#v, want resolved", got)
	}
	inner, optional := typ.SplitNilableFieldType(got.Type)
	if !optional || unwrap.Function(inner) == nil {
		t.Fatalf("ObservePath type = %v, want optional callable", got.Type)
	}
}

func TestPathProjectorUnannotatedParamPreciseRootDoesNotFabricateGradualDescendant(t *testing.T) {
	t.Parallel()

	point := cfg.Point(1)
	sym := cfg.SymbolID(18)
	projector := pathProjector{
		state: state.FunctionState{
			InPoints: map[cfg.Point]flow.PointState{
				point: {
					Env: map[flow.ValueKey]product.AbstractValue{
						flow.SymbolValueKey(sym): product.FromType(typ.NewRecord().Field("session_id", typ.String).Build()),
					},
				},
			},
		},
		unannotatedParams: canonicalTestSymbolSet(sym),
	}

	present := projector.RefinedPathAt(point, constraint.NewPath(sym, "self").Field("session_id"))
	if present.State != flow.StateResolved || !typ.TypeEquals(present.Type, typ.String) {
		t.Fatalf("present descendant = (%v, %v), want resolved string", present.Type, present.State)
	}
	missing := projector.RefinedPathValueAt(point, constraint.NewPath(sym, "self").Field("user_id"))
	if missing.State == flow.StateResolved {
		t.Fatalf("missing descendant resolved as %v, want no fabricated gradual child", missing.Value.ProjectValue())
	}
}

func TestObservePathCallableRefKeepsMustPresentProductValueDefinite(t *testing.T) {
	t.Parallel()

	point := cfg.Point(1)
	sym := cfg.SymbolID(13)
	ref := summary.FuncRef{GraphID: 104}
	declaredSig := typ.Func().Returns(typ.Unknown).Build()
	solvedSig := typ.Func().Returns(typ.String).Build()
	rootType := typ.NewRecord().
		Field("handlers", typ.NewMap(typ.String, declaredSig)).
		Build()
	path := constraint.NewPath(sym, "app").Field("handlers").IndexStr("present")
	addr, ok := flow.StableAddressOfSymbol(sym, path.Segments)
	if !ok {
		t.Fatal("static-member address did not build")
	}
	fs := state.FunctionState{
		InPoints: map[cfg.Point]flow.PointState{
			point: {
				StaticMembers: flow.StaticMemberFactsDomain.Top().
					WithAddress(addr, product.FromType(declaredSig)),
				FunctionRefs: flow.WithFunctionRefPath(nil, path, flow.FunctionRefSetOf(canonref.ToFlow(ref))),
			},
		},
	}
	facts := &canonicalFacts{
		state:    fs,
		declared: map[cfg.SymbolID]typ.Type{sym: rootType},
		annotate: flow.AnnotatedSymbolsFromMap(map[cfg.SymbolID]bool{sym: true}),
		paths:    newPathProjector(fs, canonicalTestSymbolSet(), testCallableProjector(ref, solvedSig)),
	}

	got := facts.ObservePath(flow.PathObservationQuery{Point: point, Path: path})
	fn := unwrap.Function(got.Type)
	if !got.Resolved() || fn == nil {
		t.Fatalf("ObservePath = %#v, want definite function", got)
	}
	if _, optional := typ.SplitNilableFieldType(got.Type); optional {
		t.Fatalf("ObservePath type = %v, want must-present function", got.Type)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.String) {
		t.Fatalf("returns = %#v, want solved [string]", fn.Returns)
	}
}

func testCallableProjector(ref summary.FuncRef, sig *typ.Function) callableProjector {
	return callableProjector{
		prog: &program{declaredReturns: map[summary.FuncRef][]typ.Type{}},
		reader: summary.NewReader(nil, nil, map[summary.FuncRef]summary.Summary{
			ref: {},
		}),
		baseSignature: func(got summary.FuncRef) *typ.Function {
			if got != ref {
				return nil
			}
			return sig
		},
	}
}

func canonicalTestSymbolSet(syms ...cfg.SymbolID) functionsymbols.Set {
	var set functionsymbols.Set
	for _, sym := range syms {
		set.Add(sym)
	}
	return set
}
