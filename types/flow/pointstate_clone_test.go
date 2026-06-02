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
	num := numeric.NewState()
	num.ApplyLenGeConst(path, 1)
	ref := FunctionRef{GraphID: 11, ParentHash: 3}
	closure := ClosureRefOf(FunctionRef{GraphID: 12, ParentHash: 4}, CaptureCellsDomain.Bottom(), nil)
	original := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(sym): product.FromType(typ.String),
		},
		Num:          num,
		FunctionRefs: WithFunctionRef(nil, path, FunctionRefSetOf(ref)),
		ClosureRefs:  WithClosureRef(nil, path, ClosureRefSetOf(closure)),
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
	if got, ok := cloned.IndexWrites.Admission(IndexWriteQuery{
		Target:  constraint.NewPath(sym, ""),
		KeyType: typ.String,
	}); !ok || !typ.TypeEquals(got.ProjectValue(), typ.Number) {
		t.Fatalf("cloned IndexWrites admission = %v/%v, want number/true", got.ProjectValue(), ok)
	}
}
