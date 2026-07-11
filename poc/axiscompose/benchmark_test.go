package axiscompose

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
)

var (
	benchBool  bool
	benchState State
)

type comparisonFixture struct {
	arena   *Arena
	base    State
	changed State
}

func newComparisonFixture(n int) comparisonFixture {
	catalog := &Catalog{}
	ids := make([]AxisID, n)
	handles := make([]Handle[uint8], n)
	for i := 0; i < n; i++ {
		id := AxisID(fmt.Sprintf("may.%03d", i))
		ids[i] = id
		handles[i] = MustRegister(catalog, Spec[uint8]{
			ID:       id,
			Polarity: May,
			Domain: lattice.Lattice[uint8]{
				Bottom:   func() uint8 { return 0 },
				Top:      func() uint8 { return 0xff },
				Equal:    func(a, b uint8) bool { return a == b },
				Same:     func(a, b uint8) bool { return a == b },
				LessOrEq: func(a, b uint8) bool { return a&^b == 0 },
				Join:     func(a, b uint8) uint8 { return a | b },
				Widen:    func(a, b uint8) uint8 { return a | b },
			},
			Hash: func(v uint8) uint64 { return uint64(v) },
		})
	}
	schema, err := catalog.Seal(ids...)
	if err != nil {
		panic(err)
	}
	arena := &Arena{}
	base := Bottom(arena, schema)
	changed := Put(arena, base, handles[n-1], uint8(1))
	return comparisonFixture{arena: arena, base: base, changed: changed}
}

func BenchmarkLessOrEqOneChanged(b *testing.B) {
	for _, lanes := range []int{3, 17, 32, 64} {
		fixture := newComparisonFixture(lanes)
		b.Run(fmt.Sprintf("masked/%d", lanes), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				benchBool = LessOrEq(fixture.base, fixture.changed)
			}
		})
		b.Run(fmt.Sprintf("baseline/%d", lanes), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				benchBool = LessOrEqBaseline(fixture.base, fixture.changed)
			}
		})
	}
}

func BenchmarkLessOrEqAllChanged17(b *testing.B) {
	fixture := newComparisonFixture(17)
	allChanged := fixture.base
	for i := range allChanged.slots {
		allChanged.slots[i] = slot{value: uint8(1), stamp: fixture.arena.fresh()}
	}
	b.Run("masked", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchBool = LessOrEq(fixture.base, allChanged)
		}
	})
	b.Run("baseline", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchBool = LessOrEqBaseline(fixture.base, allChanged)
		}
	})
}

func BenchmarkJoinOneChanged17(b *testing.B) {
	fixture := newComparisonFixture(17)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		benchState = Join(fixture.arena, fixture.base, fixture.changed)
	}
}

func BenchmarkBoundary(b *testing.B) {
	s := newToySetup()
	exactSchema, _ := s.catalog.Seal(s.may.ID(), s.must.ID())
	fallbackSchema, _ := s.catalog.Seal(s.may.ID(), s.unsupported.ID())
	arena := &Arena{}
	exactState := Put(arena, Bottom(arena, exactSchema), s.may, uint8(3))
	exactProjection := ProjectBoundary(exactState, ProjectCtx{Used: AllUsed(exactSchema), Binding: Binding{Symbol: "p"}})
	fallbackState := Put(arena, Bottom(arena, fallbackSchema), s.unsupported, uint8(1))
	fallbackProjection := ProjectBoundary(fallbackState, ProjectCtx{Used: AllUsed(fallbackSchema), Binding: Binding{Symbol: "p"}})
	b.Run("exact", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchState, benchBool = InstantiateBoundary(arena, exactProjection, InstantiateCtx{Binding: Binding{Symbol: "a"}}, nil)
		}
	})
	b.Run("contextual-fallback", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchState, benchBool = InstantiateBoundary(arena, fallbackProjection, InstantiateCtx{Binding: Binding{Symbol: "a"}}, func() State { return fallbackState })
		}
	})
}
