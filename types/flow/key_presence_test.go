package flow

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

func TestKeyPresenceFactsDomain_Laws(t *testing.T) {
	lattice.LawSuite[KeyPresenceFacts]{
		Name:   "KeyPresenceFacts",
		Domain: KeyPresenceFactsDomain,
		Sample: keyPresenceFactsSample(),
		Format: KeyPresenceFacts.Format,
	}.Run(t)
}

func TestKeyPresenceFactsJoinKeepsCommonProvenPairs(t *testing.T) {
	table := SymbolPathKey(cfg.SymbolID(1), nil)
	keyA := SymbolPathKey(cfg.SymbolID(2), nil)
	keyB := SymbolPathKey(cfg.SymbolID(3), nil)

	left := KeyPresenceFactsOf([]KeyPresenceFact{
		{Table: table, Key: keyA},
		{Table: table, Key: keyB},
	})
	right := KeyPresenceFactsOf([]KeyPresenceFact{
		{Table: table, Key: keyA},
	})

	joined := KeyPresenceFactsDomain.Join(left, right)
	if !joined.Has(table, keyA) {
		t.Fatal("join dropped common key-presence proof")
	}
	if joined.Has(table, keyB) {
		t.Fatal("join kept key-presence proof not present on every predecessor")
	}
}

func TestKeyPresenceCanonicalizationReusesCanonicalRows(t *testing.T) {
	table := SymbolPathKey(cfg.SymbolID(1), nil)
	keyA := SymbolPathKey(cfg.SymbolID(2), nil)
	keyB := SymbolPathKey(cfg.SymbolID(3), nil)
	rows := []KeyPresenceFact{
		{Table: table, Key: keyA},
		{Table: table, Key: keyB},
	}

	facts := canonicalKeyPresenceFactSet(keyPresenceFactSet{entries: rows})
	if !facts.Has(table, keyA) || !facts.Has(table, keyB) {
		t.Fatalf("canonical rows lost key-presence facts: %s", facts.Format())
	}
	if reflect.ValueOf(facts.rows.entries).Pointer() != reflect.ValueOf(rows).Pointer() {
		t.Fatalf("canonical key-presence rows were copied")
	}

	filtered := canonicalKeyPresenceFactSet(keyPresenceFactSet{entries: []KeyPresenceFact{
		{Table: table, Key: keyA},
		{Table: "", Key: keyB},
	}})
	if filtered.Has("", keyB) {
		t.Fatalf("invalid key-presence row survived canonicalization: %s", filtered.Format())
	}
}

func TestKeyPresenceFactsJoinSpecializesEmptyKeyArray(t *testing.T) {
	array := SymbolPathKey(cfg.SymbolID(1), nil)
	table := SymbolPathKey(cfg.SymbolID(2), nil)
	otherTable := SymbolPathKey(cfg.SymbolID(3), nil)

	empty := KeyPresenceFacts{}.WithEmptyKeyArray(array)
	backedge := KeyPresenceFacts{}.
		WithKeyArray(array, table).
		WithKeyArray(otherTable, table)

	joined := KeyPresenceFactsDomain.Join(empty, backedge)
	if joined.HasEmptyKeyArray(array) {
		t.Fatalf("join with concrete backedge kept stale empty-array fact: %s", joined.Format())
	}
	if tables := joined.KeyArrayTables(array); len(tables) != 1 || tables[0] != table {
		t.Fatalf("join did not specialize empty array to observed table: %s", joined.Format())
	}
	if tables := joined.KeyArrayTables(otherTable); len(tables) != 0 {
		t.Fatalf("join specialized unrelated array: %s", joined.Format())
	}
}

func TestKeyPresenceAppendHistoryJoinUnionsCoveredEvents(t *testing.T) {
	array := SymbolPathKey(cfg.SymbolID(10), nil)
	nodes := SymbolPathKey(cfg.SymbolID(11), nil)
	edges := SymbolPathKey(cfg.SymbolID(12), nil)
	keyA := SymbolPathKey(cfg.SymbolID(13), nil)
	keyB := SymbolPathKey(cfg.SymbolID(14), nil)
	nodeValue := product.FromType(typ.String)
	edgeValue := product.FromType(typ.Number)

	left := KeyPresenceFacts{}.
		WithEmptyKeyArray(array).
		WithAppendHistoryEvent(array, keyA).
		WithAppendHistoryCoverage(array, keyA, nodes, nodeValue).
		WithAppendHistoryCoverage(array, keyA, edges, edgeValue)
	right := KeyPresenceFacts{}.
		WithAppendHistoryBase(array).
		WithAppendHistoryEvent(array, keyB).
		WithAppendHistoryCoverage(array, keyB, nodes, nodeValue).
		WithAppendHistoryCoverage(array, keyB, edges, edgeValue)

	joined := KeyPresenceFactsDomain.Join(left, right)
	tables := joined.KeyArrayTables(array)
	if len(tables) != 2 {
		t.Fatalf("covered tables = %v, want nodes and edges; facts=%s", tables, joined.Format())
	}
	var tableSet pathKeySet
	for _, table := range tables {
		tableSet.Add(table)
	}
	if !tableSet.Contains(nodes) {
		t.Fatalf("covered tables missing nodes: %v facts=%s", tables, joined.Format())
	}
	if !tableSet.Contains(edges) {
		t.Fatalf("covered tables missing edges: %v facts=%s", tables, joined.Format())
	}
	nodeValues := joined.KeyArrayValues(array, nodes)
	if len(nodeValues) != 1 || !product.Domain.Equal(nodeValues[0], nodeValue) {
		t.Fatalf("node coverage values = %v, want string; facts=%s", nodeValues, joined.Format())
	}
}

func TestKeyPresenceAppendHistoryRejectsPartiallyCoveredTable(t *testing.T) {
	array := SymbolPathKey(cfg.SymbolID(20), nil)
	nodes := SymbolPathKey(cfg.SymbolID(21), nil)
	edges := SymbolPathKey(cfg.SymbolID(22), nil)
	keyA := SymbolPathKey(cfg.SymbolID(23), nil)
	keyB := SymbolPathKey(cfg.SymbolID(24), nil)

	facts := KeyPresenceFacts{}.
		WithAppendHistoryBase(array).
		WithAppendHistoryEvent(array, keyA).
		WithAppendHistoryEvent(array, keyB).
		WithAppendHistoryCoverage(array, keyA, nodes, product.FromType(typ.String)).
		WithAppendHistoryCoverage(array, keyA, edges, product.FromType(typ.Number)).
		WithAppendHistoryCoverage(array, keyB, edges, product.FromType(typ.Number))

	if values := facts.KeyArrayValues(array, nodes); len(values) != 0 {
		t.Fatalf("partially covered nodes table produced values: %v facts=%s", values, facts.Format())
	}
	if values := facts.KeyArrayValues(array, edges); len(values) != 1 || !product.Domain.Equal(values[0], product.FromType(typ.Number)) {
		t.Fatalf("fully covered edges table values = %v facts=%s", values, facts.Format())
	}
}

func TestAppendElementFieldOriginUseSourcePath(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(30), "routes")
	sourcePath := constraint.NewPath(cfg.SymbolID(31), "op")
	field := []constraint.Segment{{Kind: constraint.SegmentField, Name: "target_name"}}
	facts := KeyPresenceFacts{}.
		WithAppendHistoryBaseAddress(testStableAddressPath(t, arrayPath)).
		WithAppendElementFieldOriginFromAddresses(
			testStableAddressPath(t, arrayPath),
			field,
			testStableAddressPath(t, sourcePath),
			nil,
		)

	array := testStableAddressPath(t, arrayPath)
	uses := facts.AppendElementFieldOriginUses(AppendElementFieldSourceQuery{
		Array: array,
		Field: field,
	})
	if len(uses) != 1 {
		t.Fatalf("uses got %d, want one source", len(uses))
	}
	got, ok := uses[0].SourcePath()
	if !ok || !got.Equal(sourcePath) {
		t.Fatalf("source path = %v/%v, want op", got, ok)
	}
}

func TestAppendElementFieldSourceAddressesIgnoreLegacyStoredSource(t *testing.T) {
	arrayPath := constraint.NewPath(cfg.SymbolID(130), "routes")
	sourcePath := constraint.NewPath(cfg.SymbolID(131), "op")
	array := testStableAddressPath(t, arrayPath)
	field := []constraint.Segment{{Kind: constraint.SegmentField, Name: "target_name"}}
	facts := KeyPresenceFacts{}.
		WithAppendHistoryBaseAddress(array).
		WithAppendElementFieldOriginFromSource(
			array.Key(),
			AppendElementFieldPathKey(field),
			sourcePath.Key(),
			"",
		)
	raw := facts.AppendElementFieldOriginUses(AppendElementFieldSourceQuery{
		Array: array,
		Field: field,
	})
	if len(raw) != 1 || raw[0].Origin.Source != sourcePath.Key() {
		t.Fatalf("test setup did not keep legacy stored source key: %s", facts.Format())
	}

	if got := facts.AppendElementFieldSourceAddresses(AppendElementFieldSourceQuery{
		Array: array,
		Field: field,
	}); len(got) != 0 {
		t.Fatalf("canonical source view accepted legacy stored source key: %#v", got)
	}
}

func TestKeyPresenceFactsKillSubtreeRemovesDependentFacts(t *testing.T) {
	table := SymbolPathKey(cfg.SymbolID(1), nil)
	key := SymbolPathKey(cfg.SymbolID(2), nil)
	value := SymbolPathKey(cfg.SymbolID(3), nil)
	other := SymbolPathKey(cfg.SymbolID(4), nil)
	array := SymbolPathKey(cfg.SymbolID(5), nil)

	facts := KeyPresenceFacts{}.
		With(table, key).
		WithValue(table, key, value).
		With(table, other).
		WithKeyArray(array, table).
		WithEmptyKeyArray(array).
		WithPendingKeyArray(array, table, key)

	killedValue := facts.KillSubtreeAddress(testStableAddressKey(t, value))
	if killedValue.HasValue(table, key, value) {
		t.Fatal("value assignment kept stale table/key/value self-origin fact")
	}
	if !killedValue.Has(table, key) {
		t.Fatal("value assignment should not kill independent table/key presence")
	}

	killedKey := facts.KillSubtreeAddress(testStableAddressKey(t, key))
	if killedKey.Has(table, key) || killedKey.HasValue(table, key, value) {
		t.Fatal("key assignment kept stale key-presence facts")
	}
	if !killedKey.Has(table, other) {
		t.Fatal("key assignment killed unrelated key-presence fact")
	}

	killedTable := facts.KillSubtreeAddress(testStableAddressKey(t, table))
	if len(killedTable.Entries()) != 0 || len(killedTable.ValueEntries()) != 0 || len(killedTable.KeyArrayEntries()) != 0 || len(killedTable.PendingKeyArrayEntries()) != 0 {
		t.Fatalf("table assignment left stale facts: %s", killedTable.Format())
	}
	if !killedTable.HasEmptyKeyArray(array) {
		t.Fatalf("table assignment killed independent empty-array fact: %s", killedTable.Format())
	}

	killedArray := facts.KillSubtreeAddress(testStableAddressKey(t, array))
	if len(killedArray.KeyArrayEntries()) != 0 || len(killedArray.PendingKeyArrayEntries()) != 0 || killedArray.HasEmptyKeyArray(array) {
		t.Fatalf("array assignment left stale key-array facts: %s", killedArray.Format())
	}
	if !killedArray.Has(table, key) {
		t.Fatalf("array assignment killed unrelated table/key fact: %s", killedArray.Format())
	}
}

func TestKeyPresenceFactsKillAffectedByWriteDropsOverlappingTableFacts(t *testing.T) {
	table := SymbolPathKey(cfg.SymbolID(1), nil)
	tableMember := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "x"},
	})
	key := SymbolPathKey(cfg.SymbolID(2), nil)
	value := SymbolPathKey(cfg.SymbolID(3), nil)
	otherTable := SymbolPathKey(cfg.SymbolID(4), nil)
	array := SymbolPathKey(cfg.SymbolID(5), nil)

	facts := KeyPresenceFacts{}.
		With(table, key).
		WithValue(table, key, value).
		With(otherTable, key).
		WithKeyArray(array, table).
		WithEmptyKeyArray(array).
		WithPendingKeyArray(array, table, key)

	killed := facts.KillAffectedByWriteAddress(testStableAddressKey(t, tableMember))
	if killed.Has(table, key) || killed.HasValue(table, key, value) || len(killed.KeyArrayTables(array)) != 0 || len(killed.PendingKeyArrayEntries()) != 0 {
		t.Fatalf("member write kept stale table-root facts: %s", killed.Format())
	}
	if !killed.HasEmptyKeyArray(array) {
		t.Fatalf("table member write killed independent empty-array fact: %s", killed.Format())
	}
	if !killed.Has(otherTable, key) {
		t.Fatalf("member write killed unrelated table fact: %s", killed.Format())
	}
}

func TestKeyPresenceFactsAddressInvalidationSupportsNamedRoots(t *testing.T) {
	table, _ := StableAddressOfRoot("$0", nil)
	tableMember, _ := StableAddressOfRoot("$0", []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}})
	key, _ := StableAddressOfRoot("$1", nil)
	otherTable, _ := StableAddressOfRoot("$2", nil)
	array, _ := StableAddressOfRoot("$3", nil)

	facts := KeyPresenceFacts{}.
		With(table.Key(), key.Key()).
		With(otherTable.Key(), key.Key()).
		WithKeyArray(array.Key(), table.Key()).
		WithEmptyKeyArray(array.Key())

	killed := facts.KillAffectedByWriteAddress(tableMember)
	if killed.Has(table.Key(), key.Key()) || len(killed.KeyArrayTables(array.Key())) != 0 {
		t.Fatalf("address member write kept stale table facts: %s", killed.Format())
	}
	if !killed.Has(otherTable.Key(), key.Key()) {
		t.Fatalf("address member write killed unrelated table fact: %s", killed.Format())
	}
	if !killed.HasEmptyKeyArray(array.Key()) {
		t.Fatalf("address member write killed independent empty-array fact: %s", killed.Format())
	}
}

func TestKeyPresenceAddressFilterCachesPredicateByStoredKey(t *testing.T) {
	table, _ := StableAddressOfSymbol(cfg.SymbolID(80), nil)
	key, _ := StableAddressOfSymbol(cfg.SymbolID(81), nil)
	value, _ := StableAddressOfSymbol(cfg.SymbolID(82), nil)
	array, _ := StableAddressOfSymbol(cfg.SymbolID(83), nil)
	calls := map[constraint.PathKey]int{}

	facts := KeyPresenceFacts{}.
		WithAddresses(table, key).
		WithValueAddresses(table, key, value).
		WithKeyArrayAddresses(array, table).
		WithPendingKeyArrayAddresses(array, table, true, key)

	_ = facts.factSet().filter(keyPresenceFullAddressFilter(func(addr StableAddress) bool {
		calls[addr.Key()]++
		return false
	}))

	for _, addr := range []StableAddress{table, key, value, array} {
		if got := calls[addr.Key()]; got != 1 {
			t.Fatalf("predicate calls for %s = %d, want one", addr.Key(), got)
		}
	}
}

func TestKeyPresenceFactsPresentElementWriteKeepsKeyPresenceButDropsValueFacts(t *testing.T) {
	table := SymbolPathKey(cfg.SymbolID(1), nil)
	tableMember := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "x"},
	})
	key := SymbolPathKey(cfg.SymbolID(2), nil)
	value := SymbolPathKey(cfg.SymbolID(3), nil)
	array := SymbolPathKey(cfg.SymbolID(4), nil)

	facts := KeyPresenceFacts{}.
		With(table, key).
		WithValue(table, key, value).
		WithKeyArray(array, table).
		WithKeyArrayValue(array, table, product.FromType(typ.String)).
		WithEmptyKeyArray(array).
		WithPendingKeyArray(array, table, key)

	killed := facts.KillAffectedByPresentElementWriteAddress(testStableAddressKey(t, tableMember))
	if !killed.Has(table, key) {
		t.Fatalf("present element write dropped key-presence fact: %s", killed.Format())
	}
	if killed.HasValue(table, key, value) {
		t.Fatalf("present element write kept stale value-specific fact: %s", killed.Format())
	}
	if len(killed.KeyArrayTables(array)) != 1 {
		t.Fatalf("present element write dropped independent key-array fact: %s", killed.Format())
	}
	values := killed.KeyArrayValues(array, table)
	if len(values) != 1 || !product.Domain.Equal(values[0], product.FromType(typ.String)) {
		t.Fatalf("present element write dropped key-array value before write proof: %s", killed.Format())
	}
	if len(killed.PendingKeyArrayEntries()) != 1 {
		t.Fatalf("present element write dropped pending key-array fact: %s", killed.Format())
	}
	if !killed.HasEmptyKeyArray(array) {
		t.Fatalf("present table element write killed independent empty-array fact: %s", killed.Format())
	}
}

func TestKeyPresenceFactsPresentElementAddressWriteKeepsPresence(t *testing.T) {
	table, _ := StableAddressOfRoot("$0", nil)
	tableMember, _ := StableAddressOfRoot("$0", []constraint.Segment{{Kind: constraint.SegmentField, Name: "x"}})
	key, _ := StableAddressOfRoot("$1", nil)
	value, _ := StableAddressOfRoot("$2", nil)

	facts := KeyPresenceFacts{}.
		With(table.Key(), key.Key()).
		WithValue(table.Key(), key.Key(), value.Key())

	killed := facts.KillAffectedByPresentElementWriteAddress(tableMember)
	if !killed.Has(table.Key(), key.Key()) {
		t.Fatalf("present element address write dropped key-presence fact: %s", killed.Format())
	}
	if killed.HasValue(table.Key(), key.Key(), value.Key()) {
		t.Fatalf("present element address write kept stale value fact: %s", killed.Format())
	}
}

func TestKeyPresenceFactsAddressSubtreeKillUsesPrefixOnly(t *testing.T) {
	root, _ := StableAddressOfSymbol(cfg.SymbolID(40), nil)
	child, _ := StableAddressOfSymbol(cfg.SymbolID(40), []constraint.Segment{{Kind: constraint.SegmentField, Name: "child"}})
	sibling, _ := StableAddressOfSymbol(cfg.SymbolID(41), nil)
	key, _ := StableAddressOfSymbol(cfg.SymbolID(42), nil)

	facts := KeyPresenceFacts{}.
		With(child.Key(), key.Key()).
		With(sibling.Key(), key.Key())

	killed := facts.KillSubtreeAddress(root)
	if killed.Has(child.Key(), key.Key()) {
		t.Fatalf("subtree kill kept child fact: %s", killed.Format())
	}
	if !killed.Has(sibling.Key(), key.Key()) {
		t.Fatalf("subtree kill removed sibling root fact: %s", killed.Format())
	}
}

func TestKeyPresenceFactsAddressInvalidationIgnoresLegacyStoredKeys(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(43), "table")
	member := table.Field("child")
	key := constraint.NewPath(cfg.SymbolID(44), "key")
	value := constraint.NewPath(cfg.SymbolID(45), "value")
	array := constraint.NewPath(cfg.SymbolID(46), "array")
	facts := KeyPresenceFacts{}.
		With(table.Key(), key.Key()).
		WithValue(table.Key(), key.Key(), value.Key()).
		WithKeyArray(array.Key(), table.Key()).
		WithEmptyKeyArray(array.Key()).
		WithPendingKeyArray(array.Key(), table.Key(), key.Key())

	killed := facts.KillAffectedByWriteAddress(testStableAddressPath(t, member))
	if !KeyPresenceFactsDomain.Equal(killed, facts) {
		t.Fatalf("write invalidation treated legacy stored keys as canonical: got %s want %s", killed.Format(), facts.Format())
	}

	killed = facts.KillSubtreeAddress(testStableAddressPath(t, table))
	if !KeyPresenceFactsDomain.Equal(killed, facts) {
		t.Fatalf("subtree invalidation treated legacy stored keys as canonical: got %s want %s", killed.Format(), facts.Format())
	}
}

func TestKeyPresenceFactsPresentElementInvalidationIgnoresLegacyStoredKeys(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(47), "table")
	member := table.Field("child")
	key := constraint.NewPath(cfg.SymbolID(48), "key")
	value := constraint.NewPath(cfg.SymbolID(49), "value")
	array := constraint.NewPath(cfg.SymbolID(50), "array")
	facts := KeyPresenceFacts{}.
		With(table.Key(), key.Key()).
		WithValue(table.Key(), key.Key(), value.Key()).
		WithKeyArray(array.Key(), table.Key()).
		WithKeyArrayValue(array.Key(), table.Key(), product.FromType(typ.String)).
		WithPendingKeyArray(array.Key(), table.Key(), key.Key())

	killed := facts.KillAffectedByPresentElementWriteAddress(testStableAddressPath(t, member))
	if !KeyPresenceFactsDomain.Equal(killed, facts) {
		t.Fatalf("present-element invalidation treated legacy stored keys as canonical: got %s want %s", killed.Format(), facts.Format())
	}
}

func TestKeyPresenceFactsKillAffectedByWriteDropsArrayMemberFacts(t *testing.T) {
	array := SymbolPathKey(cfg.SymbolID(1), nil)
	arrayMember := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{
		{Kind: constraint.SegmentIndexInt, Index: 1},
	})
	table := SymbolPathKey(cfg.SymbolID(2), nil)
	otherArray := SymbolPathKey(cfg.SymbolID(3), nil)

	facts := KeyPresenceFacts{}.
		WithKeyArray(array, table).
		WithEmptyKeyArray(array).
		WithKeyArray(otherArray, table)

	killed := facts.KillAffectedByWriteAddress(testStableAddressKey(t, arrayMember))
	if len(killed.KeyArrayTables(array)) != 0 {
		t.Fatalf("array member write kept stale key-array fact: %s", killed.Format())
	}
	if killed.HasEmptyKeyArray(array) {
		t.Fatalf("array member write kept stale empty-array fact: %s", killed.Format())
	}
	if len(killed.KeyArrayTables(otherArray)) != 1 {
		t.Fatalf("array member write killed unrelated key-array fact: %s", killed.Format())
	}
}

func TestStablePathKeyIgnoresVersion(t *testing.T) {
	a := constraint.NewPath(cfg.SymbolID(1), "m").Field("items")
	a.Version = 3
	b := constraint.NewPath(cfg.SymbolID(1), "m").Field("items")
	b.Version = 9
	if got, want := StablePathKey(a), StablePathKey(b); got != want {
		t.Fatalf("StablePathKey version-sensitive: got %s, want %s", got, want)
	}
}

func keyPresenceFactsSample() []KeyPresenceFacts {
	tableA := SymbolPathKey(cfg.SymbolID(1), nil)
	tableB := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentField, Name: "items"}})
	keyA := SymbolPathKey(cfg.SymbolID(2), nil)
	keyB := SymbolPathKey(cfg.SymbolID(3), []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "id"}})

	return []KeyPresenceFacts{
		KeyPresenceFactsDomain.Bottom(),
		KeyPresenceFactsDomain.Top(),
		KeyPresenceFactsOf([]KeyPresenceFact{{Table: tableA, Key: keyA}}),
		KeyPresenceFactsOf([]KeyPresenceFact{{Table: tableA, Key: keyB}}),
		KeyPresenceFactsOf([]KeyPresenceFact{{Table: tableB, Key: keyA}}),
		KeyPresenceFactsOf([]KeyPresenceFact{
			{Table: tableA, Key: keyA},
			{Table: tableB, Key: keyB},
		}).WithValue(tableA, keyA, keyB).WithKeyArray(keyA, tableA),
		KeyPresenceFacts{}.WithEmptyKeyArray(keyA),
		KeyPresenceFacts{}.WithPendingKeyArray(keyA, tableA, keyB),
		KeyPresenceFacts{}.WithPendingKeyArray(keyA, "", keyB),
	}
}
