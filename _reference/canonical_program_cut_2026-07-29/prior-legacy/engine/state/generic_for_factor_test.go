package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestGenericForFactorTransactionAcceptsValuesOnlyProduct(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{LaneValues})
	if err != nil {
		t.Fatal(err)
	}
	keys := keyspace.New()
	transaction, err := domain.PrepareGenericForFactorTransaction(GenericForFactorConfig{
		Keys: keys, VariableIndex: 0, Target: keys.FromPath(pathdom.Path{Symbol: symbol.ID(91)}),
	})
	if err != nil || !transaction.Valid() {
		t.Fatalf("values-only generic-for transaction = %#v, %v", transaction, err)
	}
	if len(transaction.SourceLanes()) != 0 || len(transaction.CurrentLanes()) != 0 || len(transaction.WriteLanes()) != 0 {
		t.Fatal("values-only generic-for transaction invented a residual factor")
	}
	writes, err := transaction.Apply(nil, nil)
	if err != nil || len(writes) != 0 {
		t.Fatalf("values-only factor application = %v, %v", writes, err)
	}
}

func TestGenericForIndexedFactorTransactionCopiesCommonShapeAndOrigin(t *testing.T) {
	reg := standard.Registry()
	domain, err := TryRegisteredProductDomainWithLanes(reg, []LaneID{
		LanePathEvidence, LaneDynamicIndex, LaneKeyMemberships,
	})
	if err != nil {
		t.Fatal(err)
	}
	keys := keyspace.New()
	container := keys.FromPath(pathdom.Path{Symbol: 301, Version: 1})
	target := keys.FromPath(pathdom.Path{Symbol: 302, Version: 1})
	field := segment.Segment{Kind: segment.SegmentField, Name: "name"}
	member := func(index int) keyspace.Key {
		item, ok := keys.AppendSegment(container, segment.Segment{Kind: segment.SegmentIndexInt, Index: index})
		if !ok {
			t.Fatal("indexed member")
		}
		out, ok := keys.AppendSegment(item, field)
		if !ok {
			t.Fatal("indexed field")
		}
		return out
	}
	want := typevalue.LiteralString(reg, "shared")
	site := dynamicindex.SiteForPoint(9)
	source := Reachable(State{}).
		WriteLocalPathStaticMember(member(1), want).
		WriteLocalPathStaticMember(member(2), want).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: container, Site: site}, dynamicindex.NewFact(reg, dynamicindex.FactConfig{
			KeyValue: typevalue.LiteralInt(reg, 1), HasKeyValue: true,
			Value: want, HasValue: true, Admission: dynamicindex.AdmissionAdmitted,
		}))
	targetState, ok := pathaddr.StateKeyFromPathKey(keys.FormatReadOnly(target))
	if !ok {
		t.Fatal("target StateKey")
	}
	current := Reachable(State{}).AddPathKeyMembership(targetState, targetState)
	transaction, err := domain.PrepareGenericForFactorTransaction(GenericForFactorConfig{
		Keys: keys, Iterator: iteration.IterateIndexed, HasIterator: true, VariableIndex: 1,
		Target: target, SourceContainer: container, SourceTable: container, TypeValues: typevalue.NewCache(),
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceFactors, err := domain.DecomposeLanes(source, transaction.SourceLanes())
	if err != nil {
		t.Fatal(err)
	}
	currentFactors, err := domain.DecomposeLanes(current, transaction.CurrentLanes())
	if err != nil {
		t.Fatal(err)
	}
	writes, err := transaction.Apply(sourceFactors, currentFactors)
	if err != nil {
		t.Fatal(err)
	}
	delta, err := domain.ComposeSparse(writes)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]LaneID, len(transaction.WriteLanes()))
	for index, lane := range transaction.WriteLanes() {
		ids[index] = lane.ID()
	}
	result, err := domain.PatchFactors(current, delta, NewLaneSet(ids...))
	if err != nil {
		t.Fatal(err)
	}
	targetField, _ := keys.AppendSegment(target, field)
	got, present := result.ReadLocalPathStaticMember(targetField)
	if !present || !product.Equal(reg, got, want) {
		t.Fatalf("indexed common field = %#v/%t, want %#v", got, present, want)
	}
	origin := DynamicIndexValueOrigin{Value: targetState, Container: container, Site: site}
	if _, present := result.keyMemberships.valueOrigins[origin]; !present {
		t.Fatal("indexed value origin was not published")
	}
	if result.HasPathKeyMembership(targetState, targetState) {
		t.Fatal("stale target membership survived indexed rebinding")
	}
}
