package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBoundaryFactsPartitionByReturnIndices(t *testing.T) {
	paramTable := BoundaryPath{
		Kind:     BoundaryPathParam,
		Index:    0,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}},
	}
	paramKey := BoundaryPath{
		Kind:     BoundaryPathParam,
		Index:    1,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}},
	}
	ret0Array := BoundaryPath{
		Kind:     BoundaryPathReturn,
		Index:    0,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "node_order"}},
	}
	ret0Table := BoundaryPath{
		Kind:     BoundaryPathReturn,
		Index:    0,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}},
	}
	ret1Key := BoundaryPath{
		Kind:     BoundaryPathReturn,
		Index:    1,
		Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}},
	}

	facts := BoundaryFactsFromParts(BoundaryFactParts{
		KeyPresence:    []BoundaryKeyPresenceFact{{Table: paramTable, Key: paramKey}},
		KeyArrays:      []BoundaryKeyArrayFact{{Array: ret0Array, Table: paramTable}},
		KeyArrayValues: []BoundaryKeyArrayValueFact{{Array: ret0Array, Table: ret0Table, Value: product.FromType(typ.String)}},
		AppendKeys:     []BoundaryAppendKeyFact{{Array: ret0Array, Key: ret1Key}},
		LengthLower:    []BoundaryLengthLowerBound{{Target: ret1Key, Lower: 1}},
		IndexWrites: []BoundaryIndexWriteFact{{
			Table:      ret0Table,
			KeyPath:    ret1Key,
			HasKeyPath: true,
			KeyValue:   product.FromType(typ.String),
			Value:      product.FromType(typ.Number),
		}},
	}).WithAppendElementFieldOrigins([]BoundaryAppendElementFieldOriginFact{{
		Array:  ret0Array,
		Field:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "child"}},
		Source: ret1Key,
	}}).WithLengthRelations([]BoundaryLengthRelationFact{{
		Target: ret0Array,
		Source: paramTable,
	}}).WithLengthUpperBounds([]BoundaryLengthUpperBound{{Target: ret1Key, Upper: 0}})

	params, buckets := facts.PartitionByReturnIndices()
	if len(params.KeyPresence()) != 1 {
		t.Fatalf("param facts = %#v, want one parameter-only key-presence fact", params)
	}
	if len(buckets) != 3 {
		t.Fatalf("return buckets = %#v, want buckets for [0], [0,1], and [1]", buckets)
	}
	if bucket := boundaryBucketForTest(t, buckets, []int{0}); len(bucket.Facts().KeyArrays()) != 1 || len(bucket.Facts().KeyArrayValues()) != 1 {
		t.Fatalf("bucket [0] = %#v, want key-array and key-array-value facts", bucket.Facts())
	}
	if bucket := boundaryBucketForTest(t, buckets, []int{0}); len(bucket.Facts().LengthRelations()) != 1 {
		t.Fatalf("bucket [0] = %#v, want length relation fact", bucket.Facts())
	}
	if bucket := boundaryBucketForTest(t, buckets, []int{1}); len(bucket.Facts().LengthLowerBounds()) != 1 {
		t.Fatalf("bucket [1] = %#v, want length lower bound", bucket.Facts())
	}
	if bucket := boundaryBucketForTest(t, buckets, []int{1}); len(bucket.Facts().LengthUpperBounds()) != 1 {
		t.Fatalf("bucket [1] = %#v, want length upper bound", bucket.Facts())
	}
	if bucket := boundaryBucketForTest(t, buckets, []int{0, 1}); len(bucket.Facts().IndexWrites()) != 1 {
		t.Fatalf("bucket [0,1] = %#v, want index write", bucket.Facts())
	}
	if bucket := boundaryBucketForTest(t, buckets, []int{0, 1}); len(bucket.Facts().AppendKeys()) != 1 {
		t.Fatalf("bucket [0,1] = %#v, want append-key fact", bucket.Facts())
	}
	if bucket := boundaryBucketForTest(t, buckets, []int{0, 1}); len(bucket.Facts().AppendElementFieldOrigins()) != 1 {
		t.Fatalf("bucket [0,1] = %#v, want append-element-field origin", bucket.Facts())
	}
}

func TestMergeBoundaryFactProofsUnionsIndependentProofs(t *testing.T) {
	table := BoundaryPath{Kind: BoundaryPathParam, Index: 0}
	key := BoundaryPath{Kind: BoundaryPathParam, Index: 1}
	array := BoundaryPath{Kind: BoundaryPathReturn, Index: 0}

	merged := MergeBoundaryFactProofs(
		BoundaryFactsFromParts(BoundaryFactParts{
			KeyPresence: []BoundaryKeyPresenceFact{{Table: table, Key: key}},
		}),
		BoundaryFactsFromParts(BoundaryFactParts{
			KeyArrays: []BoundaryKeyArrayFact{{Array: array, Table: table}},
		}),
	)
	if len(merged.KeyPresence()) != 1 || len(merged.KeyArrays()) != 1 {
		t.Fatalf("merged facts = %#v, want union of independent proofs", merged)
	}
}

func TestBoundaryFactsPartsBuildsAndExportsAllLanesAliasFree(t *testing.T) {
	table := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}}}
	key := BoundaryPath{Kind: BoundaryPathParam, Index: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}}
	array := BoundaryPath{Kind: BoundaryPathReturn, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "order"}}}
	value := product.FromType(typ.String)
	facts := BoundaryFactsFromParts(BoundaryFactParts{
		KeyPresence:         []BoundaryKeyPresenceFact{{Table: table, Key: key}},
		KeyArrays:           []BoundaryKeyArrayFact{{Array: array, Table: table}},
		KeyArrayValues:      []BoundaryKeyArrayValueFact{{Array: array, Table: table, Value: value}},
		AppendKeys:          []BoundaryAppendKeyFact{{Array: array, Key: key, Table: table, HasTable: true}},
		AppendBases:         []BoundaryAppendHistoryBaseFact{{Array: array}},
		AppendEvents:        []BoundaryAppendHistoryEventFact{{Array: array, Key: key}},
		AppendCoverage:      []BoundaryAppendHistoryCoverageFact{{Array: array, Key: key, Table: table, Value: value}},
		AppendTableCoverage: []BoundaryAppendHistoryTableCoverageFact{{Array: array, Table: table, Value: value}},
		AppendOrigins: []BoundaryAppendElementFieldOriginFact{{
			Array:       array,
			Field:       []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}},
			Source:      key,
			SourceField: []constraint.Segment{{Kind: constraint.SegmentField, Name: "source"}},
		}},
		LengthLower:     []BoundaryLengthLowerBound{{Target: array, Lower: 1}},
		LengthUpper:     []BoundaryLengthUpperBound{{Target: array, Upper: 4}},
		LengthRelations: []BoundaryLengthRelationFact{{Target: array, Source: table}},
		IndexWrites: []BoundaryIndexWriteFact{{
			Table:        table,
			KeyPath:      key,
			HasKeyPath:   true,
			KeyValue:     value,
			ValuePath:    array,
			HasValuePath: true,
			Value:        product.FromType(typ.Number),
		}},
		StaticMembers: []BoundaryStaticMemberFact{{Target: table, Value: value}},
	})
	parts := facts.Parts()
	if len(parts.KeyPresence) != 1 || len(parts.KeyArrays) != 1 || len(parts.KeyArrayValues) != 1 ||
		len(parts.AppendKeys) != 1 || len(parts.AppendBases) != 1 || len(parts.AppendEvents) != 1 ||
		len(parts.AppendCoverage) != 1 || len(parts.AppendTableCoverage) != 1 || len(parts.AppendOrigins) != 1 ||
		len(parts.LengthLower) != 1 || len(parts.LengthUpper) != 1 || len(parts.LengthRelations) != 1 ||
		len(parts.IndexWrites) != 1 || len(parts.StaticMembers) != 1 {
		t.Fatalf("parts = %#v, want every boundary lane populated once", parts)
	}

	parts.AppendOrigins[0].Field[0].Name = "mutated"
	if got := facts.AppendElementFieldOrigins()[0].Field[0].Name; got != "id" {
		t.Fatalf("Parts leaked append-origin field slice alias: got %q", got)
	}
}

func TestBoundaryFactsJoinKeepsAppendFieldOriginWhenBaseSurvives(t *testing.T) {
	array := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "pending_routes"}}}
	source := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "last_node_id"}}}
	origin := BoundaryAppendElementFieldOriginFact{
		Array:  array,
		Field:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "from_node_id"}},
		Source: source,
	}
	base := BoundaryAppendHistoryBaseFact{Array: array}

	appended := BoundaryFactsDomain.Top().
		WithAppendHistoryBases([]BoundaryAppendHistoryBaseFact{base}).
		WithAppendElementFieldOrigins([]BoundaryAppendElementFieldOriginFact{origin})
	notAppended := BoundaryFactsDomain.Top().
		WithAppendHistoryBases([]BoundaryAppendHistoryBaseFact{base})

	got := BoundaryFactsDomain.Join(appended, notAppended)
	if bases := got.AppendHistoryBases(); len(bases) != 1 || compareBoundaryAppendHistoryBase(bases[0], base) != 0 {
		t.Fatalf("append bases = %#v, want %#v", bases, base)
	}
	if origins := got.AppendElementFieldOrigins(); len(origins) != 1 || compareBoundaryAppendElementFieldOrigin(origins[0], origin) != 0 {
		t.Fatalf("append origins = %#v, want %#v", origins, origin)
	}

	withoutBase := BoundaryFactsDomain.Join(appended, BoundaryFactsDomain.Top())
	if origins := withoutBase.AppendElementFieldOrigins(); len(origins) != 0 {
		t.Fatalf("append origins without surviving base = %#v, want none", origins)
	}
}

func TestBoundaryFactsIdentityHashTracksCanonicalLanes(t *testing.T) {
	target := BoundaryPath{Kind: BoundaryPathParam, Index: 0}
	relA := BoundaryLengthRelationFact{
		Target: target,
		Source: BoundaryPath{Kind: BoundaryPathParam, Index: 1},
	}
	relB := BoundaryLengthRelationFact{
		Target: target,
		Source: BoundaryPath{Kind: BoundaryPathParam, Index: 2},
	}
	factsA := BoundaryFactsDomain.Top().WithLengthRelations([]BoundaryLengthRelationFact{relA})
	factsACopy := BoundaryFactsDomain.Top().WithLengthRelations([]BoundaryLengthRelationFact{relA})
	factsB := BoundaryFactsDomain.Top().WithLengthRelations([]BoundaryLengthRelationFact{relB})
	factsUpper := BoundaryFactsDomain.Top().WithLengthUpperBounds([]BoundaryLengthUpperBound{{Target: target, Upper: 0}})

	if got, want := factsA.IdentityHash("test"), factsACopy.IdentityHash("test"); got != want {
		t.Fatalf("same boundary facts hash = %d, want %d", got, want)
	}
	if got, other := factsA.IdentityHash("test"), factsB.IdentityHash("test"); got == other {
		t.Fatalf("distinct boundary length relations collapsed to hash %d", got)
	}
	if got, other := factsA.IdentityHash("test"), factsUpper.IdentityHash("test"); got == other {
		t.Fatalf("length relation and length upper facts collapsed to hash %d", got)
	}
}

func boundaryBucketForTest(t *testing.T, buckets []BoundaryReturnFactBucket, indices []int) BoundaryReturnFactBucket {
	t.Helper()
	for _, bucket := range buckets {
		if intsEqual(bucket.Indices(), indices) {
			return bucket
		}
	}
	t.Fatalf("bucket for indices %v not found in %#v", indices, buckets)
	return BoundaryReturnFactBucket{}
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
