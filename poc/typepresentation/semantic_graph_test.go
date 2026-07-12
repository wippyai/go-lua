package typepresentation

import (
	"fmt"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPairedRecordUnionDeterministicNestedFunctionLabels(t *testing.T) {
	build := func(label string) TypePair {
		fn := typ.Func().Param(label, typ.String).Returns(typ.Any).Build()
		union := PairUnion(PairFunction(fn), TypePair{Presentation: typ.Nil, Semantic: typ.Nil})
		return PairRecord([]PairedField{{Name: "handler", Type: union}})
	}
	left, right := build("key"), build("component_id")
	if left.Presentation.String() == right.Presentation.String() {
		t.Fatal("presentation labels were lost")
	}
	if left.Semantic.String() != right.Semantic.String() || !typ.TypeEquals(left.Semantic, right.Semantic) {
		t.Fatalf("semantic graphs differ:\n%s\n%s", left.Semantic, right.Semantic)
	}
	if got := left.Semantic.String(); got != "{handler: nil | fun(string) -> any}" {
		t.Fatalf("semantic nested function = %s", got)
	}
}

func TestPairedRecursiveGraphKeepsSeparateClosedIdentities(t *testing.T) {
	recursive := NewPairedRecursive("Node")
	method := PairFunction(typ.Func().Param("node", recursive.Refs().Presentation).Returns(recursive.Refs().Presentation).Build())
	// Rebuild the semantic function with semantic recursive references; this is
	// what a paired constructor receives naturally from child TypePairs.
	semanticMethod := PairFunction(typ.Func().Param("node", recursive.Refs().Semantic).Returns(recursive.Refs().Semantic).Build())
	method.Semantic = semanticMethod.Semantic
	body := PairRecord([]PairedField{{Name: "next", Type: recursive.Refs(), Optional: true}, {Name: "visit", Type: method}})
	recursive.SetBody(body)
	refs := recursive.Refs()
	if refs.Presentation == refs.Semantic || !typ.ContainsRecursive(refs.Presentation) || !typ.ContainsRecursive(refs.Semantic) {
		t.Fatal("paired recursive identities were not closed separately")
	}
	if refs.Presentation.String() == refs.Semantic.String() {
		t.Fatal("recursive presentation label was not separated")
	}
}

func TestLazyGraphMatchesEagerGraph(t *testing.T) {
	eager := largeManifestPair(32)
	lazy := NewLazySemanticGraph(eager.Presentation)
	if !typ.TypeEquals(lazy.Semantic(), eager.Semantic) || lazy.Semantic() != lazy.Semantic() {
		t.Fatal("lazy semantic graph differs or is not stably published")
	}
}

func largeManifestPair(fields int) TypePair {
	out := make([]PairedField, fields)
	for i := range out {
		first := PairFunction(typ.Func().Param(fmt.Sprintf("input_%d", i), typ.String).Returns(typ.Any).Build())
		second := PairFunction(typ.Func().Param(fmt.Sprintf("context_%d", i), typ.Any).Returns(typ.Boolean).Build())
		out[i] = PairedField{Name: fmt.Sprintf("field_%04d", i), Type: PairUnion(first, second, TypePair{Presentation: typ.Nil, Semantic: typ.Nil})}
	}
	return PairRecord(out)
}

func largeManifestPresentation(fields int) typ.Type {
	tableFields := make([]typ.Field, fields)
	for i := range tableFields {
		first := typ.Func().Param(fmt.Sprintf("input_%d", i), typ.String).Returns(typ.Any).Build()
		second := typ.Func().Param(fmt.Sprintf("context_%d", i), typ.Any).Returns(typ.Boolean).Build()
		tableFields[i] = typ.Field{
			Name: fmt.Sprintf("field_%04d", i),
			Type: typ.MaterializeUnion([]typ.Type{first, second, typ.Nil}),
		}
	}
	return typ.RebuildRecord(typ.RecordParts{Fields: tableFields})
}

func BenchmarkEagerPairedLargeManifestConstruction(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = largeManifestPair(256)
	}
}

func BenchmarkLazyLargeManifestConstruction(b *testing.B) {
	presentation := largeManifestPresentation(256)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewLazySemanticGraph(presentation)
	}
}

func BenchmarkPresentationOnlyLargeManifestConstruction(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = largeManifestPresentation(256)
	}
}

func BenchmarkLazyTotalLargeManifestConstruction(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewLazySemanticGraph(largeManifestPresentation(256))
	}
}

func BenchmarkLazyLargeManifestFirstSemanticSelection(b *testing.B) {
	presentation := largeManifestPresentation(256)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = NewLazySemanticGraph(presentation).Semantic()
	}
}

func BenchmarkEagerLargeManifestSteadySelection(b *testing.B) {
	pair := largeManifestPair(256)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if pair.Semantic == nil {
			b.Fatal("nil semantic graph")
		}
	}
}

func BenchmarkLazyLargeManifestSteadySelection(b *testing.B) {
	graph := NewLazySemanticGraph(largeManifestPresentation(256))
	want := graph.Semantic()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if graph.Semantic() != want {
			b.Fatal("unstable semantic graph")
		}
	}
}

func BenchmarkPairedRepresentationSize(b *testing.B) {
	b.ReportMetric(float64(unsafe.Sizeof(TypePair{})), "bytes/eager-root")
	b.ReportMetric(float64(unsafe.Sizeof(LazySemanticGraph{})), "bytes/lazy-root")
	for i := 0; i < b.N; i++ {
	}
}
