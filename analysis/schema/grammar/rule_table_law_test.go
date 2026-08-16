package grammar

import (
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

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

// TestRuleTableCoversEveryArtifactRoleExactlyOnce states the coverage the
// eighteen-arm switches used to assume. A role the artifact format can carry
// and the table does not declare is a loud failure here.
func TestRuleTableCoversEveryArtifactRoleExactlyOnce(t *testing.T) {
	seen := make(map[programartifact.RuleRole]int, RuleCount())
	for position := 0; position < RuleCount(); position++ {
		role, ok := RuleRoleAt(position)
		if !ok {
			t.Fatalf("table position %d has no role", position)
		}
		seen[role]++
		if seen[role] != 1 {
			t.Fatalf("role %d declared %d times", role, seen[role])
		}
		if int(role) != position+1 {
			t.Fatalf("role %d declared at position %d; the declaration order is the role ordinal", role, position)
		}
	}
	for index := 0; index < programartifact.MountedRuleRoleCount(); index++ {
		role, ok := programartifact.MountedRuleRoleAt(index)
		if !ok || seen[role] != 1 {
			t.Fatalf("mounted artifact role %d is not declared exactly once", role)
		}
	}
	if RuleCount() != programartifact.MountedRuleRoleCount()+len(LinkRoles()) {
		t.Fatalf("table = %d rules, mounted = %d, link = %d", RuleCount(), programartifact.MountedRuleRoleCount(), len(LinkRoles()))
	}
}

// TestRuleTableDrivesEveryDerivedView is the drift law. Every projection the
// analyzer consumes - identity, semantic key, diagnostic classification, lane
// membership, and the factor principal - is computed from the table entry, so
// a rule that reaches the table is wired everywhere and a name that is not in
// the table is classified nowhere.
func TestRuleTableDrivesEveryDerivedView(t *testing.T) {
	bundle, bundleOK := vocabulary.New()
	if !bundleOK {
		t.Fatal("vocabulary")
	}
	links := make(map[programartifact.RuleRole]bool, len(LinkRoles()))
	for _, role := range LinkRoles() {
		links[role] = true
	}
	identities := make(map[schema.EntryID]programartifact.RuleRole, RuleCount())
	for position := 0; position < RuleCount(); position++ {
		role, roleOK := RuleRoleAt(position)
		if !roleOK {
			t.Fatalf("table position %d has no role", position)
		}
		id, idOK := RuleEntryID(role)
		if !idOK || !id.Available() {
			t.Fatalf("role %d has no stable table identity", role)
		}
		if prior, duplicate := identities[id]; duplicate {
			t.Fatalf("roles %d and %d share one table identity", prior, role)
		}
		identities[id] = role

		semantic, semanticOK := RuleSemantic(role)
		if !semanticOK {
			t.Fatalf("role %d has no semantic identity", role)
		}
		diagnostic := DiagnosticRuleForRole(role)
		if diagnostic == DiagnosticRuleUnknown {
			t.Fatalf("role %d has no diagnostic classification", role)
		}
		if got := DiagnosticRuleForSemantic(semantic); got != diagnostic {
			t.Fatalf("semantic classification of role %d = %d, want %d", role, got, diagnostic)
		}
		if diagnostic.String() == "unknown" || diagnostic.String() == "" {
			t.Fatalf("role %d has no diagnostic name", role)
		}
		entry, entryOK := templateForRole(role)
		if !entryOK {
			t.Fatalf("role %d is not resolvable by role", role)
		}
		if string(entry.Key()) != diagnostic.String() {
			t.Fatalf("role %d is spelled %q by the table and %q by its diagnostic", role, entry.Key(), diagnostic.String())
		}
		if entry.Principal() == programartifact.RuleOutputInvalid {
			t.Fatalf("role %d writes no factor lane", role)
		}
		if links[role] != (entry.Lane() == rule.LaneLink) {
			t.Fatalf("role %d lane membership disagrees with the link projection", role)
		}
		if entry.Lane().Mounted() != !links[role] {
			t.Fatalf("role %d is neither mounted nor link owned", role)
		}
	}
	if len(identities) != RuleCount() {
		t.Fatalf("table identities = %d, rules = %d", len(identities), RuleCount())
	}
	// A key that is not in the table is classified by nothing.
	foreign, foreignOK := vocabulary.Key("rule/absent-from-the-table")
	if !foreignOK {
		t.Fatal("foreign key")
	}
	if DiagnosticRuleForSemantic(foreign) != DiagnosticRuleUnknown {
		t.Fatal("a key outside the table was classified")
	}
	if DiagnosticRuleForRole(programartifact.RuleRoleInvalid) != DiagnosticRuleUnknown {
		t.Fatal("the invalid role was classified")
	}
	_ = bundle
}

// TestGrammarDeclaresEveryTableRule is the cold-side drift law: the sealed
// grammar's fragment inventory is exactly the table's, so a rule reaching the
// table is declared into the schema without a second hand-kept sequence.
func TestGrammarDeclaresEveryTableRule(t *testing.T) {
	receipt, ok := Global()
	if !ok || !receipt.Available() {
		t.Fatal("global grammar")
	}
	declared := 0
	for position := 0; position < RuleCount(); position++ {
		role, roleOK := RuleRoleAt(position)
		if !roleOK {
			t.Fatalf("table position %d has no role", position)
		}
		if !receipt.catalog.ruleFragments[role].Available() {
			t.Fatalf("role %d declared no cold fragment", role)
		}
		declared++
	}
	if declared != RuleCount() {
		t.Fatalf("declared %d fragments for %d rules", declared, RuleCount())
	}
	for role := 0; role < ruleRoleLimit; role++ {
		_, known := templateForRole(programartifact.RuleRole(role))
		if receipt.catalog.ruleFragments[role].Available() != known {
			t.Fatalf("role %d has a fragment the table does not declare", role)
		}
	}
}
