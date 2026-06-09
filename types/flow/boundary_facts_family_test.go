package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBoundaryKeyIndexFamilyLaws(t *testing.T) {
	table := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}}}
	key := BoundaryPath{Kind: BoundaryPathParam, Index: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}}
	array := BoundaryPath{Kind: BoundaryPathReturn, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "order"}}}
	value := product.FromType(typ.String)
	facts := BoundaryFactsFromParts(BoundaryFactParts{
		KeyPresence:    []BoundaryKeyPresenceFact{{Table: table, Key: key}},
		KeyArrays:      []BoundaryKeyArrayFact{{Array: array, Table: table}},
		KeyArrayValues: []BoundaryKeyArrayValueFact{{Array: array, Table: table, Value: value}},
		IndexWrites: []BoundaryIndexWriteFact{{
			Table:      table,
			KeyPath:    key,
			HasKeyPath: true,
			KeyValue:   product.FromType(typ.String),
			Value:      product.FromType(typ.Number),
		}},
	})
	same := BoundaryFactsFromParts(facts.Parts())

	if !BoundaryFactsDomain.Equal(facts, same) {
		t.Fatalf("key/index family equality failed: %#v vs %#v", facts, same)
	}
	if joined := BoundaryFactsDomain.Join(facts, same); !BoundaryFactsDomain.Equal(joined, facts) {
		t.Fatalf("key/index family join = %#v, want %#v", joined, facts)
	}
	if widened := BoundaryFactsDomain.Widen(facts, same); !BoundaryFactsDomain.Equal(widened, facts) {
		t.Fatalf("key/index family widen = %#v, want %#v", widened, facts)
	}
	if dropped := BoundaryFactsDomain.Join(facts, BoundaryFactsDomain.Top()); dropped.HasProof() {
		t.Fatalf("key/index must-facts survived missing branch: %#v", dropped)
	}
}

func TestBoundaryAppendHistoryFamilyLaws(t *testing.T) {
	array := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "pending"}}}
	key := BoundaryPath{Kind: BoundaryPathParam, Index: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}}
	table := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}}}
	source := BoundaryPath{Kind: BoundaryPathReturn, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "node"}}}
	value := product.FromType(typ.String)
	facts := BoundaryFactsFromParts(BoundaryFactParts{
		AppendKeys:          []BoundaryAppendKeyFact{{Array: array, Key: key, Table: table, HasTable: true}},
		AppendBases:         []BoundaryAppendHistoryBaseFact{{Array: array}},
		AppendEvents:        []BoundaryAppendHistoryEventFact{{Array: array, Key: key}},
		AppendCoverage:      []BoundaryAppendHistoryCoverageFact{{Array: array, Key: key, Table: table, Value: value}},
		AppendTableCoverage: []BoundaryAppendHistoryTableCoverageFact{{Array: array, Table: table, Value: value}},
		AppendOrigins: []BoundaryAppendElementFieldOriginFact{{
			Array:       array,
			Field:       []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}},
			Source:      source,
			SourceField: []constraint.Segment{{Kind: constraint.SegmentField, Name: "raw"}},
		}},
	})
	same := BoundaryFactsFromParts(facts.Parts())

	if !BoundaryFactsDomain.Equal(facts, same) {
		t.Fatalf("append family equality failed: %#v vs %#v", facts, same)
	}
	if joined := BoundaryFactsDomain.Join(facts, same); !BoundaryFactsDomain.Equal(joined, facts) {
		t.Fatalf("append family join = %#v, want %#v", joined, facts)
	}
	if widened := BoundaryFactsDomain.Widen(facts, same); !BoundaryFactsDomain.Equal(widened, facts) {
		t.Fatalf("append family widen = %#v, want %#v", widened, facts)
	}

	baseOnly := BoundaryFactsFromParts(BoundaryFactParts{
		AppendBases: []BoundaryAppendHistoryBaseFact{{Array: array}},
	})
	joined := BoundaryFactsDomain.Join(facts, baseOnly)
	if len(joined.AppendHistoryEvents()) != 1 ||
		len(joined.AppendHistoryCoverage()) != 1 ||
		len(joined.AppendHistoryTableCoverage()) != 1 ||
		len(joined.AppendElementFieldOrigins()) != 1 {
		t.Fatalf("append family base-survival join = %#v, want event/coverage/origin proof", joined)
	}
	if keys := joined.AppendKeys(); len(keys) != 0 {
		t.Fatalf("definite append key survived missing branch: %#v", keys)
	}
}

func TestBoundaryFamilyIdentityHashIncludesFamilyLanes(t *testing.T) {
	table := BoundaryPath{Kind: BoundaryPathParam, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}}}
	key := BoundaryPath{Kind: BoundaryPathParam, Index: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}}
	array := BoundaryPath{Kind: BoundaryPathParam, Index: 2, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "order"}}}
	value := product.FromType(typ.String)
	seed := "family-hash"
	emptyHash := BoundaryFactsDomain.Top().IdentityHash(seed)
	cases := []struct {
		name  string
		parts BoundaryFactParts
	}{
		{
			name:  "key/index",
			parts: BoundaryFactParts{KeyPresence: []BoundaryKeyPresenceFact{{Table: table, Key: key}}},
		},
		{
			name:  "append",
			parts: BoundaryFactParts{AppendBases: []BoundaryAppendHistoryBaseFact{{Array: array}}},
		},
		{
			name:  "length",
			parts: BoundaryFactParts{LengthLower: []BoundaryLengthLowerBound{{Target: array, Lower: 1}}},
		},
		{
			name:  "static",
			parts: BoundaryFactParts{StaticMembers: []BoundaryStaticMemberFact{{Target: table, Value: value}}},
		},
	}
	for _, tc := range cases {
		facts := BoundaryFactsFromParts(tc.parts)
		if got := facts.IdentityHash(seed); got == emptyHash {
			t.Fatalf("%s family hash = empty hash %d", tc.name, got)
		}
		if got, want := facts.IdentityHash(seed), BoundaryFactsFromParts(facts.Parts()).IdentityHash(seed); got != want {
			t.Fatalf("%s family hash = %d, want canonical clone hash %d", tc.name, got, want)
		}
	}
}

func TestRebaseBoundaryReturnFactsToParamRebasesFamilyLanes(t *testing.T) {
	retArray := BoundaryPath{Kind: BoundaryPathReturn, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "order"}}}
	retTable := BoundaryPath{Kind: BoundaryPathReturn, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}}}
	retKey := BoundaryPath{Kind: BoundaryPathReturn, Index: 0, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}}
	otherRetKey := BoundaryPath{Kind: BoundaryPathReturn, Index: 1, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}}
	paramArray := BoundaryPath{Kind: BoundaryPathParam, Index: 2, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "order"}}}
	paramTable := BoundaryPath{Kind: BoundaryPathParam, Index: 2, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "nodes"}}}
	paramKey := BoundaryPath{Kind: BoundaryPathParam, Index: 2, Segments: []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}}}
	value := product.FromType(typ.String)
	facts := BoundaryFactsFromParts(BoundaryFactParts{
		KeyPresence: []BoundaryKeyPresenceFact{
			{Table: retTable, Key: retKey},
			{Table: retTable, Key: otherRetKey},
		},
		KeyArrays:           []BoundaryKeyArrayFact{{Array: retArray, Table: retTable}},
		KeyArrayValues:      []BoundaryKeyArrayValueFact{{Array: retArray, Table: retTable, Value: value}},
		AppendKeys:          []BoundaryAppendKeyFact{{Array: retArray, Key: retKey, Table: retTable, HasTable: true}},
		AppendBases:         []BoundaryAppendHistoryBaseFact{{Array: retArray}},
		AppendEvents:        []BoundaryAppendHistoryEventFact{{Array: retArray, Key: retKey}},
		AppendCoverage:      []BoundaryAppendHistoryCoverageFact{{Array: retArray, Key: retKey, Table: retTable, Value: value}},
		AppendTableCoverage: []BoundaryAppendHistoryTableCoverageFact{{Array: retArray, Table: retTable, Value: value}},
		AppendOrigins: []BoundaryAppendElementFieldOriginFact{{
			Array:       retArray,
			Field:       []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}},
			Source:      retKey,
			SourceField: []constraint.Segment{{Kind: constraint.SegmentField, Name: "raw"}},
		}},
		LengthLower:     []BoundaryLengthLowerBound{{Target: retArray, Lower: 1}},
		LengthUpper:     []BoundaryLengthUpperBound{{Target: retArray, Upper: 4}},
		LengthRelations: []BoundaryLengthRelationFact{{Target: retArray, Source: retTable}},
		IndexWrites: []BoundaryIndexWriteFact{{
			Table:        retTable,
			KeyPath:      retKey,
			HasKeyPath:   true,
			KeyValue:     value,
			ValuePath:    retArray,
			HasValuePath: true,
			Value:        product.FromType(typ.Number),
		}},
		StaticMembers: []BoundaryStaticMemberFact{{Target: retTable, Value: value}},
	})
	want := BoundaryFactsFromParts(BoundaryFactParts{
		KeyPresence:         []BoundaryKeyPresenceFact{{Table: paramTable, Key: paramKey}},
		KeyArrays:           []BoundaryKeyArrayFact{{Array: paramArray, Table: paramTable}},
		KeyArrayValues:      []BoundaryKeyArrayValueFact{{Array: paramArray, Table: paramTable, Value: value}},
		AppendKeys:          []BoundaryAppendKeyFact{{Array: paramArray, Key: paramKey, Table: paramTable, HasTable: true}},
		AppendBases:         []BoundaryAppendHistoryBaseFact{{Array: paramArray}},
		AppendEvents:        []BoundaryAppendHistoryEventFact{{Array: paramArray, Key: paramKey}},
		AppendCoverage:      []BoundaryAppendHistoryCoverageFact{{Array: paramArray, Key: paramKey, Table: paramTable, Value: value}},
		AppendTableCoverage: []BoundaryAppendHistoryTableCoverageFact{{Array: paramArray, Table: paramTable, Value: value}},
		AppendOrigins: []BoundaryAppendElementFieldOriginFact{{
			Array:       paramArray,
			Field:       []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}},
			Source:      paramKey,
			SourceField: []constraint.Segment{{Kind: constraint.SegmentField, Name: "raw"}},
		}},
		LengthLower:     []BoundaryLengthLowerBound{{Target: paramArray, Lower: 1}},
		LengthUpper:     []BoundaryLengthUpperBound{{Target: paramArray, Upper: 4}},
		LengthRelations: []BoundaryLengthRelationFact{{Target: paramArray, Source: paramTable}},
		IndexWrites: []BoundaryIndexWriteFact{{
			Table:        paramTable,
			KeyPath:      paramKey,
			HasKeyPath:   true,
			KeyValue:     value,
			ValuePath:    paramArray,
			HasValuePath: true,
			Value:        product.FromType(typ.Number),
		}},
		StaticMembers: []BoundaryStaticMemberFact{{Target: paramTable, Value: value}},
	})

	got := RebaseBoundaryReturnFactsToParam(facts, 0, 2)
	if !BoundaryFactsDomain.Equal(got, want) {
		t.Fatalf("rebased facts = %#v, want %#v", got, want)
	}
}
