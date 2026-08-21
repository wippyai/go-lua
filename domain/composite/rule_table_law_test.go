package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// TestRuleTableSeals is the table's own totality law: the declaration root
// admits the analyzer rule surface, and the surface's coverage, ordering, and
// identity laws all hold on the production inventory.
func TestRuleTableSeals(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	table, failure := Table(compilation)
	if failure.Available() {
		t.Fatalf("rule table rejected: contributor=%d entry=%x law=%d disposition=%s", failure.Contributor, failure.Entry, failure.Law, failure.Disposition)
	}
	if table == nil || !table.Available() {
		t.Fatal("rule table sealed without an available schema")
	}
	if !table.Digest().Available() {
		t.Fatal("sealed table published no digest")
	}
	view, viewOK := table.Surface(schema.SurfaceKindRule)
	if !viewOK || view.Count() != RuleCount(compilation) {
		t.Fatalf("rule surface view = %d entries, table = %d", view.Count(), RuleCount(compilation))
	}
}

// TestRuleTableCoversEveryDeclarationKeyExactlyOnce states that each table
// position publishes one unique declaration key.
func TestRuleTableCoversEveryDeclarationKeyExactlyOnce(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	seen := make(map[schema.Key]int, RuleCount(compilation))
	mounted := 0
	for position := 0; position < RuleCount(compilation); position++ {
		key, ok := RuleKeyAt(compilation, position)
		if !ok || !key.Available() {
			t.Fatalf("table position %d has no key", position)
		}
		seen[key]++
		if seen[key] != 1 {
			t.Fatalf("key %q declared %d times", key, seen[key])
		}
		if MountedRuleKey(compilation, key) {
			mounted++
		}
	}
	if RuleCount(compilation) != mounted+len(LinkKeys(compilation)) {
		t.Fatalf("table = %d rules, mounted = %d, link = %d", RuleCount(compilation), mounted, len(LinkKeys(compilation)))
	}
}

// TestRuleTableDrivesEveryDerivedView is the drift law. Every projection the
// analyzer consumes - identity, semantic key, diagnostic classification, lane
// membership, the write axis, and the owner that must supply the operand
// resolver - is computed from the table entry, so a rule that reaches the
// table is wired everywhere and a name that is not in the table is classified
// nowhere.
func TestRuleTableDrivesEveryDerivedView(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	state := compilation.catalog
	links := make(map[schema.Key]bool, len(LinkKeys(compilation)))
	for _, key := range LinkKeys(compilation) {
		links[key] = true
	}
	identities := make(map[schema.EntryID]schema.Key, RuleCount(compilation))
	for position := 0; position < RuleCount(compilation); position++ {
		key, keyOK := RuleKeyAt(compilation, position)
		if !keyOK {
			t.Fatalf("table position %d has no key", position)
		}
		id, idOK := RuleEntryID(compilation, key)
		if !idOK || !id.Available() {
			t.Fatalf("key %q has no stable table identity", key)
		}
		if prior, duplicate := identities[id]; duplicate {
			t.Fatalf("keys %q and %q share one table identity", prior, key)
		}
		identities[id] = key

		semantic, semanticOK := RuleSemantic(compilation, key)
		if !semanticOK {
			t.Fatalf("key %q has no semantic identity", key)
		}
		diagnostic := DiagnosticRuleForKey(compilation, key)
		if diagnostic == DiagnosticRuleUnknown {
			t.Fatalf("key %q has no diagnostic classification", key)
		}
		if got := DiagnosticRuleForSemantic(compilation, semantic); got != diagnostic {
			t.Fatalf("semantic classification of %q = %d, want %d", key, got, diagnostic)
		}
		if diagnostic.String() == "unknown" || diagnostic.String() == "" {
			t.Fatalf("key %q has no diagnostic name", key)
		}
		entry, entryOK := templateForKey(state, key)
		if !entryOK {
			t.Fatalf("key %q is not resolvable", key)
		}
		if string(entry.Key()) != diagnostic.String() {
			t.Fatalf("key %q is spelled %q by its diagnostic", key, diagnostic.String())
		}
		if _, declared := axisForKey(state, entry.Writes()); !declared {
			t.Fatalf("key %q writes %q, which no axis declares", key, entry.Writes())
		}
		owner, ownerOK := RuleOwner(compilation, key)
		if !ownerOK || owner != entry.Owner() {
			t.Fatalf("key %q has no owner projection", key)
		}
		if _, declared := axisForKey(state, owner); !declared {
			t.Fatalf("key %q owner %q names no declared axis", key, owner)
		}
		if links[key] != (entry.Lane() == rule.LaneLink) {
			t.Fatalf("key %q lane membership disagrees with the link projection", key)
		}
		if entry.Lane().Mounted() != !links[key] {
			t.Fatalf("key %q is neither mounted nor link owned", key)
		}
	}
	if len(identities) != RuleCount(compilation) {
		t.Fatalf("table identities = %d, rule count = %d", len(identities), RuleCount(compilation))
	}
	foreign, foreignOK := vocabulary.Key("rule/absent-from-the-table")
	if !foreignOK {
		t.Fatal("foreign key")
	}
	if DiagnosticRuleForSemantic(compilation, foreign) != DiagnosticRuleUnknown {
		t.Fatal("a key outside the table was classified")
	}
	if DiagnosticRuleForKey(compilation, "") != DiagnosticRuleUnknown {
		t.Fatal("the empty key was classified")
	}
}

// TestRuleTableOwnerNamesABoundPrincipal is the schema join of the operand
// resolver slot: every declared rule names exactly one owner, that owner is a
// bound axis, and two rules may share an owner but one rule never names two.
func TestRuleTableOwnerNamesABoundPrincipal(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	state := compilation.catalog
	seen := make(map[schema.Key]schema.Key, RuleCount(compilation))
	for position := 0; position < RuleCount(compilation); position++ {
		key, keyOK := RuleKeyAt(compilation, position)
		if !keyOK {
			t.Fatalf("table position %d has no key", position)
		}
		owner, ownerOK := RuleOwner(compilation, key)
		if !ownerOK {
			t.Fatalf("key %q declares no owner", key)
		}
		entry, entryOK := axisForKey(state, owner)
		if !entryOK {
			t.Fatalf("key %q owner %q is not a declared axis", key, owner)
		}
		if !entry.Storage().Bound() {
			t.Fatalf("key %q owner %q is not a bound writer principal", key, owner)
		}
		if prior, duplicate := seen[key]; duplicate {
			t.Fatalf("key %q names owners %q and %q", key, prior, owner)
		}
		seen[key] = owner
	}
	if len(seen) != RuleCount(compilation) {
		t.Fatalf("owners = %d, rule count = %d", len(seen), RuleCount(compilation))
	}
	if owner, ok := RuleOwner(compilation, "no-such-rule"); ok || owner.Available() {
		t.Fatal("unknown key resolved an owner")
	}
}

// TestRuleKeyResolvesTheDeclaredSlot states that a rule's authored key names
// its declaration position.
func TestRuleKeyResolvesTheDeclaredSlot(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	for position := 0; position < RuleCount(compilation); position++ {
		key, keyOK := RuleKeyAt(compilation, position)
		if !keyOK {
			t.Fatalf("table position %d has no key", position)
		}
		if DiagnosticRuleForKey(compilation, key) != DiagnosticRule(position+1) {
			t.Fatalf("key %q classifies away from position %d", key, position)
		}
		slot, slotOK := ruleSlotForKey(compilation.catalog, key)
		if !slotOK || slot != position+1 {
			t.Fatalf("key %q resolves slot %d, table position %d", key, slot, position+1)
		}
	}
	if DiagnosticRuleForKey(compilation, "no-such-rule") != DiagnosticRuleUnknown {
		t.Fatal("unknown key classified a rule")
	}
}

// TestLinkKeysNameTheLinkLane states that LinkKeys is the declaration-key
// projection of the Link lane, in table order.
func TestLinkKeysNameTheLinkLane(t *testing.T) {
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	state := compilation.catalog
	keys := LinkKeys(compilation)
	if len(keys) == 0 {
		t.Fatal("link keys are empty")
	}
	for index, key := range keys {
		if !key.Available() {
			t.Fatalf("link key %d is empty", index)
		}
		if DiagnosticRuleForKey(compilation, key) == DiagnosticRuleUnknown {
			t.Fatalf("link key %q has no diagnostic classification", key)
		}
		if !MountedRuleKey(compilation, key) {
			if _, ok := templateForKey(state, key); !ok {
				t.Fatalf("link key %q is not a table rule", key)
			}
		}
		entry, ok := templateForKey(state, key)
		if !ok || entry.Lane() != rule.LaneLink {
			t.Fatalf("link key %q is not on the Link lane", key)
		}
	}
	if MountedRuleKey(compilation, "value-bootstrap") || MountedRuleKey(compilation, "heap-bootstrap") {
		t.Fatal("bootstrap keys classified as mounted")
	}
	if !MountedRuleKey(compilation, "effect-selected") || !MountedRuleKey(compilation, "value-source") {
		t.Fatal("mounted keys were not classified as mounted")
	}
	if MountedRuleKey(compilation, "no-such-rule") {
		t.Fatal("unknown key classified as mounted")
	}
}

// TestCatalogDeclaresEveryTableRule is the cold-side drift law: the sealed
// catalog's fragment inventory is exactly the table's, so a rule reaching the
// table is declared into the schema without a second hand-kept sequence.
func TestCatalogDeclaresEveryTableRule(t *testing.T) {
	receipt, ok := Build()
	if !ok || !receipt.Available() {
		t.Fatal("global catalog")
	}
	for position := 0; position < RuleCount(receipt); position++ {
		slot := position + 1
		if !receipt.catalog.ruleFragments[slot].Available() {
			t.Fatalf("slot %d declared no cold fragment", slot)
		}
	}
	for slot := 0; slot < len(receipt.catalog.ruleFragments); slot++ {
		_, known := templateAtSlot(receipt.catalog, slot)
		if receipt.catalog.ruleFragments[slot].Available() != known {
			t.Fatalf("slot %d has a fragment the table does not declare", slot)
		}
	}
}

// TestRuleBindingPublishesOneProgramRulePerOperandRule is the construction
// join: every mounted operand and Link rule publishes exactly one cell-owned
// construction primitive, and the activation lane publishes none.
func TestRuleBindingPublishesOneProgramRulePerOperandRule(t *testing.T) {
	bound := materializerBinding(t, mountedRecord(t, "program-rule", materializerSource))
	compilation := bound.Compilation()
	state := compilation.catalog
	rules := bound.Rules()
	if rules == nil {
		t.Fatal("bound rules")
	}
	published := 0
	for position := 0; position < RuleCount(compilation); position++ {
		key, keyOK := RuleKeyAt(compilation, position)
		entry, entryOK := templateForKey(state, key)
		if !keyOK || !entryOK {
			t.Fatalf("table position %d has no declaration", position)
		}
		program, programOK := rules.ProgramRuleByKey(key)
		if entry.Lane() == rule.LaneActivation {
			if programOK || program.Available() {
				t.Fatalf("activation %q published a construction primitive", key)
			}
			continue
		}
		if !programOK || !program.Available() {
			t.Fatalf("key %q has no construction primitive", key)
		}
		published++
	}
	if published == 0 {
		t.Fatal("no operand rule published a construction primitive")
	}
	if program, ok := rules.ProgramRuleByKey("no-such-rule"); ok || program.Available() {
		t.Fatal("unknown key published a construction primitive")
	}
}
