package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func BenchmarkStateDomainJoinMostlyEqual(b *testing.B) {
	reg := standard.Registry()
	ks := keyspace.New()
	domain := Domain(reg)
	slot := key.SymbolValue(symbol.ID(41))
	tableID := identity.ID{Kind: "table", Site: "join-mostly-equal", Index: 1}
	pathKey := pathdom.PathKey("sym41@1.value")
	stateKey, ok := pathaddr.StateKeyFromPathKey(pathKey)
	if !ok {
		b.Fatalf("StateKeyFromPathKey(%q) failed", pathKey)
	}
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		b.Fatalf("FromPathKey(%q) failed", pathKey)
	}

	prev := domain.Bottom().
		WriteValue(reg, slot, presentValue(reg)).
		WriteLocalPathKey(reg, localKey, presentValue(reg)).
		FreezeTable(tableID)
	next := prev.WriteNumFloor(ks, stateKey, 1)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = domain.Join(prev, next)
	}
}

func BenchmarkStateDomainJoinIdentical(b *testing.B) {
	reg := standard.Registry()
	domain := Domain(reg)
	slot := key.SymbolValue(symbol.ID(42))
	state := domain.Bottom().WriteValue(reg, slot, presentValue(reg))

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = domain.Join(state, state)
	}
}

func BenchmarkStateSeedValues(b *testing.B) {
	reg := standard.Registry()
	base := Domain(reg).Bottom()
	value := typevalue.FromType(reg, typ.String)
	seeds := make([]ValueSeed, 16)
	for i := range seeds {
		seeds[i] = ValueSeed{
			Slot:  key.SymbolValue(symbol.ID(5000 + i)),
			Value: value,
		}
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = base.SeedValues(reg, seeds)
	}
}

func BenchmarkStateValueEditMultipleWrites(b *testing.B) {
	reg := standard.Registry()
	base := Domain(reg).Bottom()
	value := typevalue.FromType(reg, typ.String)
	slots := make([]key.Value, 16)
	for i := range slots {
		slots[i] = key.SymbolValue(symbol.ID(6000 + i))
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		edit := base.EditValues(reg)
		for _, slot := range slots {
			edit.Write(slot, value)
		}
		_ = edit.Done()
	}
}

func BenchmarkStateSequentialMultipleWrites(b *testing.B) {
	reg := standard.Registry()
	base := Domain(reg).Bottom()
	value := typevalue.FromType(reg, typ.String)
	slots := make([]key.Value, 16)
	for i := range slots {
		slots[i] = key.SymbolValue(symbol.ID(7000 + i))
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		state := base
		for _, slot := range slots {
			state = state.WriteValue(reg, slot, value)
		}
	}
}
