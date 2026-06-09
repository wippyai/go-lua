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
