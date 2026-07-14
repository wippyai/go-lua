package typepresentation

import (
	"fmt"
	"sync"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/type/annotation"
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

func TestLazyGraphProjectsEveryCompositeEdge(t *testing.T) {
	left := comprehensivePresentation("left_argument", false)
	right := comprehensivePresentation("right_argument", true)
	if left.String() == right.String() {
		t.Fatal("presentation graph unexpectedly discarded nested labels/order")
	}

	leftSemantic := NewLazySemanticGraph(left).Semantic()
	rightSemantic := NewLazySemanticGraph(right).Semantic()
	if !typ.TypeEquals(leftSemantic, rightSemantic) {
		t.Fatalf("semantic composite graphs differ:\n%s\n%s", leftSemantic, rightSemantic)
	}
	if leftSemantic.String() != rightSemantic.String() {
		t.Fatalf("semantic strings depend on construction order or labels:\n%s\n%s", leftSemantic, rightSemantic)
	}
	if got := leftSemantic.String(); containsAny(got, "left_argument", "right_argument") {
		t.Fatalf("semantic graph retained a presentation label: %s", got)
	}
}

func TestSemanticProjectionIsInvariantAcrossLabelAndConstructionOrder(t *testing.T) {
	want := NewLazySemanticGraph(comprehensivePresentation("parameter_000", false)).Semantic()
	wantString := want.String()
	wantHash := typ.EqualityHash(want)
	for i := 1; i <= 64; i++ {
		candidate := NewLazySemanticGraph(comprehensivePresentation(fmt.Sprintf("parameter_%03d", i), i%2 == 0)).Semantic()
		if !typ.TypeEquals(want, candidate) {
			t.Fatalf("case %d: projected type differs", i)
		}
		if got := candidate.String(); got != wantString {
			t.Fatalf("case %d: semantic string differs:\nwant %s\n got %s", i, wantString, got)
		}
		if got := typ.EqualityHash(candidate); got != wantHash {
			t.Fatalf("case %d: equality hash differs: %x != %x", i, got, wantHash)
		}
	}
}

func TestLazyGraphProjectsRecursiveGenericGraph(t *testing.T) {
	build := func(label string) typ.Type {
		recursive := typ.NewRecursivePlaceholder("Node")
		param := typ.NewTypeParam("T", typ.Func().Param(label, typ.String).Returns(typ.Any).Build())
		generic := typ.NewGeneric("Box", []*typ.TypeParam{param}, nil)
		generic.SetBody(typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{
			{Name: "next", Type: typ.MaterializeOptional(recursive)},
			{Name: "value", Type: param},
		}}))
		recursive.SetBody(typ.RebuildRecord(typ.RecordParts{Fields: []typ.Field{
			{Name: "box", Type: typ.Instantiate(generic, typ.Func().Param(label, typ.Number).Returns(typ.String).Build())},
		}}))
		return recursive
	}

	left := NewLazySemanticGraph(build("left")).Semantic()
	right := NewLazySemanticGraph(build("right")).Semantic()
	equal, recursive, generic := typ.TypeEquals(left, right), typ.ContainsRecursive(left), typ.ContainsGeneric(left)
	if !equal || !recursive || !generic {
		t.Fatalf("recursive/generic projection lost closure or retained labels: equal=%t recursive=%t generic=%t", equal, recursive, generic)
	}
}

func TestSemanticGraphCachePrewarmPublishesOneStableGraph(t *testing.T) {
	root := comprehensivePresentation("source_name", false)
	cache := NewSemanticGraphCache()
	cache.Prewarm(root, root)
	want := cache.Semantic(root)

	const readers = 32
	var wg sync.WaitGroup
	wg.Add(readers)
	for range readers {
		go func() {
			defer wg.Done()
			if got := cache.Semantic(root); got != want {
				t.Errorf("cache published different semantic roots: %p != %p", got, want)
			}
		}()
	}
	wg.Wait()
}

func comprehensivePresentation(label string, reverse bool) typ.Type {
	leaf := typ.Func().Param(label, typ.String).Returns(typ.Any).Build()
	nested := typ.Func().
		Param("callback", typ.NewArray(leaf)).
		Variadic(typ.NewReadonlyMap(typ.String, leaf)).
		Returns(typ.NewTuple(typ.NewMap(typ.String, leaf), typ.NewMeta(leaf))).
		Build()

	unionMembers := []typ.Type{typ.Nil, typ.MaterializeOptional(leaf), nested}
	intersectionMembers := []typ.Type{typ.NewInterface("", []typ.Method{{Name: "call", Type: leaf}}), typ.Any}
	if reverse {
		for i, j := 0, len(unionMembers)-1; i < j; i, j = i+1, j-1 {
			unionMembers[i], unionMembers[j] = unionMembers[j], unionMembers[i]
		}
		for i, j := 0, len(intersectionMembers)-1; i < j; i, j = i+1, j-1 {
			intersectionMembers[i], intersectionMembers[j] = intersectionMembers[j], intersectionMembers[i]
		}
	}
	fields := []typ.Field{
		{Name: "array", Type: typ.NewArray(leaf)},
		{Name: "intersection", Type: typ.MaterializeIntersection(intersectionMembers)},
		{Name: "map", Type: typ.NewMap(leaf, typ.NewReadonlyMap(typ.String, nested))},
		{Name: "tuple", Type: typ.NewTuple(leaf, typ.MaterializeUnion(unionMembers))},
	}
	if reverse {
		for i, j := 0, len(fields)-1; i < j; i, j = i+1, j-1 {
			fields[i], fields[j] = fields[j], fields[i]
		}
	}

	tp := typ.NewTypeParam("T", leaf)
	generic := typ.NewGeneric("Container", []*typ.TypeParam{tp}, typ.NewArray(tp))
	annotated := typ.NewAnnotated(nested, []annotation.Annotation{{Name: "checked", Arg: annotation.BoolArg(true)}})
	return typ.RebuildRecord(typ.RecordParts{
		Fields: fields,
		StaticMembers: []typ.StaticMember{{
			Kind: typ.StaticMemberStringIndex, Name: "factory", Type: annotated,
		}},
		Metatable: typ.NewAlias("MetaAlias", typ.NewMeta(leaf)),
		MapKey:    typ.NewRef("actor", "Key"),
		MapValue:  typ.Instantiate(generic, leaf),
		Open:      true,
	})
}

func containsAny(s string, values ...string) bool {
	for _, value := range values {
		if len(value) <= len(s) {
			for i := 0; i+len(value) <= len(s); i++ {
				if s[i:i+len(value)] == value {
					return true
				}
			}
		}
	}
	return false
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

func largeRecursiveManifestPresentation(fields int) typ.Type {
	recursive := typ.NewRecursivePlaceholder("ManifestNode")
	tableFields := make([]typ.Field, fields+1)
	tableFields[0] = typ.Field{Name: "next", Type: typ.MaterializeOptional(recursive)}
	for i := 0; i < fields; i++ {
		callback := typ.Func().
			Param(fmt.Sprintf("message_%d", i), typ.NewArray(recursive)).
			Returns(typ.NewReadonlyMap(typ.String, typ.NewTuple(recursive, typ.Any))).
			Build()
		tableFields[i+1] = typ.Field{
			Name: fmt.Sprintf("handler_%04d", i),
			Type: typ.MaterializeUnion([]typ.Type{
				callback,
				typ.NewMap(typ.String, callback),
				typ.Nil,
			}),
		}
	}
	recursive.SetBody(typ.RebuildRecord(typ.RecordParts{Fields: tableFields}))
	return recursive
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

func BenchmarkCachedRecursiveManifestFirstSemanticSelection(b *testing.B) {
	presentation := largeRecursiveManifestPresentation(256)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		cache := NewSemanticGraphCache()
		cache.Prewarm(presentation)
	}
}

func BenchmarkCachedRecursiveManifestSteadySelection(b *testing.B) {
	presentation := largeRecursiveManifestPresentation(256)
	cache := NewSemanticGraphCache()
	cache.Prewarm(presentation)
	want := cache.Semantic(presentation)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := cache.Semantic(presentation); got != want {
			b.Fatal("unstable cached recursive semantic graph")
		}
	}
}

func BenchmarkPairedRepresentationSize(b *testing.B) {
	b.ReportMetric(float64(unsafe.Sizeof(TypePair{})), "bytes/eager-root")
	b.ReportMetric(float64(unsafe.Sizeof(LazySemanticGraph{})), "bytes/lazy-root")
	for i := 0; i < b.N; i++ {
	}
}
