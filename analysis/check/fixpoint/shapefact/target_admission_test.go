package shapefact_test

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestTargetAdmissionCanonicalizesToStructuralHandle(t *testing.T) {
	firstSource := typ.Func().Param("left", typ.String).Returns(typ.Integer).Build()
	secondSource := typ.Func().Param("right", typ.String).Returns(typ.Integer).Build()
	firstWire, firstOK := shapefact.EncodeTarget(firstSource)
	secondWire, secondOK := shapefact.EncodeTarget(secondSource)
	if !firstOK || !secondOK || !bytes.Equal(firstWire, secondWire) {
		t.Fatalf("canonical wires = %q/%v and %q/%v", firstWire, firstOK, secondWire, secondOK)
	}
	first, firstDecoded := shapefact.DecodeTarget(firstWire)
	second, secondDecoded := shapefact.DecodeTarget(secondWire)
	if !firstDecoded || !secondDecoded || first != second {
		t.Fatalf("decoded handles = %T/%v and %T/%v", first, firstDecoded, second, secondDecoded)
	}
	if first == firstSource || first == secondSource {
		t.Fatal("shape admission retained a source declaration handle")
	}
}

func TestDecodeTargetReusesAdmittedHandleWithoutAllocating(t *testing.T) {
	encoded, ok := shapefact.EncodeTarget(typ.Func().
		Param("name", typ.String).
		Param("count", typ.Integer).
		Returns(typ.MaterializeOptional(typ.String)).
		Build())
	if !ok {
		t.Fatal("EncodeTarget rejected fixture")
	}
	first, ok := shapefact.DecodeTarget(encoded)
	if !ok {
		t.Fatal("DecodeTarget rejected admitted fixture")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		next, decoded := shapefact.DecodeTarget(encoded)
		if !decoded || next != first {
			t.Fatalf("DecodeTarget = %T/%v, want admitted handle %T", next, decoded, first)
		}
	}); allocations != 0 {
		t.Fatalf("warm DecodeTarget allocated %v times", allocations)
	}
}

func BenchmarkDecodeTargetAdmitted(b *testing.B) {
	encoded, ok := shapefact.EncodeTarget(typ.Func().
		Param("name", typ.String).
		Param("count", typ.Integer).
		Returns(typ.MaterializeOptional(typ.String)).
		Build())
	if !ok {
		b.Fatal("EncodeTarget rejected fixture")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, decoded := shapefact.DecodeTarget(encoded); !decoded {
			b.Fatal("DecodeTarget rejected admitted fixture")
		}
	}
}
