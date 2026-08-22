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
	mountedPoint := len(mountedPointKeys(compilation.catalog))
	if RuleCount(compilation) != mounted+len(LinkKeys(compilation))+mountedPoint {
		t.Fatalf("table = %d rules, mounted = %d, mounted-point = %d, link = %d", RuleCount(compilation), mounted, mountedPoint, len(LinkKeys(compilation)))
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
		mountedPoint := entry.Lane() == rule.LaneMountedPoint
		if mountedPoint != containsSchemaKey(mountedPointKeys(state), key) {
			t.Fatalf("key %q lane membership disagrees with the mounted-point projection", key)
		}
		if entry.Lane().Mounted() != !links[key] && !mountedPoint {
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

func containsSchemaKey(keys []schema.Key, want schema.Key) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
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
	for _, key := range mountedPointKeys(state) {
		entry, ok := templateForKey(state, key)
		if !ok || entry.Lane() != rule.LaneMountedPoint || MountedRuleKey(compilation, key) {
			t.Fatalf("mounted-point key %q lost its lane ownership", key)
		}
		for _, link := range keys {
			if link == key {
				t.Fatalf("mounted-point key %q appeared in LinkKeys", key)
			}
		}
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

// TestRuleBindingPublishesOneCanonicalCellPerRule is the construction join:
// every mounted operand and Link rule publishes exactly one sealed cell, and
// the activation lane publishes its distinct activation capability through
// that same canonical cell directory.
func TestRuleBindingPublishesOneCanonicalCellPerRule(t *testing.T) {
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
		cell, cellOK := rules.cellByKey(key)
		capability, capabilityOK := rules.CapabilityByKey(key)
		if !cellOK || !cell.Available() || !capabilityOK || !capability.Available() {
			t.Fatalf("key %q has no sealed canonical cell/capability", key)
		}
		if entry.Lane() == rule.LaneActivation {
			if !capability.Activation() {
				t.Fatalf("activation %q has no activation capability", key)
			}
			continue
		}
		if capability.Activation() {
			t.Fatalf("ordinary key %q has an activation capability", key)
		}
		published++
	}
	if published == 0 {
		t.Fatal("no ordinary rule published a canonical cell")
	}
	if cell, ok := rules.cellByKey("no-such-rule"); ok || cell.Available() {
		t.Fatal("unknown key published a canonical cell")
	}
}
