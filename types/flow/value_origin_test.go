package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
)

func TestValueOriginFactsDomainLaws(t *testing.T) {
	entry := constraint.NewPath(cfg.SymbolID(1), "entry")
	entryMeta := entry.Field("meta")
	tests := constraint.NewPath(cfg.SymbolID(2), "tests")
	items := constraint.NewPath(cfg.SymbolID(3), "items")
	name := constraint.NewPath(cfg.SymbolID(4), "name")
	value := constraint.NewPath(cfg.SymbolID(5), "value")

	indexedEntry := ValueOriginFacts{}.WithAddresses(testStableAddressPath(t, entry), testStableAddressPath(t, tests), ValueOriginIndexedIterator, 1)
	indexedMeta := indexedEntry.WithAddresses(testStableAddressPath(t, entryMeta), testStableAddressPath(t, items), ValueOriginIndexedIterator, 1)
	keyedKey := ValueOriginFacts{}.WithAddresses(testStableAddressPath(t, name), testStableAddressPath(t, items), ValueOriginKeyedIterator, 0)
	keyedValue := keyedKey.WithAddresses(testStableAddressPath(t, value), testStableAddressPath(t, items), ValueOriginKeyedIterator, 1)
	assignmentAlias := ValueOriginFacts{}.WithAddresses(testStableAddressPath(t, name), testStableAddressPath(t, entry), ValueOriginAssignmentAlias, 0)

	lattice.LawSuite[ValueOriginFacts]{
		Name:   "ValueOriginFacts",
		Domain: ValueOriginFactsDomain,
		Sample: []ValueOriginFacts{
			ValueOriginFactsDomain.Bottom(),
			ValueOriginFactsDomain.Top(),
			indexedEntry,
			indexedMeta,
			keyedKey,
			keyedValue,
			assignmentAlias,
			ValueOriginFactsDomain.Join(indexedMeta, keyedValue),
		},
		Format: func(f ValueOriginFacts) string { return f.Format() },
	}.Run(t)
}

func TestValueOriginFactsOriginsCoveringPath(t *testing.T) {
	entry := constraint.NewPath(cfg.SymbolID(11), "entry")
	tests := constraint.NewPath(cfg.SymbolID(12), "tests")
	facts := ValueOriginFacts{}.WithAddresses(testStableAddressPath(t, entry), testStableAddressPath(t, tests), ValueOriginIndexedIterator, 1)

	uses := facts.OriginsCoveringAddress(testStableAddressPath(t, entry.Field("id")))
	if len(uses) != 1 {
		t.Fatalf("OriginsCoveringPath(entry.id) got %d uses, want 1: %s", len(uses), facts.Format())
	}
	if uses[0].Origin.Kind != ValueOriginIndexedIterator || uses[0].Origin.VarIndex != 1 {
		t.Fatalf("origin = %#v, want indexed value origin", uses[0].Origin)
	}
	if len(uses[0].Remainder) != 1 || uses[0].Remainder[0].Name != "id" {
		t.Fatalf("remainder = %#v, want [.id]", uses[0].Remainder)
	}
	sourcePath, ok := uses[0].Origin.SourcePath()
	if !ok || !sourcePath.Equal(tests) {
		t.Fatalf("source path = %v/%v, want tests", sourcePath, ok)
	}
}

func TestValueOriginFactsOriginsCoveringPathKeepsAllPrefixOrigins(t *testing.T) {
	entry := constraint.NewPath(cfg.SymbolID(14), "entry")
	entryMeta := entry.Field("meta")
	tests := constraint.NewPath(cfg.SymbolID(15), "tests")
	metadata := constraint.NewPath(cfg.SymbolID(16), "metadata")
	facts := ValueOriginFacts{}.
		WithAddresses(testStableAddressPath(t, entry), testStableAddressPath(t, tests), ValueOriginIndexedIterator, 1).
		WithAddresses(testStableAddressPath(t, entryMeta), testStableAddressPath(t, metadata), ValueOriginIndexedIterator, 1)

	uses := facts.OriginsCoveringAddress(testStableAddressPath(t, entryMeta.Field("id")))
	if len(uses) != 2 {
		t.Fatalf("OriginsCoveringPath(entry.meta.id) got %d uses, want both covering origins: %s", len(uses), facts.Format())
	}
	if len(uses[0].Remainder) != 1 || uses[0].Remainder[0].Name != "id" {
		t.Fatalf("first origin should be most-specific entry.meta remainder [.id], got %#v", uses[0].Remainder)
	}
	if len(uses[1].Remainder) != 2 || uses[1].Remainder[0].Name != "meta" || uses[1].Remainder[1].Name != "id" {
		t.Fatalf("second origin should be entry remainder [.meta, .id], got %#v", uses[1].Remainder)
	}
}

func TestValueOriginFactsAddressCoverageSupportsNamedRoots(t *testing.T) {
	value, _ := StableAddressOfRoot("$0", nil)
	source, _ := StableAddressOfRoot("$1", nil)
	child, _ := StableAddressOfRoot("$0", []constraint.Segment{{Kind: constraint.SegmentField, Name: "id"}})

	facts := ValueOriginFacts{}.WithAddresses(value, source, ValueOriginAssignmentAlias, 0)
	uses := facts.OriginsCoveringAddress(child)
	if len(uses) != 1 {
		t.Fatalf("OriginsCoveringAddress($0.id) got %d uses, want 1: %s", len(uses), facts.Format())
	}
	if uses[0].Origin.Source != source.Key() {
		t.Fatalf("source = %s, want %s", uses[0].Origin.Source, source.Key())
	}
	if len(uses[0].Remainder) != 1 || uses[0].Remainder[0].Name != "id" {
		t.Fatalf("remainder = %#v, want [.id]", uses[0].Remainder)
	}
}

func TestValueOriginAssignmentAliasSourceAddressesIgnoreLegacyStoredSource(t *testing.T) {
	valuePath := constraint.NewPath(cfg.SymbolID(17), "alias")
	sourcePath := constraint.NewPath(cfg.SymbolID(18), "source")
	value := testStableAddressPath(t, valuePath)
	facts := ValueOriginFacts{}.With(ValueOriginFact{
		Value:  value.Key(),
		Source: sourcePath.Key(),
		Kind:   ValueOriginAssignmentAlias,
	})
	raw := facts.OriginsCoveringAddress(value)
	if len(raw) != 1 || raw[0].Origin.Source != sourcePath.Key() {
		t.Fatalf("test setup did not keep legacy stored source key: %s", facts.Format())
	}

	if got := facts.assignmentAliasSourceRoutesCoveringAddress(value); len(got) != 0 {
		t.Fatalf("canonical assignment-alias view accepted legacy source key: %#v", got)
	}
}

func TestValueOriginFactsOriginsCoveringAddressIgnoresLegacyStoredValue(t *testing.T) {
	valuePath := constraint.NewPath(cfg.SymbolID(19), "entry")
	sourcePath := constraint.NewPath(cfg.SymbolID(20), "items")
	value := testStableAddressPath(t, valuePath)
	facts := ValueOriginFacts{}.With(ValueOriginFact{
		Value:    valuePath.Key(),
		Source:   testStableAddressPath(t, sourcePath).Key(),
		Kind:     ValueOriginIndexedIterator,
		VarIndex: 1,
	})
	if entries := facts.Entries(); len(entries) != 1 || entries[0].Value != valuePath.Key() {
		t.Fatalf("test setup did not keep legacy stored value key: %s", facts.Format())
	}

	if got := facts.OriginsCoveringAddress(value); len(got) != 0 {
		t.Fatalf("legacy stored value key produced origin uses: %#v", got)
	}
}

func TestValueOriginFactsJoinKeepsMustFacts(t *testing.T) {
	entry := constraint.NewPath(cfg.SymbolID(21), "entry")
	tests := constraint.NewPath(cfg.SymbolID(22), "tests")
	other := constraint.NewPath(cfg.SymbolID(23), "other")
	common := ValueOriginFact{
		Value:    StablePathKey(entry),
		Source:   StablePathKey(tests),
		Kind:     ValueOriginIndexedIterator,
		VarIndex: 1,
	}
	extra := ValueOriginFact{
		Value:    StablePathKey(other),
		Source:   StablePathKey(tests),
		Kind:     ValueOriginIndexedIterator,
		VarIndex: 1,
	}
	left := ValueOriginFacts{}.With(common).With(extra)
	right := ValueOriginFacts{}.With(common)

	got := ValueOriginFactsDomain.Join(left, right)
	if !ValueOriginFactsDomain.Equal(got, right) {
		t.Fatalf("Join kept non-must origin: got %s want %s", got.Format(), right.Format())
	}
}

func TestValueOriginFactsCanonicalizationIsDeterministic(t *testing.T) {
	entry := constraint.NewPath(cfg.SymbolID(24), "entry")
	tests := constraint.NewPath(cfg.SymbolID(25), "tests")
	name := constraint.NewPath(cfg.SymbolID(26), "name")
	items := constraint.NewPath(cfg.SymbolID(27), "items")
	a := ValueOriginFact{
		Value:    StablePathKey(entry),
		Source:   StablePathKey(tests),
		Kind:     ValueOriginIndexedIterator,
		VarIndex: 1,
	}
	b := ValueOriginFact{
		Value:    StablePathKey(name),
		Source:   StablePathKey(items),
		Kind:     ValueOriginKeyedIterator,
		VarIndex: 0,
	}

	left := ValueOriginFacts{}.With(a).With(b).With(a)
	right := ValueOriginFacts{}.With(b).With(a)
	if !ValueOriginFactsDomain.Equal(left, right) {
		t.Fatalf("canonicalization depends on insertion order: left=%s right=%s", left.Format(), right.Format())
	}
	if left.Format() != right.Format() {
		t.Fatalf("format should be deterministic: left=%s right=%s", left.Format(), right.Format())
	}
}

func TestValueOriginFactsKillAffectedByWrite(t *testing.T) {
	entry := constraint.NewPath(cfg.SymbolID(31), "entry")
	tests := constraint.NewPath(cfg.SymbolID(32), "tests")
	facts := ValueOriginFacts{}.WithAddresses(testStableAddressPath(t, entry), testStableAddressPath(t, tests), ValueOriginIndexedIterator, 1)

	got := facts.KillAffectedByWriteAddress(testStableAddressPath(t, tests.Field("items")))
	if len(got.Entries()) != 0 {
		t.Fatalf("source subtree write kept origin: %s", got.Format())
	}

	facts = ValueOriginFacts{}.WithAddresses(testStableAddressPath(t, entry), testStableAddressPath(t, tests), ValueOriginIndexedIterator, 1)
	got = facts.KillAffectedByWriteAddress(testStableAddressPath(t, entry.Field("id")))
	if len(got.Entries()) != 0 {
		t.Fatalf("derived-value subtree write kept origin: %s", got.Format())
	}
}

func TestValueOriginFactsKillAffectedByWritePreservesSiblings(t *testing.T) {
	entryID := constraint.NewPath(cfg.SymbolID(33), "entry").Field("id")
	entryName := constraint.NewPath(cfg.SymbolID(33), "entry").Field("name")
	tests := constraint.NewPath(cfg.SymbolID(34), "tests")
	facts := ValueOriginFacts{}.WithAddresses(testStableAddressPath(t, entryID), testStableAddressPath(t, tests), ValueOriginIndexedIterator, 1)

	got := facts.KillAffectedByWriteAddress(testStableAddressPath(t, entryName))
	if !ValueOriginFactsDomain.Equal(got, facts) {
		t.Fatalf("sibling write killed unrelated origin: got=%s want=%s", got.Format(), facts.Format())
	}
}
