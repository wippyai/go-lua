package variant

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestImmutableOriginViewMatchesSliceAPIs(t *testing.T) {
	childA := typetable.NewRecord().Field("tag", typ.LiteralString("a")).Build()
	childB := typetable.NewRecord().Field("tag", typ.LiteralString("b")).Build()
	childUnion := typeexpr.Union(childA, childB)
	parentA := typetable.NewRecord().
		Field("kind", typ.LiteralString("left")).
		Field("payload", childUnion).
		Build()
	parentB := typetable.NewRecord().
		Field("kind", typ.LiteralString("right")).
		Field("payload", childUnion).
		Build()
	parentUnion := typeexpr.Union(parentA, parentB)
	parentFamily, parentCases, ok := OriginOfType(parentUnion)
	if !ok || len(parentCases) != 2 {
		t.Fatalf("parent origin = %d/%v/%v, want two cases", parentFamily, parentCases, ok)
	}
	childFamily, childCases, ok := OriginOfType(childUnion)
	if !ok || len(childCases) != 2 {
		t.Fatalf("child origin = %d/%v/%v, want two cases", childFamily, childCases, ok)
	}
	payload := []segment.Segment{{Kind: segment.SegmentField, Name: "payload"}}

	inputs := [][]int{
		{parentCases[1], parentCases[0], parentCases[1]},
		{parentCases[0]},
		{parentCases[0], 999999},
	}
	for _, input := range inputs {
		input := input
		t.Run(fmt.Sprint(input), func(t *testing.T) {
			view := variantorigin.Of(parentFamily, input).CasesView()

			sliceType, sliceOK := TypeFromOrigin(parentFamily, input)
			viewType, viewOK := TypeFromOriginView(parentFamily, view)
			assertTypeResultEqual(t, "TypeFromOrigin", sliceType, sliceOK, viewType, viewOK)

			sliceNarrow, sliceChanged := NarrowByOrigin(parentUnion, parentFamily, input)
			viewNarrow, viewChanged := NarrowByOriginView(parentUnion, parentFamily, view)
			assertTypeResultEqual(t, "NarrowByOrigin", sliceNarrow, sliceChanged, viewNarrow, viewChanged)

			sliceFamily, sliceCases, sliceProjected := ProjectOrigin(parentFamily, input, payload)
			viewFamily, viewCases, viewProjected := ProjectOriginView(parentFamily, view, payload)
			if sliceFamily != viewFamily || sliceProjected != viewProjected || !reflect.DeepEqual(sliceCases, viewCases) {
				t.Fatalf("ProjectOrigin parity: slice=%d/%v/%v view=%d/%v/%v", sliceFamily, sliceCases, sliceProjected, viewFamily, viewCases, viewProjected)
			}

			sliceByType, sliceByTypeOK := NarrowOriginByPathType(parentFamily, input, payload, childA, true)
			viewByType, viewByTypeOK := NarrowOriginByPathTypeView(parentFamily, view, payload, childA, true)
			if sliceByTypeOK != viewByTypeOK || !reflect.DeepEqual(sliceByType, viewByType) {
				t.Fatalf("NarrowOriginByPathType parity: slice=%v/%v view=%v/%v", sliceByType, sliceByTypeOK, viewByType, viewByTypeOK)
			}

			childInput := []int{childCases[1], childCases[0], childCases[1]}
			childView := variantorigin.Of(childFamily, childInput).CasesView()
			sliceByOrigin, sliceByOriginOK := NarrowOriginByPath(parentFamily, input, payload, childFamily, childInput, true)
			viewByOrigin, viewByOriginOK := NarrowOriginByPathView(parentFamily, view, payload, childFamily, childView, true)
			if sliceByOriginOK != viewByOriginOK || !reflect.DeepEqual(sliceByOrigin, viewByOrigin) {
				t.Fatalf("NarrowOriginByPath parity: slice=%v/%v view=%v/%v", sliceByOrigin, sliceByOriginOK, viewByOrigin, viewByOriginOK)
			}

			cache := NewCache()
			sliceCached, sliceCachedOK := cache.TypeFromOrigin(parentFamily, input)
			viewCached, viewCachedOK := cache.TypeFromOriginView(parentFamily, view)
			assertTypeResultEqual(t, "cached TypeFromOrigin", sliceCached, sliceCachedOK, viewCached, viewCachedOK)
		})
	}
}

func assertTypeResultEqual(t *testing.T, operation string, left typ.Type, leftOK bool, right typ.Type, rightOK bool) {
	t.Helper()
	if leftOK != rightOK || (left != nil && right != nil && !typ.TypeEquals(left, right)) || (left == nil) != (right == nil) {
		t.Fatalf("%s parity: slice=%v/%v view=%v/%v", operation, left, leftOK, right, rightOK)
	}
}

var benchmarkOriginType typ.Type

func BenchmarkImmutableOriginView(b *testing.B) {
	for _, count := range []int{1, 4, 16} {
		members := make([]typ.Type, count)
		for i := range members {
			members[i] = typetable.NewRecord().
				Field("kind", typ.LiteralString(fmt.Sprintf("case-%02d", i))).
				Build()
		}
		union := typeexpr.Union(members...)
		family, cases, ok := OriginOfType(union)
		if !ok {
			b.Fatalf("origin missing for %d cases", count)
		}
		view := variantorigin.Of(family, cases).CasesView()
		b.Run(fmt.Sprintf("cases=%d/slice", count), func(b *testing.B) {
			cache := NewCache()
			_, _ = cache.TypeFromOrigin(family, cases)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkOriginType, _ = cache.TypeFromOrigin(family, cases)
			}
		})
		b.Run(fmt.Sprintf("cases=%d/view", count), func(b *testing.B) {
			cache := NewCache()
			_, _ = cache.TypeFromOriginView(family, view)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkOriginType, _ = cache.TypeFromOriginView(family, view)
			}
		})
	}
}
