package composite

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	callactivation "github.com/wippyai/go-lua/domain/call/activation"
	calldispatch "github.com/wippyai/go-lua/domain/call/dispatch"
	callsite "github.com/wippyai/go-lua/domain/effect/callsite"
	heapclosed "github.com/wippyai/go-lua/domain/heap/allocation/closed"
	heapempty "github.com/wippyai/go-lua/domain/heap/allocation/empty"
	heapingress "github.com/wippyai/go-lua/domain/heap/allocation/ingress"
	heapbootstrap "github.com/wippyai/go-lua/domain/heap/bootstrap"
	heapindex "github.com/wippyai/go-lua/domain/heap/index"
	packsource "github.com/wippyai/go-lua/domain/pack/source"
	valueallocation "github.com/wippyai/go-lua/domain/value/allocation"
	valuearithmetic "github.com/wippyai/go-lua/domain/value/arithmetic"
	valuebootstrap "github.com/wippyai/go-lua/domain/value/bootstrap"
	valueequality "github.com/wippyai/go-lua/domain/value/equality"
	valueorder "github.com/wippyai/go-lua/domain/value/order"
	valuerefinement "github.com/wippyai/go-lua/domain/value/refinement"
	valueruntimekind "github.com/wippyai/go-lua/domain/value/runtimekind"
	valuesource "github.com/wippyai/go-lua/domain/value/source"
	valuetransfer "github.com/wippyai/go-lua/domain/value/transfer"
)

type probeRuleEntry struct {
	name string
	ok   bool
}

func probeRuleEntries() []probeRuleEntry {
	row := func(name string, ok bool) probeRuleEntry { return probeRuleEntry{name: name, ok: ok} }
	_, valueSourceOK := rule.New(valuesource.RuleEntry[principals, authorities]())
	_, packSourceOK := rule.New(packsource.RuleEntry[principals, authorities]())
	_, heapIngressOK := rule.New(heapingress.RuleEntry[principals, authorities]())
	_, valueAllocationOK := rule.New(valueallocation.RuleEntry[principals, authorities]())
	_, heapEmptyOK := rule.New(heapempty.RuleEntry[principals, authorities]())
	_, heapClosedOK := rule.New(heapclosed.RuleEntry[principals, authorities]())
	_, rawGetOK := rule.New(heapindex.RawGetEntry[principals, authorities]())
	_, rawSetOK := rule.New(heapindex.RawSetEntry[principals, authorities]())
	_, callDispatchOK := rule.New(calldispatch.RuleEntry[principals, authorities]())
	_, effectSelectedOK := rule.New(callsite.SelectedEntry[principals, authorities]())
	_, effectOpaqueOK := rule.New(callsite.OpaqueEntry[principals, authorities]())
	_, effectBodyOK := rule.New(callsite.BodyEntry[principals, authorities]())
	_, callActivationOK := rule.New(callactivation.RuleEntry[principals, authorities]())
	_, valueRuntimeKindOK := rule.New(valueruntimekind.RuleEntry[principals, authorities]())
	_, valueBootstrapOK := rule.New(valuebootstrap.RuleEntry[principals, authorities]())
	_, heapBootstrapOK := rule.New(heapbootstrap.RuleEntry[principals, authorities]())
	_, valueTransferOK := rule.New(valuetransfer.RuleEntry[principals, authorities]())
	_, valueArithmeticOK := rule.New(valuearithmetic.RuleEntry[principals, authorities]())
	_, valueEqualityOK := rule.New(valueequality.RuleEntry[principals, authorities]())
	_, valueOrderOK := rule.New(valueorder.RuleEntry[principals, authorities]())
	_, valueRefinementOK := rule.New(valuerefinement.RuleEntry[principals, authorities]())
	return []probeRuleEntry{
		row("value-source", valueSourceOK),
		row("pack-source", packSourceOK),
		row("heap-ingress", heapIngressOK),
		row("value-allocation", valueAllocationOK),
		row("heap-empty", heapEmptyOK),
		row("heap-closed", heapClosedOK),
		row("raw-get", rawGetOK),
		row("raw-set", rawSetOK),
		row("call-dispatch", callDispatchOK),
		row("effect-selected", effectSelectedOK),
		row("effect-opaque", effectOpaqueOK),
		row("effect-body", effectBodyOK),
		row("call-activation", callActivationOK),
		row("value-runtime-kind-call", valueRuntimeKindOK),
		row("value-bootstrap", valueBootstrapOK),
		row("heap-bootstrap", heapBootstrapOK),
		row("value-transfer", valueTransferOK),
		row("value-binary-arithmetic", valueArithmeticOK),
		row("value-binary-equality", valueEqualityOK),
		row("value-binary-order", valueOrderOK),
		row("value-presence-refinement", valueRefinementOK),
	}
}

func TestRegisteredRuleSpecsAdmit(t *testing.T) {
	rejected := make([]string, 0)
	for _, row := range probeRuleEntries() {
		if !row.ok {
			rejected = append(rejected, row.name)
		}
	}
	if len(rejected) != 0 {
		t.Fatalf("rule specs did not admit: %s", strings.Join(rejected, ", "))
	}
}

// TestRuleTableSeals is the table's own totality law: the declaration root
// admits the analyzer rule surface, and the surface's coverage, ordering, and
// identity laws all hold on the production inventory.
func TestRuleTableSeals(t *testing.T) {
	table, failure := Table()
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
	if !viewOK || view.Count() != RuleCount() {
		t.Fatalf("rule surface view = %d entries, table = %d", view.Count(), RuleCount())
	}
}

// TestRuleTableCoversEveryDeclarationKeyExactlyOnce states that each table
// position publishes one unique declaration key.
func TestRuleTableCoversEveryDeclarationKeyExactlyOnce(t *testing.T) {
	seen := make(map[schema.Key]int, RuleCount())
	mounted := 0
	for position := 0; position < RuleCount(); position++ {
		key, ok := RuleKeyAt(position)
		if !ok || !key.Available() {
			t.Fatalf("table position %d has no key", position)
		}
		seen[key]++
		if seen[key] != 1 {
			t.Fatalf("key %q declared %d times", key, seen[key])
		}
		if MountedRuleKey(key) {
			mounted++
		}
	}
	if RuleCount() != mounted+len(LinkKeys()) {
		t.Fatalf("table = %d rules, mounted = %d, link = %d", RuleCount(), mounted, len(LinkKeys()))
	}
}

// TestRuleTableDrivesEveryDerivedView is the drift law. Every projection the
// analyzer consumes - identity, semantic key, diagnostic classification, lane
// membership, the write axis, and the owner that must supply the operand
// resolver - is computed from the table entry, so a rule that reaches the
// table is wired everywhere and a name that is not in the table is classified
// nowhere.
func TestRuleTableDrivesEveryDerivedView(t *testing.T) {
	links := make(map[schema.Key]bool, len(LinkKeys()))
	for _, key := range LinkKeys() {
		links[key] = true
	}
	identities := make(map[schema.EntryID]schema.Key, RuleCount())
	for position := 0; position < RuleCount(); position++ {
		key, keyOK := RuleKeyAt(position)
		if !keyOK {
			t.Fatalf("table position %d has no key", position)
		}
		id, idOK := RuleEntryID(key)
		if !idOK || !id.Available() {
			t.Fatalf("key %q has no stable table identity", key)
		}
		if prior, duplicate := identities[id]; duplicate {
			t.Fatalf("keys %q and %q share one table identity", prior, key)
		}
		identities[id] = key

		semantic, semanticOK := RuleSemantic(key)
		if !semanticOK {
			t.Fatalf("key %q has no semantic identity", key)
		}
		diagnostic := DiagnosticRuleForKey(key)
		if diagnostic == DiagnosticRuleUnknown {
			t.Fatalf("key %q has no diagnostic classification", key)
		}
		if got := DiagnosticRuleForSemantic(semantic); got != diagnostic {
			t.Fatalf("semantic classification of %q = %d, want %d", key, got, diagnostic)
		}
		if diagnostic.String() == "unknown" || diagnostic.String() == "" {
			t.Fatalf("key %q has no diagnostic name", key)
		}
		entry, entryOK := templateForKey(key)
		if !entryOK {
			t.Fatalf("key %q is not resolvable", key)
		}
		if string(entry.Key()) != diagnostic.String() {
			t.Fatalf("key %q is spelled %q by its diagnostic", key, diagnostic.String())
		}
		if _, declared := axisForKey(entry.Writes()); !declared {
			t.Fatalf("key %q writes %q, which no axis declares", key, entry.Writes())
		}
		owner, ownerOK := RuleOwner(key)
		if !ownerOK || owner != entry.Owner() {
			t.Fatalf("key %q has no owner projection", key)
		}
		if _, declared := axisForKey(owner); !declared {
			t.Fatalf("key %q owner %q names no declared axis", key, owner)
		}
		if links[key] != (entry.Lane() == rule.LaneLink) {
			t.Fatalf("key %q lane membership disagrees with the link projection", key)
		}
		if entry.Lane().Mounted() != !links[key] {
			t.Fatalf("key %q is neither mounted nor link owned", key)
		}
	}
	if len(identities) != RuleCount() {
		t.Fatalf("table identities = %d, rule count = %d", len(identities), RuleCount())
	}
	foreign, foreignOK := vocabulary.Key("rule/absent-from-the-table")
	if !foreignOK {
		t.Fatal("foreign key")
	}
	if DiagnosticRuleForSemantic(foreign) != DiagnosticRuleUnknown {
		t.Fatal("a key outside the table was classified")
	}
	if DiagnosticRuleForKey("") != DiagnosticRuleUnknown {
		t.Fatal("the empty key was classified")
	}
}

// TestRuleTableOwnerNamesABoundPrincipal is the schema join of the operand
// resolver slot: every declared rule names exactly one owner, that owner is a
// bound axis, and two rules may share an owner but one rule never names two.
func TestRuleTableOwnerNamesABoundPrincipal(t *testing.T) {
	seen := make(map[schema.Key]schema.Key, RuleCount())
	for position := 0; position < RuleCount(); position++ {
		key, keyOK := RuleKeyAt(position)
		if !keyOK {
			t.Fatalf("table position %d has no key", position)
		}
		owner, ownerOK := RuleOwner(key)
		if !ownerOK {
			t.Fatalf("key %q declares no owner", key)
		}
		entry, entryOK := axisForKey(owner)
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
	if len(seen) != RuleCount() {
		t.Fatalf("owners = %d, rule count = %d", len(seen), RuleCount())
	}
	if owner, ok := RuleOwner("no-such-rule"); ok || owner.Available() {
		t.Fatal("unknown key resolved an owner")
	}
}

// TestRuleKeyResolvesTheDeclaredSlot states that a rule's authored key names
// its declaration position.
func TestRuleKeyResolvesTheDeclaredSlot(t *testing.T) {
	for position := 0; position < RuleCount(); position++ {
		key, keyOK := RuleKeyAt(position)
		if !keyOK {
			t.Fatalf("table position %d has no key", position)
		}
		if DiagnosticRuleForKey(key) != DiagnosticRule(position+1) {
			t.Fatalf("key %q classifies away from position %d", key, position)
		}
		slot, slotOK := ruleSlotForKey(key)
		if !slotOK || slot != position+1 {
			t.Fatalf("key %q resolves slot %d, table position %d", key, slot, position+1)
		}
	}
	if DiagnosticRuleForKey("no-such-rule") != DiagnosticRuleUnknown {
		t.Fatal("unknown key classified a rule")
	}
}

// TestLinkKeysNameTheLinkLane states that LinkKeys is the declaration-key
// projection of the Link lane, in table order.
func TestLinkKeysNameTheLinkLane(t *testing.T) {
	keys := LinkKeys()
	if len(keys) == 0 {
		t.Fatal("link keys are empty")
	}
	for index, key := range keys {
		if !key.Available() {
			t.Fatalf("link key %d is empty", index)
		}
		if DiagnosticRuleForKey(key) == DiagnosticRuleUnknown {
			t.Fatalf("link key %q has no diagnostic classification", key)
		}
		if !MountedRuleKey(key) {
			if _, ok := templateForKey(key); !ok {
				t.Fatalf("link key %q is not a table rule", key)
			}
		}
		entry, ok := templateForKey(key)
		if !ok || entry.Lane() != rule.LaneLink {
			t.Fatalf("link key %q is not on the Link lane", key)
		}
	}
	if MountedRuleKey("value-bootstrap") || MountedRuleKey("heap-bootstrap") {
		t.Fatal("bootstrap keys classified as mounted")
	}
	if !MountedRuleKey("effect-selected") || !MountedRuleKey("value-source") {
		t.Fatal("mounted keys were not classified as mounted")
	}
	if MountedRuleKey("no-such-rule") {
		t.Fatal("unknown key classified as mounted")
	}
}

// TestCatalogDeclaresEveryTableRule is the cold-side drift law: the sealed
// catalog's fragment inventory is exactly the table's, so a rule reaching the
// table is declared into the schema without a second hand-kept sequence.
func TestCatalogDeclaresEveryTableRule(t *testing.T) {
	receipt, ok := Global()
	if !ok || !receipt.Available() {
		t.Fatal("global catalog")
	}
	for position := 0; position < RuleCount(); position++ {
		slot := position + 1
		if !receipt.catalog.ruleFragments[slot].Available() {
			t.Fatalf("slot %d declared no cold fragment", slot)
		}
	}
	for slot := 0; slot < len(receipt.catalog.ruleFragments); slot++ {
		_, known := templateAtSlot(slot)
		if receipt.catalog.ruleFragments[slot].Available() != known {
			t.Fatalf("slot %d has a fragment the table does not declare", slot)
		}
	}
}

// TestRuleBindingPublishesOneProgramAttachPerOperandRule is the construction
// join: every mounted operand and Link rule publishes exactly one cell-owned
// attach, and the activation lane publishes none.
func TestRuleBindingPublishesOneProgramAttachPerOperandRule(t *testing.T) {
	bound := materializerBinding(t, mountedRecord(t, "program-attach", materializerSource))
	rules := bound.Rules()
	if rules == nil {
		t.Fatal("bound rules")
	}
	published := 0
	for position := 0; position < RuleCount(); position++ {
		key, keyOK := RuleKeyAt(position)
		entry, entryOK := templateForKey(key)
		if !keyOK || !entryOK {
			t.Fatalf("table position %d has no declaration", position)
		}
		attach, attachOK := rules.ProgramAttachByKey(key)
		if entry.Lane() == rule.LaneActivation {
			if attachOK || attach != nil {
				t.Fatalf("activation %q published a program attach", key)
			}
			continue
		}
		if !attachOK || attach == nil {
			t.Fatalf("key %q has no program attach", key)
		}
		published++
	}
	if published == 0 {
		t.Fatal("no operand rule published a program attach")
	}
	if attach, ok := rules.ProgramAttachByKey("no-such-rule"); ok || attach != nil {
		t.Fatal("unknown key published a program attach")
	}
}
