package keyspace

import (
	"reflect"
	"sort"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

var lessSink bool

func TestExportedKeyMintingSurfaceIsCensused(t *testing.T) {
	want := map[string]bool{
		"AppendPathSegment": true, "AppendSegment": true, "FieldCanonical": true, "FromPath": true,
		"InternFormalRoot": true,
		"KeyByHandle":      true,
		"FromPathKey":      true, "FromResolverKey": true, "FromRootlessSuffix": true,
		"FromStableSymbol": true, "FromStateKey": true, "ImportExistential": true,
		"ImportKey": true, "InternStateKey": true, "LookupResolverKey": true,
		"Rebase": true, "RebaseToExistential": true, "StructuralRoot": true,
		"WithFormalRoot": true, "WithStructuralRoot": true,
	}
	keyType := reflect.TypeOf(Key{})
	typ := reflect.TypeOf((*KeySpace)(nil))
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		returnsKey := false
		for out := 0; out < method.Type.NumOut(); out++ {
			returnsKey = returnsKey || method.Type.Out(out) == keyType
		}
		if returnsKey {
			if !want[method.Name] {
				t.Fatalf("exported Key producer %s lacks a sealed-mint census fixture", method.Name)
			}
			delete(want, method.Name)
		}
	}
	if len(want) != 0 {
		t.Fatalf("stale exported Key producer census: %v", want)
	}
}

func TestWithStructuralRootUsesSealedCanonicalMint(t *testing.T) {
	sourceSpace, targetSpace := New(), New()
	source := sourceSpace.FromPath(pathdom.NewPath(symbol.ID(77), "source").Field("member"))
	target := targetSpace.FromPath(pathdom.NewPath(symbol.ID(88), "target"))
	got, ok := targetSpace.WithStructuralRoot(sourceSpace, source, target)
	if !ok || targetSpace.FormatReadOnly(got) != "sym88.member" {
		t.Fatalf("WithStructuralRoot = %#v/%t (%q), want sealed sym88.member", got, ok, targetSpace.FormatReadOnly(got))
	}
	if root, exact := targetSpace.StructuralRoot(got); !exact || root != target {
		t.Fatalf("WithStructuralRoot root = %#v/%t, want %#v", root, exact, target)
	}
}

func TestStructuralRootUsesSealedCanonicalMint(t *testing.T) {
	ks := New()
	child := ks.FromPath(pathdom.NewPath(symbol.ID(77), "value").Field("member"))
	root, ok := ks.StructuralRoot(child)
	if !ok || root.Segs != 0 || ks.FormatReadOnly(root) != "sym77" {
		t.Fatalf("StructuralRoot = %#v/%t (%q), want sealed sym77", root, ok, ks.FormatReadOnly(root))
	}
	if !ks.Less(root, child) {
		t.Fatal("sealed structural root does not participate in canonical order")
	}
	rootless, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "member"}})
	if !ok {
		t.Fatal("rootless fixture")
	}
	if got, ok := ks.StructuralRoot(rootless); ok || got != (Key{}) {
		t.Fatalf("rootless StructuralRoot = %#v/%t, want zero/false", got, ok)
	}
}

func TestLessDeepRootsMatchesIndependentSpellingOrder(t *testing.T) {
	ks := New()
	deep := make([]segment.Segment, 256)
	for i := range deep {
		deep[i] = segment.Segment{Kind: segment.SegmentField, Name: "shared"}
	}
	type entry struct {
		key  Key
		want pathdom.PathKey
	}
	entries := make([]entry, 0, 64)
	for i := 1; i <= 64; i++ {
		segs := append([]segment.Segment(nil), deep...)
		segs[len(segs)-1] = segment.Segment{Kind: segment.SegmentIndexInt, Index: 64 - i}
		p := pathdom.Path{Symbol: symbol.ID(i), Version: i, Segments: segs}
		entries = append(entries, entry{key: ks.FromPath(p), want: p.Key()})
	}
	sort.Slice(entries, func(i, j int) bool { return ks.Less(entries[i].key, entries[j].key) })
	want := make([]string, len(entries))
	for i := range entries {
		want[i] = string(entries[i].want)
	}
	sort.Strings(want)
	for i := range entries {
		if got := string(ks.Format(entries[i].key)); got != want[i] {
			t.Fatalf("deep order[%d] = %q, want %q", i, got, want[i])
		}
	}
}

func TestLessTwelveThousandRootsAllocatesZeroAndDoesNotMutate(t *testing.T) {
	ks := New()
	keys := make([]Key, 12110)
	for i := range keys {
		keys[i], _ = ks.FromResolverKey(symbol.ID(i+1), 1, nil)
	}
	sealed := len(ks.formatByKey)
	allocs := testing.AllocsPerRun(5, func() {
		for i := 1; i < len(keys); i++ {
			lessSink = ks.Less(keys[i], keys[i-1])
		}
	})
	if allocs != 0 {
		t.Fatalf("Less over 12,110 roots allocated %.2f times, want zero", allocs)
	}
	if got := len(ks.formatByKey); got != sealed {
		t.Fatalf("Less mutated spelling table: %d -> %d", sealed, got)
	}
}

func BenchmarkLessTwelveThousandSealedRoots(b *testing.B) {
	ks := New()
	keys := make([]Key, 12110)
	for i := range keys {
		keys[i], _ = ks.FromResolverKey(symbol.ID(i+1), 1, nil)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := 1; i < len(keys); i++ {
			lessSink = ks.Less(keys[i], keys[i-1])
		}
	}
}
