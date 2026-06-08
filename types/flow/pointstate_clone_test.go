package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
)

func TestClonePointStateCopiesMutableCarriers(t *testing.T) {
	const sym = cfg.SymbolID(1)
	const other = cfg.SymbolID(2)
	path := SymbolPathKey(sym, nil)
	otherPath := SymbolPathKey(other, nil)
	addr, _ := StableAddressOfSymbol(sym, nil)
	num := numeric.NewState()
	num.ApplyLenGeConst(path, 1)
	ref := FunctionRef{GraphID: 11, ParentHash: 3}
	closure := ClosureRefOf(FunctionRef{GraphID: 12, ParentHash: 4}, CaptureCellsDomain.Bottom(), nil)
	original := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.String),
		},
		Num:          num,
		FunctionRefs: WithFunctionRefAddress(nil, addr, FunctionRefSetOf(ref)),
		ClosureRefs:  WithClosureRefAddress(nil, addr, ClosureRefSetOf(closure)),
	}

	cloned := ClonePointState(original)
	cloned.Env[SymbolValueKey(sym)] = product.FromType(typ.Number)
	cloned.Env[SymbolValueKey(other)] = product.FromType(typ.Boolean)
	cloned.Num.ApplyLenGeConst(path, 5)
	cloned.FunctionRefs[otherPath] = FunctionRefSetOf(FunctionRef{GraphID: 99})
	cloned.ClosureRefs[otherPath] = ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 100}, CaptureCellsDomain.Bottom(), nil))

	got, ok := original.Env[SymbolValueKey(sym)]
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("original Env[%s] = %v/%v, want string/true", SymbolValueKey(sym), got.ProjectValue(), ok)
	}
	if _, ok := original.Env[SymbolValueKey(other)]; ok {
		t.Fatalf("clone Env mutation leaked into original Env[%s]", SymbolValueKey(other))
	}
	lower, _, ok := original.Num.LenBoundsFor(path)
	if !ok || lower != 1 {
		t.Fatalf("original Num len lower = %d/%v, want 1/true", lower, ok)
	}
	if _, ok := original.FunctionRefs[otherPath]; ok {
		t.Fatalf("clone FunctionRefs mutation leaked into original")
	}
	if _, ok := original.ClosureRefs[otherPath]; ok {
		t.Fatalf("clone ClosureRefs mutation leaked into original")
	}
}

func TestClonePointStateForEdgeFactEffectCopiesOnlyEdgeFactMutableAxes(t *testing.T) {
	const sym = cfg.SymbolID(1)
	const other = cfg.SymbolID(2)
	path := SymbolPathKey(sym, nil)
	otherPath := SymbolPathKey(other, nil)
	addr, _ := StableAddressOfSymbol(sym, nil)
	num := numeric.NewState()
	num.ApplyLenGeConst(path, 1)
	ref := FunctionRef{GraphID: 11, ParentHash: 3}
	closure := ClosureRefOf(FunctionRef{GraphID: 12, ParentHash: 4}, CaptureCellsDomain.Bottom(), nil)
	original := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.String),
		},
		Num:          num,
		FunctionRefs: WithFunctionRefAddress(nil, addr, FunctionRefSetOf(ref)),
		ClosureRefs:  WithClosureRefAddress(nil, addr, ClosureRefSetOf(closure)),
	}

	cloned := ClonePointStateForEdgeFactEffect(original)
	cloned.Env[SymbolValueKey(sym)] = product.FromType(typ.Number)
	cloned.Env[SymbolValueKey(other)] = product.FromType(typ.Boolean)

	got, ok := original.Env[SymbolValueKey(sym)]
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("original Env[%s] = %v/%v, want string/true", SymbolValueKey(sym), got.ProjectValue(), ok)
	}
	if _, ok := original.Env[SymbolValueKey(other)]; ok {
		t.Fatalf("edge-fact clone Env mutation leaked into original Env[%s]", SymbolValueKey(other))
	}
	if cloned.Num != original.Num {
		t.Fatalf("edge-fact clone copied numeric state; edge proof effects do not mutate Num")
	}
	cloned.FunctionRefs[otherPath] = FunctionRefSetOf(FunctionRef{GraphID: 99})
	if _, ok := original.FunctionRefs[otherPath]; !ok {
		t.Fatalf("edge-fact clone copied FunctionRefs; edge proof effects do not mutate them")
	}
	cloned.ClosureRefs[otherPath] = ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 100}, CaptureCellsDomain.Bottom(), nil))
	if _, ok := original.ClosureRefs[otherPath]; !ok {
		t.Fatalf("edge-fact clone copied ClosureRefs; edge proof effects do not mutate them")
	}
}

func TestClonePointStateCanonicalizesMutableFiniteMaps(t *testing.T) {
	const sym = cfg.SymbolID(1)
	path := SymbolPathKey(sym, nil)
	original := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.Bottom(),
		},
		FunctionRefs: FunctionRefs{
			path: FunctionRefSet{},
		},
		ClosureRefs: ClosureRefs{
			path: ClosureRefSet{},
		},
	}

	cloned := ClonePointState(original)
	if len(cloned.Env) != 0 {
		t.Fatalf("cloned Env retained bottom entries: %#v", cloned.Env)
	}
	if len(cloned.FunctionRefs) != 0 {
		t.Fatalf("cloned FunctionRefs retained bottom entries: %#v", cloned.FunctionRefs)
	}
	if len(cloned.ClosureRefs) != 0 {
		t.Fatalf("cloned ClosureRefs retained bottom entries: %#v", cloned.ClosureRefs)
	}
}

func TestClonePointStatePreservesMutableMapTopSentinels(t *testing.T) {
	original := PointState{
		Env:          envDomain.Top(),
		FunctionRefs: FunctionRefsDomain.Top(),
		ClosureRefs:  ClosureRefsDomain.Top(),
	}

	cloned := ClonePointState(original)
	if !envDomain.Equal(cloned.Env, envDomain.Top()) {
		t.Fatalf("cloned Env did not preserve top sentinel")
	}
	if !FunctionRefsDomain.Equal(cloned.FunctionRefs, FunctionRefsDomain.Top()) {
		t.Fatalf("cloned FunctionRefs did not preserve top sentinel")
	}
	if !ClosureRefsDomain.Equal(cloned.ClosureRefs, ClosureRefsDomain.Top()) {
		t.Fatalf("cloned ClosureRefs did not preserve top sentinel")
	}
}

func TestClonePointStatePreservesPersistentAxes(t *testing.T) {
	const proto = cfg.SymbolID(42)
	const sym = cfg.SymbolID(43)
	self := product.FromType(typ.NewRecord().Field("node_id", typ.String).Build())
	cell := product.FromType(typ.Number)
	original := PointState{
		Cells:              CaptureCellsDomain.Bottom().With(sym, cell),
		CellEffects:        CaptureEffectsIdentity().WithMustWrite(sym, cell),
		PrototypeSelf:      PrototypeSelfOf([]PrototypeSelfEntry{{Prototype: proto, Value: self}}),
		PrototypeInstances: PrototypeInstancesOf([]PrototypeInstanceEntry{{Symbol: sym, Prototypes: []cfg.SymbolID{proto}}}),
		PathAliases: PathAliasFacts{}.WithAddresses(
			testStableAddressPath(t, constraint.NewPath(sym, "alias")),
			testStableAddressPath(t, constraint.NewPath(cfg.SymbolID(44), "source")),
		),
		IndexWrites: IndexWriteAdmissionFacts{}.With(IndexWriteAdmissionFact{
			Target: SymbolPathKey(sym, nil),
			Key:    product.FromType(typ.String),
			Value:  product.FromType(typ.Number),
		}),
	}

	cloned := ClonePointState(original)
	if got, ok := cloned.Cells.Value(sym); !ok || !product.Domain.Equal(got, cell) {
		t.Fatalf("cloned Cells[%d] = %v/%v, want %v/true", sym, got.ProjectValue(), ok, cell.ProjectValue())
	}
	if effects := cloned.CellEffects.Entries(); len(effects) != 1 || effects[0].Symbol != sym {
		t.Fatalf("cloned CellEffects = %s, want one effect for %d", cloned.CellEffects.Format(), sym)
	}
	if got, ok := cloned.PrototypeSelf.Value(proto); !ok || !product.Domain.Equal(got, self) {
		t.Fatalf("cloned PrototypeSelf[%d] = %v/%v, want %v/true", proto, got.ProjectValue(), ok, self.ProjectValue())
	}
	if protos, ok := cloned.PrototypeInstances.Prototypes(sym); !ok || len(protos) != 1 || protos[0] != proto {
		t.Fatalf("cloned PrototypeInstances[%d] = %v/%v, want [%d]/true", sym, protos, ok, proto)
	}
	if aliases := cloned.PathAliases.AliasesOfAddress(testStableAddressPath(t, constraint.NewPath(sym, "alias"))); len(aliases) != 1 {
		t.Fatalf("cloned PathAliases = %s, want one alias", cloned.PathAliases.Format())
	}
	if got, ok := cloned.IndexWrites.AdmissionAtAddress(testIndexWriteAddressQuery(t, constraint.NewPath(sym, ""), constraint.Path{}, typ.String, constraint.Path{})); !ok || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("cloned IndexWrites admission = %v/%v, want number/true", got.ProjectValue(), ok)
	}
}
