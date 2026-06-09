package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPrototypeSelfLatticeLaws(t *testing.T) {
	n := product.FromType(typ.Number)
	s := product.FromType(typ.String)
	optN := product.FromType(typ.NewOptional(typ.Number))

	lattice.LawSuite[PrototypeSelf]{
		Name:   "PrototypeSelf",
		Domain: PrototypeSelfDomain,
		Sample: []PrototypeSelf{
			PrototypeSelfDomain.Bottom(),
			PrototypeSelfDomain.Top(),
			PrototypeSelfOf([]PrototypeSelfEntry{{Prototype: 1, Value: n}}),
			PrototypeSelfOf([]PrototypeSelfEntry{{Prototype: 2, Value: s}}),
			PrototypeSelfOf([]PrototypeSelfEntry{{Prototype: 1, Value: n}, {Prototype: 2, Value: s}}),
			PrototypeSelfOf([]PrototypeSelfEntry{{Prototype: 1, Value: optN}}),
		},
		Format: func(p PrototypeSelf) string { return p.Format() },
	}.Run(t)
}

func TestPrototypeSelfCanonicalization(t *testing.T) {
	got := PrototypeSelfOf([]PrototypeSelfEntry{
		{Prototype: 2, Value: product.FromType(typ.String)},
		{Prototype: 1, Value: product.Domain.Bottom()},
		{Prototype: 2, Value: product.FromType(typ.Number)},
		{Prototype: 0, Value: product.FromType(typ.Boolean)},
	})

	entries := got.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1: %s", len(entries), got.Format())
	}
	if entries[0].Prototype != cfg.SymbolID(2) {
		t.Fatalf("prototype = %d, want 2", entries[0].Prototype)
	}
	want := product.Domain.Join(product.FromType(typ.String), product.FromType(typ.Number))
	if !product.Domain.Equal(entries[0].Value, want) {
		t.Fatalf("merged value = %s, want %s", entries[0].Value.ProjectValue(), want.ProjectValue())
	}
}

func TestPrototypeSelfWithAndJoinValue(t *testing.T) {
	p := PrototypeSelfOf([]PrototypeSelfEntry{{Prototype: 1, Value: product.FromType(typ.Number)}})
	p = p.With(1, product.FromType(typ.String))
	got, ok := p.Value(1)
	if !ok || !product.Domain.Equal(got, product.FromType(typ.String)) {
		t.Fatalf("prototype 1 = %v/%v, want string", got.ProjectValue(), ok)
	}

	p = p.JoinValue(1, product.FromType(typ.Number))
	want := product.Domain.Join(product.FromType(typ.String), product.FromType(typ.Number))
	if got, ok := p.Value(1); !ok || !product.Domain.Equal(got, want) {
		t.Fatalf("joined prototype 1 = %v/%v, want %s", got.ProjectValue(), ok, want.ProjectValue())
	}

	p = p.With(1, product.Domain.Bottom())
	if _, ok := p.Value(1); ok {
		t.Fatalf("bottom update should remove prototype: %s", p.Format())
	}
	if !PrototypeSelfDomain.Equal(p, PrototypeSelfDomain.Bottom()) {
		t.Fatalf("after removing last prototype = %s, want bottom", p.Format())
	}
}

func TestPrototypePointHelpers(t *testing.T) {
	proto := cfg.SymbolID(11)
	otherProto := cfg.SymbolID(12)
	instance := cfg.SymbolID(21)
	otherInstance := cfg.SymbolID(22)
	self := product.FromType(typ.NewRecord().Build())
	otherSelf := product.FromType(typ.NewRecord().Field("label", typ.String).Build())

	if _, ok := ReceiverSelfValueOfPoint(nil, proto); ok {
		t.Fatal("nil point had receiver-self value")
	}
	if _, ok := PrototypeInstancePrototypesOfPoint(nil, instance); ok {
		t.Fatal("nil point had prototype-instance binding")
	}

	var state PointState
	if !RecordPrototypeSelf(&state, proto, self) {
		t.Fatal("RecordPrototypeSelf reported no change")
	}
	if !JoinPrototypeSelf(&state, PrototypeSelfOf([]PrototypeSelfEntry{{Prototype: otherProto, Value: otherSelf}})) {
		t.Fatal("JoinPrototypeSelf reported no change")
	}
	if got, ok := ReceiverSelfValueOfPoint(&state, proto); !ok || !product.Domain.Equal(got, self) {
		t.Fatalf("receiver self = %v/%v, want %s", got.ProjectValue(), ok, self.ProjectValue())
	}
	if got := PrototypeSelfOfPoint(&state); !PrototypeSelfDomain.Equal(got, PrototypeSelfAxisOf(state)) {
		t.Fatalf("prototype self point read = %s, axis read = %s", got.Format(), PrototypeSelfAxisOf(state).Format())
	}

	if !BindPrototypeInstance(&state, instance, proto) {
		t.Fatal("BindPrototypeInstance reported no change")
	}
	if !JoinPrototypeInstances(&state, PrototypeInstancesOf([]PrototypeInstanceEntry{{Symbol: otherInstance, Prototypes: []cfg.SymbolID{otherProto}}})) {
		t.Fatal("JoinPrototypeInstances reported no change")
	}
	protos, ok := PrototypeInstancePrototypesOfPoint(&state, instance)
	if !ok || len(protos) != 1 || protos[0] != proto {
		t.Fatalf("prototype instances = %v/%v, want [%d]", protos, ok, proto)
	}
	if got := PrototypeInstancesOfPoint(&state); !PrototypeInstancesDomain.Equal(got, PrototypeInstancesAxisOf(state)) {
		t.Fatalf("prototype instances point read = %s, axis read = %s", got.Format(), PrototypeInstancesAxisOf(state).Format())
	}
	if !ClearPrototypeInstance(&state, instance) {
		t.Fatal("ClearPrototypeInstance reported no change")
	}
	if _, ok := PrototypeInstancePrototypesOfPoint(&state, instance); ok {
		t.Fatalf("prototype instance survived clear: %s", PrototypeInstancesOfPoint(&state).Format())
	}
}

func TestPrototypeInstancesLatticeLaws(t *testing.T) {
	lattice.LawSuite[PrototypeInstances]{
		Name:   "PrototypeInstances",
		Domain: PrototypeInstancesDomain,
		Sample: []PrototypeInstances{
			PrototypeInstancesDomain.Bottom(),
			PrototypeInstancesDomain.Top(),
			PrototypeInstancesOf([]PrototypeInstanceEntry{{Symbol: 1, Prototypes: []cfg.SymbolID{10}}}),
			PrototypeInstancesOf([]PrototypeInstanceEntry{{Symbol: 2, Prototypes: []cfg.SymbolID{20}}}),
			PrototypeInstancesOf([]PrototypeInstanceEntry{{Symbol: 1, Prototypes: []cfg.SymbolID{10, 20}}}),
			PrototypeInstancesOf([]PrototypeInstanceEntry{
				{Symbol: 1, Prototypes: []cfg.SymbolID{10}},
				{Symbol: 2, Prototypes: []cfg.SymbolID{20}},
			}),
		},
		Format: func(p PrototypeInstances) string { return p.Format() },
	}.Run(t)
}

func TestPrototypeInstancesCanonicalizationAndStrongUpdate(t *testing.T) {
	got := PrototypeInstancesOf([]PrototypeInstanceEntry{
		{Symbol: 2, Prototypes: []cfg.SymbolID{30, 20, 20}},
		{Symbol: 1, Prototypes: nil},
		{Symbol: 2, Prototypes: []cfg.SymbolID{10}},
		{Symbol: 0, Prototypes: []cfg.SymbolID{99}},
	})
	protos, ok := got.Prototypes(2)
	if !ok || len(protos) != 3 || protos[0] != 10 || protos[1] != 20 || protos[2] != 30 {
		t.Fatalf("prototypes for 2 = %v/%v; map=%s", protos, ok, got.Format())
	}

	got = got.WithPrototype(2, 40)
	protos, ok = got.Prototypes(2)
	if !ok || len(protos) != 1 || protos[0] != 40 {
		t.Fatalf("strong update prototypes for 2 = %v/%v; map=%s", protos, ok, got.Format())
	}

	got = got.With(2, nil)
	if _, ok := got.Prototypes(2); ok {
		t.Fatalf("empty update should remove symbol: %s", got.Format())
	}
}
