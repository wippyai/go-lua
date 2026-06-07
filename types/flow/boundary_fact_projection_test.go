package flow

import (
	"slices"
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/typ"
)

func TestProjectBoundaryFactsProjectsPointFactLanesTogether(t *testing.T) {
	sym := cfg.SymbolID(31)
	root := constraint.NewPath(sym, "graph")
	tablePath := root.Field("nodes")
	keyPath := root.Field("last_node_id")
	table := boundaryProjectionTestAddress(t, tablePath)
	key := boundaryProjectionTestAddress(t, keyPath)
	value := product.FromType(typ.Number)

	num := numericStateWithLenLower(t, tablePath, 2)
	facts := ProjectBoundaryFacts(
		BoundaryFactProjectionInput{
			KeyPresence: KeyPresenceFacts{}.WithAddresses(table, key),
			Num:         num,
			IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
				Target:     table,
				KeyPath:    key,
				HasKeyPath: true,
				Key:        product.FromType(typ.String),
				Value:      value,
			}),
		},
		NewBoundaryPathProjection(map[cfg.SymbolID]int{sym: 0}, nil),
		BoundaryFactProjectionPolicy{},
	)

	wantTable := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: tablePath.Segments}
	wantKey := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: keyPath.Segments}
	if got := facts.KeyPresence(); len(got) != 1 ||
		!boundaryProjectionPathEqual(got[0].Table, wantTable) ||
		!boundaryProjectionPathEqual(got[0].Key, wantKey) {
		t.Fatalf("key-presence facts = %#v, want table %#v key %#v", got, wantTable, wantKey)
	}
	if got := facts.LengthLowerBounds(); len(got) != 1 ||
		!boundaryProjectionPathEqual(got[0].Target, wantTable) ||
		got[0].Lower != 2 {
		t.Fatalf("length facts = %#v, want len(%#v) >= 2", got, wantTable)
	}
	if got := facts.IndexWrites(); len(got) != 1 ||
		!boundaryProjectionPathEqual(got[0].Table, wantTable) ||
		!boundaryProjectionPathEqual(got[0].Key, wantKey) ||
		!product.Domain.Equal(got[0].Value, value) {
		t.Fatalf("index-write facts = %#v, want table %#v key %#v value %s", got, wantTable, wantKey, value.ProjectValue())
	}
}

func TestProjectBoundaryFactsCachesAddressProjectionAcrossLanes(t *testing.T) {
	sym := cfg.SymbolID(32)
	root := constraint.NewPath(sym, "graph")
	tablePath := root.Field("nodes")
	keyPath := root.Field("last_node_id")
	table := boundaryProjectionTestAddress(t, tablePath)
	key := boundaryProjectionTestAddress(t, keyPath)

	num := numericStateWithLenLower(t, tablePath, 2)
	projector := &countingBoundaryProjector{
		inner: NewBoundaryPathProjection(map[cfg.SymbolID]int{sym: 0}, nil),
		calls: make(map[constraint.PathKey]int),
	}

	_ = ProjectBoundaryFacts(
		BoundaryFactProjectionInput{
			KeyPresence: KeyPresenceFacts{}.
				WithAddresses(table, key).
				WithKeyArrayAddresses(key, table),
			Num: num,
			IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
				Target:     table,
				KeyPath:    key,
				HasKeyPath: true,
				Key:        product.FromType(typ.String),
				Value:      product.FromType(typ.Number),
			}),
		},
		projector,
		BoundaryFactProjectionPolicy{},
	)

	if got := projector.calls[table.Key()]; got != 1 {
		t.Fatalf("table address projected %d times, want once", got)
	}
	if got := projector.calls[key.Key()]; got != 1 {
		t.Fatalf("key address projected %d times, want once", got)
	}
}

func TestProjectBoundaryFactsIgnoresNonContainerNumericLengthKeys(t *testing.T) {
	sym := cfg.SymbolID(33)
	root := constraint.NewPath(sym, "graph")
	tablePath := root.Field("nodes")

	num := numericStateWithLenLower(t, tablePath, 2)
	num.ApplyLenGeConst(constraint.PathKey("legacy.nodes"), 9)
	facts := ProjectBoundaryFacts(
		BoundaryFactProjectionInput{Num: num},
		NewBoundaryPathProjection(map[cfg.SymbolID]int{sym: 0}, nil),
		BoundaryFactProjectionPolicy{},
	)

	wantTable := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: tablePath.Segments}
	got := facts.LengthLowerBounds()
	if len(got) != 1 || !boundaryProjectionPathEqual(got[0].Target, wantTable) || got[0].Lower != 2 {
		t.Fatalf("length facts = %#v, want only normalized container %#v >= 2", got, wantTable)
	}
}

type countingBoundaryProjector struct {
	inner BoundaryPathProjection
	calls map[constraint.PathKey]int
}

func (p *countingBoundaryProjector) PathsFromAddress(addr StableAddress) []BoundaryPath {
	p.calls[addr.Key()]++
	return p.inner.PathsFromAddress(addr)
}

func boundaryProjectionTestAddress(t *testing.T, path constraint.Path) StableAddress {
	t.Helper()
	addr, ok := StableAddressOfPath(path)
	if !ok {
		t.Fatalf("stable address for path %s", path.Key())
	}
	return addr
}

func numericStateWithLenLower(t *testing.T, path constraint.Path, lower int64) *numeric.State {
	t.Helper()
	container, ok := ContainerRefOfPath(path)
	if !ok {
		t.Fatalf("ContainerRefOfPath(%v) failed", path)
	}
	op, ok := NumericLenGeConstContainerOp(container, lower)
	if !ok {
		t.Fatalf("NumericLenGeConstContainerOp(%#v, %d) failed", container, lower)
	}
	state := PointState{}
	if !ApplyNumericEffect(&state, NumericEffect{Ops: []NumericOp{op}}) {
		t.Fatalf("ApplyNumericEffect did not apply len lower %d for %v", lower, path)
	}
	return state.Num
}

func boundaryProjectionPathEqual(a, b BoundaryPath) bool {
	return a.Kind == b.Kind &&
		a.Index == b.Index &&
		slices.Equal(a.Segments, b.Segments)
}
