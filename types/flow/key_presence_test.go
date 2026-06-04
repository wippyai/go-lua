package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
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
		WithKeyArray(array, table)

	killedValue := facts.KillSubtree(value)
	if killedValue.HasValue(table, key, value) {
		t.Fatal("value assignment kept stale table/key/value self-origin fact")
	}
	if !killedValue.Has(table, key) {
		t.Fatal("value assignment should not kill independent table/key presence")
	}

	killedKey := facts.KillSubtree(key)
	if killedKey.Has(table, key) || killedKey.HasValue(table, key, value) {
		t.Fatal("key assignment kept stale key-presence facts")
	}
	if !killedKey.Has(table, other) {
		t.Fatal("key assignment killed unrelated key-presence fact")
	}

	killedTable := facts.KillSubtree(table)
	if len(killedTable.Entries()) != 0 || len(killedTable.ValueEntries()) != 0 || len(killedTable.KeyArrayEntries()) != 0 {
		t.Fatalf("table assignment left stale facts: %s", killedTable.Format())
	}

	killedArray := facts.KillSubtree(array)
	if len(killedArray.KeyArrayEntries()) != 0 {
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
		WithKeyArray(array, table)

	killed := facts.KillAffectedByWrite(tableMember)
	if killed.Has(table, key) || killed.HasValue(table, key, value) || len(killed.KeyArrayTables(array)) != 0 {
		t.Fatalf("member write kept stale table-root facts: %s", killed.Format())
	}
	if !killed.Has(otherTable, key) {
		t.Fatalf("member write killed unrelated table fact: %s", killed.Format())
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
		WithKeyArray(array, table)

	killed := facts.KillAffectedByPresentElementWrite(tableMember)
	if !killed.Has(table, key) {
		t.Fatalf("present element write dropped key-presence fact: %s", killed.Format())
	}
	if killed.HasValue(table, key, value) {
		t.Fatalf("present element write kept stale value-specific fact: %s", killed.Format())
	}
	if len(killed.KeyArrayTables(array)) != 1 {
		t.Fatalf("present element write dropped independent key-array fact: %s", killed.Format())
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
		WithKeyArray(otherArray, table)

	killed := facts.KillAffectedByWrite(arrayMember)
	if len(killed.KeyArrayTables(array)) != 0 {
		t.Fatalf("array member write kept stale key-array fact: %s", killed.Format())
	}
	if len(killed.KeyArrayTables(otherArray)) != 1 {
		t.Fatalf("array member write killed unrelated key-array fact: %s", killed.Format())
	}
}

func TestKeyPresencePathKeyIgnoresVersion(t *testing.T) {
	a := constraint.NewPath(cfg.SymbolID(1), "m").Field("items")
	a.Version = 3
	b := constraint.NewPath(cfg.SymbolID(1), "m").Field("items")
	b.Version = 9
	if got, want := KeyPresencePathKey(a), KeyPresencePathKey(b); got != want {
		t.Fatalf("KeyPresencePathKey version-sensitive: got %s, want %s", got, want)
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
	}
}
