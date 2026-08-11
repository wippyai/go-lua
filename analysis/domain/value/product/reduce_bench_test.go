package product

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func benchReg() *axis.Registry {
	reg := axis.NewRegistry()
	axis.Register(reg, runtimekind.Spec())
	axis.Register(reg, typewitness.Spec())
	freezeTestRegistry(reg)
	return reg
}

func BenchmarkSetScalarWitness(b *testing.B) {
	reg := benchReg()
	wit := typewitness.Of(typ.String)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Set(reg, Top(), typewitness.Key, wit)
	}
}

func BenchmarkJoinWitness(b *testing.B) {
	reg := benchReg()
	a := Set(reg, Top(), typewitness.Key, typewitness.Of(typ.Number))
	c := Set(reg, Top(), typewitness.Key, typewitness.Of(typ.String))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Join(reg, a, c)
	}
}

func BenchmarkLessOrEqWitness(b *testing.B) {
	reg := benchReg()
	a := Set(reg, Top(), typewitness.Key, typewitness.Of(typ.Number))
	c := Set(reg, Top(), typewitness.Key, typewitness.Of(typ.MaterializeUnion([]typ.Type{typ.Number, typ.String})))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = LessOrEq(reg, a, c)
	}
}

func BenchmarkSetRuntimeKindOnUnionWitness(b *testing.B) {
	reg := benchReg()
	base := Set(reg, Top(), typewitness.Key, typewitness.Of(typ.MaterializeUnion([]typ.Type{typ.Number, typ.String})))
	rk := runtimekind.Top().Without(runtimekind.Number)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Set(reg, base, runtimekind.Key, rk)
	}
}

func BenchmarkEditTwoAxes(b *testing.B) {
	reg := benchReg()
	wit := typewitness.Of(typ.String)
	runtime := runtimekind.Singleton(runtimekind.String)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ed := Edit(reg, Top())
		EditSet(&ed, typewitness.Key, wit)
		EditSet(&ed, runtimekind.Key, runtime)
		_ = ed.Done()
	}
}
