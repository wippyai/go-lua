package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
)

func TestClonePointStateUsesCopyOnWriteEnvAndSharesPersistentReferenceAxes(t *testing.T) {
	const sym = cfg.SymbolID(1)
	const other = cfg.SymbolID(2)
	path := SymbolPathKey(sym, nil)
	addr, _ := StableAddressOfSymbol(sym, nil)
	otherAddr, _ := StableAddressOfSymbol(other, nil)
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
	writer := NewPointWriter(&cloned)
	writer.WriteValueKey(SymbolValueKey(sym), product.FromType(typ.Number), false)
	writer.WriteValueKey(SymbolValueKey(other), product.FromType(typ.Boolean), false)
	cloned.Num.ApplyLenGeConst(path, 5)
	cloned.FunctionRefs = WithFunctionRefAddress(cloned.FunctionRefs, otherAddr, FunctionRefSetOf(FunctionRef{GraphID: 99}))
	cloned.ClosureRefs = WithClosureRefAddress(cloned.ClosureRefs, otherAddr, ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 100}, CaptureCellsDomain.Bottom(), nil)))

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
	if _, ok := FunctionRefAtAddress(original.FunctionRefs, otherAddr); ok {
		t.Fatalf("persistent FunctionRefs update leaked into original")
	}
	if _, ok := ClosureRefAtAddress(original.ClosureRefs, otherAddr); ok {
		t.Fatalf("persistent ClosureRefs update leaked into original")
	}
}

func TestClonePointStateForEdgeFactEffectCopiesOnlyEdgeFactMutableAxes(t *testing.T) {
	const sym = cfg.SymbolID(1)
	const other = cfg.SymbolID(2)
	path := SymbolPathKey(sym, nil)
	addr, _ := StableAddressOfSymbol(sym, nil)
	otherAddr, _ := StableAddressOfSymbol(other, nil)
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
	writer := NewPointWriter(&cloned)
	writer.WriteValueKey(SymbolValueKey(sym), product.FromType(typ.Number), false)
	writer.WriteValueKey(SymbolValueKey(other), product.FromType(typ.Boolean), false)

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
	cloned.FunctionRefs = WithFunctionRefAddress(cloned.FunctionRefs, otherAddr, FunctionRefSetOf(FunctionRef{GraphID: 99}))
	if _, ok := FunctionRefAtAddress(original.FunctionRefs, otherAddr); ok {
		t.Fatalf("persistent FunctionRefs update leaked into original")
	}
	cloned.ClosureRefs = WithClosureRefAddress(cloned.ClosureRefs, otherAddr, ClosureRefSetOf(ClosureRefOf(FunctionRef{GraphID: 100}, CaptureCellsDomain.Bottom(), nil)))
	if _, ok := ClosureRefAtAddress(original.ClosureRefs, otherAddr); ok {
		t.Fatalf("persistent ClosureRefs update leaked into original")
	}
}

func TestDetachPointStateEnvCopiesBorrowedEnvOnly(t *testing.T) {
	const sym = cfg.SymbolID(1)
	const other = cfg.SymbolID(2)
	original := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.String),
		},
	}
	borrowed := original

	DetachPointStateEnv(&borrowed)
	borrowed.Env[SymbolValueKey(sym)] = product.FromType(typ.Number)
	borrowed.Env[SymbolValueKey(other)] = product.FromType(typ.Boolean)

	got, ok := original.Env[SymbolValueKey(sym)]
	if !ok || !typ.TypeEquals(got.ProjectValue(), typ.String) {
		t.Fatalf("original Env[%s] = %v/%v, want string/true", SymbolValueKey(sym), got.ProjectValue(), ok)
	}
	if _, ok := original.Env[SymbolValueKey(other)]; ok {
		t.Fatalf("borrowed Env mutation leaked into original Env[%s]", SymbolValueKey(other))
	}
}

func TestPointWriterCanonicalizesBottomEnvWrites(t *testing.T) {
	const sym = cfg.SymbolID(1)
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.String),
		},
	}

	if !NewPointWriter(&state).WriteValueKey(SymbolValueKey(sym), product.Bottom(), false) {
		t.Fatalf("bottom write should delete existing Env key")
	}
	if len(state.Env) != 0 {
		t.Fatalf("bottom Env write retained entries: %#v", state.Env)
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
