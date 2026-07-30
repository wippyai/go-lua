package variantorigin

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/variant/caseset"
)

func TestJoinUnionsCasesWithinFamily(t *testing.T) {
	got := Join(Singleton(7, 2), Of(7, []int{1, 2}))
	want := Of(7, []int{1, 2})
	if !Equal(got, want) {
		t.Fatalf("Join same family = %#v, want %#v", got, want)
	}
}

func TestJoinDifferentFamiliesIsTop(t *testing.T) {
	got := Join(Singleton(7, 0), Singleton(8, 0))
	if !got.IsTop() {
		t.Fatalf("Join different families = %#v, want Top", got)
	}
}

func TestNarrowCase(t *testing.T) {
	v := Of(7, []int{0, 1, 2})
	got := v.NarrowCase(7, 1, true)
	if !Equal(got, Singleton(7, 1)) {
		t.Fatalf("Narrow equal = %#v, want singleton case", got)
	}

	got = v.NarrowCase(7, 1, false)
	if !Equal(got, Of(7, []int{0, 2})) {
		t.Fatalf("Narrow not-equal = %#v, want excluded case", got)
	}

	got = Singleton(7, 1).NarrowCase(7, 1, false)
	if !got.IsBottom() {
		t.Fatalf("exclude only case = %#v, want Bottom", got)
	}
}

func TestNarrowCaseDifferentFamily(t *testing.T) {
	v := Of(8, []int{0, 1})

	got := v.NarrowCase(7, 1, true)
	if !got.IsBottom() {
		t.Fatalf("equal against different family = %#v, want Bottom", got)
	}

	got = v.NarrowCase(7, 1, false)
	if !Equal(got, v) {
		t.Fatalf("not-equal against different family = %#v, want unchanged %#v", got, v)
	}
}

func TestHashFollowsCanonicalCaseOrder(t *testing.T) {
	a := Of(7, []int{2, 1, 1})
	b := Of(7, []int{1, 2})
	if !Equal(a, b) {
		t.Fatalf("canonical values not equal: %#v vs %#v", a, b)
	}
	if a.Hash() != b.Hash() {
		t.Fatalf("equal values hash differently: %d vs %d", a.Hash(), b.Hash())
	}
}

func TestValueOwnsSourceAndReturnedCases(t *testing.T) {
	source := []int{3, 1, 3, 2}
	v := Of(7, source)
	source[0] = 99

	got := v.Cases()
	if !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("source mutation changed origin cases: got %v", got)
	}
	got[0] = 88
	if again := v.Cases(); !reflect.DeepEqual(again, []int{1, 2, 3}) {
		t.Fatalf("returned Cases mutation changed origin cases: got %v", again)
	}
}

func TestImmutableCasesPreserveInternedProductIdentity(t *testing.T) {
	reg := axis.NewRegistry()
	axis.Register(reg, Spec())
	reg.Freeze()

	source := []int{4, 2, 4, 1}
	origin := Of(19, source)
	stored := product.Set(reg, product.Top(), Key, origin)
	wantHash := product.Hash(reg, stored)

	source[0] = 999
	returned := origin.Cases()
	returned[0] = 888
	if got := product.Hash(reg, stored); got != wantHash {
		t.Fatalf("caller mutation changed interned product hash: got %d want %d", got, wantHash)
	}
	if got := product.Get(reg, stored, Key).Cases(); !reflect.DeepEqual(got, []int{1, 2, 4}) {
		t.Fatalf("caller mutation changed interned product payload: got %v", got)
	}
	reinterned := product.Set(reg, product.Top(), Key, Of(19, []int{1, 2, 4}))
	if reinterned != stored {
		t.Fatal("equal immutable origin did not reuse interned product node")
	}
	if !product.RetentionSafe(reg, stored) {
		t.Fatal("immutable variant origin was rejected as retention-unsafe")
	}
}

func TestCasesViewConcurrentReads(t *testing.T) {
	v := Of(23, []int{8, 3, 5, 1, 3})
	const readers = 16
	var group sync.WaitGroup
	group.Add(readers)
	for reader := 0; reader < readers; reader++ {
		go func() {
			defer group.Done()
			for pass := 0; pass < 1000; pass++ {
				view := v.CasesView()
				for i := 0; i < view.Len(); i++ {
					if i > 0 && view.At(i-1) >= view.At(i) {
						t.Errorf("view is not strictly ordered at %d", i)
						return
					}
				}
			}
		}()
	}
	group.Wait()
}

func TestCasesViewHasNoSliceEscapeSurface(t *testing.T) {
	viewType := reflect.TypeOf(caseset.View{})
	for i := 0; i < viewType.NumMethod(); i++ {
		method := viewType.Method(i)
		if method.Type.NumOut() == 1 && method.Type.Out(0).Kind() == reflect.Slice {
			t.Fatalf("caseset.View method %s exposes slice backing", method.Name)
		}
	}
}

var benchmarkCaseSum int

func BenchmarkCasesView(b *testing.B) {
	for _, count := range []int{1, 4, 16} {
		values := make([]int, count)
		for i := range values {
			values[i] = count - i
		}
		origin := Of(31, values)
		b.Run(fmt.Sprintf("cases=%d/view", count), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			sum := 0
			for i := 0; i < b.N; i++ {
				view := origin.CasesView()
				for index := 0; index < view.Len(); index++ {
					sum += view.At(index)
				}
			}
			benchmarkCaseSum = sum
		})
		b.Run(fmt.Sprintf("cases=%d/indexed", count), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			sum := 0
			for i := 0; i < b.N; i++ {
				for index := 0; index < origin.CasesLen(); index++ {
					sum += origin.CaseAt(index)
				}
			}
			benchmarkCaseSum = sum
		})
	}
}
