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

	facts := BoundaryFactsOf(
		[]BoundaryKeyPresenceFact{{Table: paramTable, Key: paramKey}},
		[]BoundaryKeyArrayFact{{Array: ret0Array, Table: paramTable}},
		[]BoundaryKeyArrayValueFact{{Array: ret0Array, Table: ret0Table, Value: product.FromType(typ.String)}},
		[]BoundaryAppendKeyFact{{Array: ret0Array, Key: ret1Key}},
		[]BoundaryLengthLowerBound{{Target: ret1Key, Lower: 1}},
		[]BoundaryIndexWriteFact{{Table: ret0Table, Key: ret1Key, Value: product.FromType(typ.Number)}},
	).WithAppendElementFieldOrigins([]BoundaryAppendElementFieldOriginFact{{
		Array:  ret0Array,
		Field:  []constraint.Segment{{Kind: constraint.SegmentField, Name: "child"}},
		Source: ret1Key,
	}}).WithLengthRelations([]BoundaryLengthRelationFact{{
		Target: ret0Array,
		Source: paramTable,
	}})

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
		BoundaryFactsOf([]BoundaryKeyPresenceFact{{Table: table, Key: key}}, nil, nil, nil, nil, nil),
		BoundaryFactsOf(nil, []BoundaryKeyArrayFact{{Array: array, Table: table}}, nil, nil, nil, nil),
	)
	if len(merged.KeyPresence()) != 1 || len(merged.KeyArrays()) != 1 {
		t.Fatalf("merged facts = %#v, want union of independent proofs", merged)
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
