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

	num := numericStateWithLenBounds(t, tablePath, 2, 5)
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
	if got := facts.LengthUpperBounds(); len(got) != 1 ||
		!boundaryProjectionPathEqual(got[0].Target, wantTable) ||
		got[0].Upper != 5 {
		t.Fatalf("length upper facts = %#v, want len(%#v) <= 5", got, wantTable)
	}
	if got := facts.IndexWrites(); len(got) != 1 ||
		!boundaryProjectionPathEqual(got[0].Table, wantTable) ||
		!got[0].HasKeyPath ||
		!boundaryProjectionPathEqual(got[0].KeyPath, wantKey) ||
		!product.Domain.Equal(got[0].KeyValue, product.FromType(typ.String)) ||
		!product.Domain.Equal(got[0].Value, value) {
		t.Fatalf("index-write facts = %#v, want table %#v key %#v value %s", got, wantTable, wantKey, value.ProjectValue())
	}
}

func TestProjectBoundaryFactsProjectsIndexWriteKeyValueWithoutBoundaryKeyPath(t *testing.T) {
	paramSym := cfg.SymbolID(34)
	localKeySym := cfg.SymbolID(35)
	root := constraint.NewPath(paramSym, "graph")
	tablePath := root.Field("nodes")
	keyPath := constraint.NewPath(localKeySym, "node_id")
	table := boundaryProjectionTestAddress(t, tablePath)
	key := boundaryProjectionTestAddress(t, keyPath)
	keyValue := product.FromType(typ.LiteralString("n1"))
	value := product.FromType(typ.NewRecord().Field("id", typ.String).Build())

	facts := ProjectBoundaryFacts(
		BoundaryFactProjectionInput{
			IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
				Target:     table,
				KeyPath:    key,
				HasKeyPath: true,
				Key:        keyValue,
				Value:      value,
			}),
		},
		NewBoundaryPathProjection(map[cfg.SymbolID]int{paramSym: 0}, nil),
		BoundaryFactProjectionPolicy{},
	)

	wantTable := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: tablePath.Segments}
	writes := facts.IndexWrites()
	if len(writes) != 1 ||
		!boundaryProjectionPathEqual(writes[0].Table, wantTable) ||
		writes[0].HasKeyPath ||
		!product.Domain.Equal(writes[0].KeyValue, keyValue) ||
		!product.Domain.Equal(writes[0].Value, value) {
		t.Fatalf("index writes = %#v, want table %#v value-key %s", writes, wantTable, keyValue.ProjectValue())
	}
}

func TestProjectBoundaryFactsProjectsUnnamedAppendHistoryTableCoverage(t *testing.T) {
	paramSym := cfg.SymbolID(38)
	localKeySym := cfg.SymbolID(39)
	root := constraint.NewPath(paramSym, "graph")
	arrayPath := root.Field("node_order")
	tablePath := root.Field("edges")
	keyPath := constraint.NewPath(localKeySym, "node_id")
	array := boundaryProjectionTestAddress(t, arrayPath)
	table := boundaryProjectionTestAddress(t, tablePath)
	key := boundaryProjectionTestAddress(t, keyPath)
	value := product.FromType(typ.NewRecord().Field("targets", typ.NewArray(typ.String)).Build())

	facts := ProjectBoundaryFacts(
		BoundaryFactProjectionInput{
			KeyPresence: KeyPresenceFacts{}.
				WithAppendHistoryBaseAddress(array).
				WithAppendHistoryCoverageAddresses(array, key, table, value),
		},
		NewBoundaryPathProjection(map[cfg.SymbolID]int{paramSym: 0}, nil),
		BoundaryFactProjectionPolicy{},
	)

	wantArray := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: arrayPath.Segments}
	wantTable := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: tablePath.Segments}
	coverage := facts.AppendHistoryTableCoverage()
	if len(coverage) != 1 ||
		!boundaryProjectionPathEqual(coverage[0].Array, wantArray) ||
		!boundaryProjectionPathEqual(coverage[0].Table, wantTable) ||
		!product.Domain.Equal(coverage[0].Value, value) {
		t.Fatalf("append table coverage = %#v, want array %#v table %#v", coverage, wantArray, wantTable)
	}
}

func TestProjectBoundaryFactsRebasesIndexWriteKeyPathThroughAlias(t *testing.T) {
	paramSym := cfg.SymbolID(36)
	localKeySym := cfg.SymbolID(37)
	root := constraint.NewPath(paramSym, "graph")
	tablePath := root.Field("edges")
	receiverKeyPath := root.Field("last_node_id")
	localKeyPath := constraint.NewPath(localKeySym, "node_id")
	table := boundaryProjectionTestAddress(t, tablePath)
	receiverKey := boundaryProjectionTestAddress(t, receiverKeyPath)
	localKey := boundaryProjectionTestAddress(t, localKeyPath)
	value := product.FromType(typ.NewRecord().Field("targets", typ.NewArray(typ.String)).Build())

	facts := ProjectBoundaryFacts(
		BoundaryFactProjectionInput{
			IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
				Target:     table,
				KeyPath:    localKey,
				HasKeyPath: true,
				Key:        product.FromType(typ.Any),
				Value:      value,
			}),
			IdentityAliases: PathAliasFacts{}.WithAddresses(receiverKey, localKey),
		},
		NewBoundaryPathProjection(map[cfg.SymbolID]int{paramSym: 0}, nil),
		BoundaryFactProjectionPolicy{},
	)

	wantTable := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: tablePath.Segments}
	wantKey := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: receiverKeyPath.Segments}
	writes := facts.IndexWrites()
	if len(writes) != 1 ||
		!boundaryProjectionPathEqual(writes[0].Table, wantTable) ||
		!writes[0].HasKeyPath ||
		!boundaryProjectionPathEqual(writes[0].KeyPath, wantKey) ||
		!product.Domain.Equal(writes[0].Value, value) {
		t.Fatalf("index writes = %#v, want table %#v key %#v", writes, wantTable, wantKey)
	}
}

func TestProjectBoundaryFactsProjectsReturnedKeyOfParamTable(t *testing.T) {
	paramSym := cfg.SymbolID(11)
	keySym := cfg.SymbolID(12)
	tablePath := constraint.NewPath(paramSym, "self").Field("nodes")
	keyPath := constraint.NewPath(keySym, "id")

	facts := ProjectBoundaryFacts(
		BoundaryFactProjectionInput{
			KeyPresence: KeyPresenceFacts{}.WithAddresses(
				boundaryProjectionTestAddress(t, tablePath),
				boundaryProjectionTestAddress(t, keyPath),
			),
		},
		NewBoundaryPathProjection(
			map[cfg.SymbolID]int{paramSym: 2},
			map[cfg.SymbolID][]BoundaryPath{
				keySym: {{Kind: BoundaryPathReturn, Index: 0}},
			},
		),
		BoundaryFactProjectionPolicy{},
	)

	wantTable := BoundaryPath{
		Kind:     BoundaryPathParam,
		Index:    2,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}},
	}
	wantKey := BoundaryPath{Kind: BoundaryPathReturn, Index: 0}
	got := facts.KeyPresence()
	if len(got) != 1 ||
		!boundaryProjectionPathEqual(got[0].Table, wantTable) ||
		!boundaryProjectionPathEqual(got[0].Key, wantKey) {
		t.Fatalf("key-presence facts = %#v, want table %#v key %#v", got, wantTable, wantKey)
	}
}

func TestProjectBoundaryFactsIgnoresNonBoundaryTableForReturnedKey(t *testing.T) {
	paramSym := cfg.SymbolID(21)
	otherSym := cfg.SymbolID(22)
	keySym := cfg.SymbolID(23)
	tablePath := constraint.NewPath(otherSym, "localTable")
	keyPath := constraint.NewPath(keySym, "id")

	facts := ProjectBoundaryFacts(
		BoundaryFactProjectionInput{
			KeyPresence: KeyPresenceFacts{}.WithAddresses(
				boundaryProjectionTestAddress(t, tablePath),
				boundaryProjectionTestAddress(t, keyPath),
			),
		},
		NewBoundaryPathProjection(
			map[cfg.SymbolID]int{paramSym: 0},
			map[cfg.SymbolID][]BoundaryPath{
				keySym: {{Kind: BoundaryPathReturn, Index: 0}},
			},
		),
		BoundaryFactProjectionPolicy{},
	)

	if facts.HasProof() {
		t.Fatalf("projected non-boundary table into boundary facts: %#v", facts)
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
	return numericStateWithLenBounds(t, path, lower, -1)
}

func numericStateWithLenBounds(t *testing.T, path constraint.Path, lower, upper int64) *numeric.State {
	t.Helper()
	container, ok := ContainerRefOfPath(path)
	if !ok {
		t.Fatalf("ContainerRefOfPath(%v) failed", path)
	}
	state := PointState{}
	ops := NumericLengthBoundContainerOps(container, ">=", lower)
	if upper >= 0 {
		ops = append(ops, NumericLengthBoundContainerOps(container, "<=", upper)...)
	}
	if !ApplyNumericEffect(&state, NumericEffect{Ops: ops}) {
		t.Fatalf("ApplyNumericEffect did not apply len bounds lower=%d upper=%d for %v", lower, upper, path)
	}
	return state.Num
}

func boundaryProjectionPathEqual(a, b BoundaryPath) bool {
	return a.Kind == b.Kind &&
		a.Index == b.Index &&
		slices.Equal(a.Segments, b.Segments)
}
