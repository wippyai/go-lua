package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPointFactsPathKeyPresenceQueries(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(101), "table")
	key := constraint.NewPath(cfg.SymbolID(102), "key")
	value := constraint.NewPath(cfg.SymbolID(103), "value")
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithAddresses(testStableAddressPath(t, table), testStableAddressPath(t, key)).
			WithValueAddresses(testStableAddressPath(t, table), testStableAddressPath(t, key), testStableAddressPath(t, value)),
	}
	facts := PointFactsOf(state)

	if !facts.HasKeyPresence(table, key) {
		t.Fatal("HasKeyPresence missed table/key fact")
	}
	if !facts.HasKeyValueReadbackSource(KeyValueReadbackSourceQuery{TablePath: table, KeyPath: key, ValuePath: value}) {
		t.Fatal("HasKeyValueReadbackSource missed table/key/value fact")
	}
	if facts.HasKeyPresence(table, constraint.NewPath(cfg.SymbolID(104), "other_key")) {
		t.Fatal("HasKeyPresence accepted unrelated key")
	}
}

func TestPointFactsReadPresentKeyValueRemovesFlowOptionalNil(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(105), "table")
	key := constraint.NewPath(cfg.SymbolID(106), "key")
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithAddresses(testStableAddressPath(t, table), testStableAddressPath(t, key)),
	}

	got := PointFactsOf(state).ReadPresentKeyValue(PresentKeyReadQuery{
		TablePath: table,
		KeyPath:   key,
		Result:    typ.NewOptional(typ.String),
	})

	if got.State != StateResolved || !typ.TypeEquals(got.Value.ProjectValue(), typ.String) {
		t.Fatalf("ReadPresentKeyValue = %v/%v, want string/resolved", got.Value.ProjectValue(), got.State)
	}
}

func TestPointFactsReadPresentKeyValueRejectsDefinitelyAbsentKey(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(107), "table")
	key := constraint.NewPath(cfg.SymbolID(108), "key")
	state := PointState{
		Env: map[ValueKey]product.AbstractValue{
			SymbolValueKey(key.Symbol): product.FromType(typ.Nil),
		},
		KeyPresence: KeyPresenceFacts{}.
			WithAddresses(testStableAddressPath(t, table), testStableAddressPath(t, key)),
	}

	got := PointFactsOf(state).ReadPresentKeyValue(PresentKeyReadQuery{
		TablePath: table,
		KeyPath:   key,
		Result:    typ.NewOptional(typ.String),
	})

	if got.State != StateUnknown {
		t.Fatalf("ReadPresentKeyValue(nil key) = %#v, want unknown", got)
	}
}

func TestPointFactsReadPresentRecordKeyValueSynthesizesClosedRecordFieldNames(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(109), "table")
	key := constraint.NewPath(cfg.SymbolID(110), "key")
	record := product.FromType(typ.NewRecord().
		Field("id", typ.String).
		Field("name", typ.String).
		Build())
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithAddresses(testStableAddressPath(t, table), testStableAddressPath(t, key)),
	}

	got := PointFactsOf(state).ReadPresentRecordKeyValue(table, key, record)
	want := typ.NewUnion(typ.LiteralString("id"), typ.LiteralString("name"))

	if got.State != StateResolved || !typ.TypeEquals(got.Value.ProjectValue(), want) {
		t.Fatalf("ReadPresentRecordKeyValue = %v/%v, want %v/resolved", got.Value.ProjectValue(), got.State, want)
	}
}

func TestPointFactsReadPresentRecordKeyValueRejectsOpenRecord(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(113), "table")
	key := constraint.NewPath(cfg.SymbolID(114), "key")
	record := product.FromType(typ.NewRecord().
		SetOpen(true).
		Field("id", typ.String).
		Build())
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithAddresses(testStableAddressPath(t, table), testStableAddressPath(t, key)),
	}

	got := PointFactsOf(state).ReadPresentRecordKeyValue(table, key, record)

	if got.State != StateUnknown {
		t.Fatalf("ReadPresentRecordKeyValue(open record) = %#v, want unknown", got)
	}
}

func TestPointFactsIndexWriteAdmissionPathQuery(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(111), "target")
	key := constraint.NewPath(cfg.SymbolID(112), "key")
	value := product.FromType(typ.String)
	state := PointState{
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target:     testStableAddressPath(t, target),
			KeyPath:    testStableAddressPath(t, key),
			HasKeyPath: true,
			Key:        product.FromType(typ.String),
			Value:      value,
		}),
	}
	facts := PointFactsOf(state)

	got, ok := facts.IndexWriteAdmission(IndexWritePathQuery{
		Target:     target,
		KeyPath:    key,
		HasKeyPath: true,
		KeyValue:   product.FromType(typ.String),
	})
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("IndexWriteAdmission = %v/%v, want string", got, ok)
	}
}

func TestPointFactsIndexWriteAdmissionPathQueryUsesValuePath(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(133), "target")
	key := constraint.NewPath(cfg.SymbolID(134), "key")
	source := constraint.NewPath(cfg.SymbolID(135), "source")
	otherSource := constraint.NewPath(cfg.SymbolID(136), "other")
	value := product.FromType(typ.String)
	state := PointState{
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target:       testStableAddressPath(t, target),
			KeyPath:      testStableAddressPath(t, key),
			HasKeyPath:   true,
			Key:          product.FromType(typ.String),
			ValuePath:    testStableAddressPath(t, source),
			HasValuePath: true,
			Value:        value,
		}),
	}
	facts := PointFactsOf(state)

	got, ok := facts.IndexWriteAdmission(IndexWritePathQuery{
		Target:       target,
		KeyPath:      key,
		HasKeyPath:   true,
		ValuePath:    source,
		HasValuePath: true,
	})
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("IndexWriteAdmission value path = %v/%v, want string", got, ok)
	}
	if got, ok := facts.IndexWriteAdmission(IndexWritePathQuery{
		Target:       target,
		KeyPath:      key,
		HasKeyPath:   true,
		ValuePath:    otherSource,
		HasValuePath: true,
	}); ok {
		t.Fatalf("IndexWriteAdmission other value path = %v/true, want false", got)
	}
}

func TestPointFactsDynamicIndexReadbackUsesStableKeyPath(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(113), "target")
	key := constraint.NewPath(cfg.SymbolID(114), "key")
	value := product.FromType(typ.Number)
	state := PointState{
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target:     testStableAddressPath(t, target),
			KeyPath:    testStableAddressPath(t, key),
			HasKeyPath: true,
			Key:        product.FromType(typ.String),
			Value:      value,
		}),
	}
	facts := PointFactsOf(state)

	got, ok := facts.DynamicIndexReadback(DynamicIndexReadbackQuery{
		Target:   target,
		KeyPath:  key,
		KeyValue: product.FromType(typ.String),
	})
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("DynamicIndexReadback path = %v/%v, want number", got, ok)
	}
}

func TestPointFactsDynamicIndexReadbackUsesStableKeyPathWithoutKeyValue(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(213), "target")
	key := constraint.NewPath(cfg.SymbolID(214), "key")
	value := product.FromType(typ.Number)
	facts := PointFactsOf(PointState{
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target:     testStableAddressPath(t, target),
			KeyPath:    testStableAddressPath(t, key),
			HasKeyPath: true,
			Key:        product.FromType(typ.String),
			Value:      value,
		}),
	})

	got, ok := facts.DynamicIndexReadback(DynamicIndexReadbackQuery{
		Target:  target,
		KeyPath: key,
	})
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("DynamicIndexReadback path without key value = %v/%v, want number", got, ok)
	}
}

func TestPointFactsDynamicIndexReadbackUsesIndexedIteratorKeyArrayFacts(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(130), "node_order")
	tablePath := constraint.NewPath(cfg.SymbolID(131), "nodes")
	keyPath := constraint.NewPath(cfg.SymbolID(132), "current_node_id")
	value := product.FromType(typ.String)
	state := PointState{
		KeyPresence: KeyPresenceFacts{}.
			WithKeyArrayValueAddresses(testStableAddressPath(t, arrayPath), testStableAddressPath(t, tablePath), value),
		ValueOrigins: ValueOriginFacts{}.
			WithAddresses(testStableAddressPath(t, keyPath), testStableAddressPath(t, arrayPath), ValueOriginIndexedIterator, 1),
	}
	facts := PointFactsOf(state)

	got, ok := facts.DynamicIndexReadback(DynamicIndexReadbackQuery{
		Target:   tablePath,
		KeyPath:  keyPath,
		KeyValue: product.FromType(typ.String),
	})
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("DynamicIndexReadback key-array = %v/%v, want string", got, ok)
	}
}

func TestPointFactsDynamicIndexReadbackUsesAppendElementFieldProvenance(t *testing.T) {
	routesPath := constraint.NewPath(cfg.SymbolID(137), "pending_routes")
	routeEntryPath := constraint.NewPath(cfg.SymbolID(138), "route_entry")
	lastNodePath := constraint.NewPath(cfg.SymbolID(139), "graph").Field("last_node_id")
	edgesPath := constraint.NewPath(cfg.SymbolID(140), "graph").Field("edges")
	field := []constraint.Segment{{Kind: constraint.SegmentField, Name: "from_node_id"}}
	edgeRecord := product.FromType(typ.NewRecord().
		Field("targets", typ.NewArray(typ.Any)).
		Field("error_targets", typ.NewArray(typ.Any)).
		Build())
	state := PointState{
		ValueOrigins: ValueOriginFacts{}.WithAddresses(
			testStableAddressPath(t, routeEntryPath),
			testStableAddressPath(t, routesPath),
			ValueOriginIndexedIterator,
			1,
		),
		KeyPresence: KeyPresenceFacts{}.
			WithAppendHistoryBaseAddress(testStableAddressPath(t, routesPath)).
			WithAppendElementFieldOriginFromAddresses(
				testStableAddressPath(t, routesPath),
				field,
				testStableAddressPath(t, lastNodePath),
				nil,
			),
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target:     testStableAddressPath(t, edgesPath),
			KeyPath:    testStableAddressPath(t, lastNodePath),
			HasKeyPath: true,
			Key:        product.FromType(typ.String),
			Value:      edgeRecord,
		}),
	}

	got, ok := PointFactsOf(state).DynamicIndexReadback(DynamicIndexReadbackQuery{
		Target:           edgesPath,
		KeyPath:          routeEntryPath.Field("from_node_id"),
		KeyValue:         product.FromType(typ.String),
		FollowKeyAliases: true,
	})
	if !ok || !product.Domain.Equal(got, edgeRecord) {
		t.Fatalf("DynamicIndexReadback append-field provenance = %v/%v, want edge record", got.ProjectValue(), ok)
	}
}

func TestPointFactsDynamicIndexReadbackAllowsLiteralValueOnly(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(115), "target")
	key := typ.LiteralString("id")
	value := product.FromType(typ.Boolean)
	state := PointState{
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target: testStableAddressPath(t, target),
			Key:    product.FromType(key),
			Value:  value,
		}),
	}
	facts := PointFactsOf(state)

	got, ok := facts.DynamicIndexReadback(DynamicIndexReadbackQuery{
		Target:   target,
		KeyValue: product.FromType(key),
	})
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("DynamicIndexReadback literal = %v/%v, want boolean", got, ok)
	}
}

func TestPointFactsDynamicIndexReadbackNormalizesOptionalLiteralKeyValue(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(117), "target")
	key := typ.LiteralString("id")
	value := product.FromType(typ.Boolean)
	state := PointState{
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target: testStableAddressPath(t, target),
			Key:    product.FromType(key),
			Value:  value,
		}),
	}
	facts := PointFactsOf(state)

	got, ok := facts.DynamicIndexReadback(DynamicIndexReadbackQuery{
		Target:   target,
		KeyValue: product.FromType(typ.NewOptional(key)),
	})
	if !ok || !product.Domain.Equal(got, value) {
		t.Fatalf("DynamicIndexReadback optional literal = %v/%v, want boolean", got, ok)
	}
}

func TestPointFactsDynamicIndexReadbackRejectsNonLiteralValueOnly(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(116), "target")
	state := PointState{
		IndexWrites: IndexWriteAdmissionFacts{}.WithAddress(IndexWriteAdmissionAddressFact{
			Target: testStableAddressPath(t, target),
			Key:    product.FromType(typ.String),
			Value:  product.FromType(typ.Number),
		}),
	}
	facts := PointFactsOf(state)

	if got, ok := facts.DynamicIndexReadback(DynamicIndexReadbackQuery{
		Target:   target,
		KeyValue: product.FromType(typ.String),
	}); ok {
		t.Fatalf("DynamicIndexReadback nonliteral value-only = %v/true, want false", got)
	}
}

func TestPointFactsIdentityAliasClosurePaths(t *testing.T) {
	root := constraint.NewPath(cfg.SymbolID(121), "root")
	source := constraint.NewPath(cfg.SymbolID(122), "source")
	state := PointState{
		PathAliases: PathAliasFacts{}.WithAddresses(
			testStableAddressPath(t, root),
			testStableAddressPath(t, source),
		),
	}
	facts := PointFactsOf(state)

	got := facts.IdentityAliasClosurePaths(root)
	if len(got) != 2 {
		t.Fatalf("IdentityAliasClosurePaths got %d paths, want root + source", len(got))
	}
	if !got[0].Equal(root) || !got[1].Equal(source) {
		t.Fatalf("IdentityAliasClosurePaths = %v, want %s then %s", got, root.String(), source.String())
	}
}
