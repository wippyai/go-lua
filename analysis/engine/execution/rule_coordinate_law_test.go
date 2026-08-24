package execution

import "testing"

// TestAClaimIsAPositionOfTheSealedRuleTable states what the family table IS.
//
// A rule's ordinal is its position in the sealed rule table, so the claim
// table has that table's width and a claim is a position in it. An ordinal
// past the end is not an unclaimed rule that might be claimed later - it is a
// coordinate of no table at all, and admitting it would let a claim grow the
// directory it is supposed to index.
func TestAClaimIsAPositionOfTheSealedRuleTable(t *testing.T) {
	families, opened := NewRuleFamilies[uint64, uint64](4)
	if !opened || families == nil {
		t.Fatal("a claim table over a four-rule schema")
	}
	provider := &installedFamilyProvider{rule: 3, install: installedFamily{}}
	if !families.Install(3, provider) {
		t.Fatal("the last position of the sealed table refused a claim")
	}
	if families.Install(4, &installedFamilyProvider{rule: 4, install: installedFamily{}}) {
		t.Fatal("a claim past the sealed rule table was admitted")
	}
	if _, authored := families.Installer(4); authored {
		t.Fatal("an ordinal past the sealed rule table reports an authority")
	}
	if _, opened := NewRuleFamilies[uint64, uint64](-1); opened {
		t.Fatal("a claim table over a negative rule count opened")
	}
}

// TestAbsenceInTheFamilyTableStaysSparse keeps the ordinary case whole. Most
// rules install no family, and their positions must stay empty rather than
// pack the claims down: a claimed rule is resolved at its OWN ordinal even
// when every ordinal before it is unclaimed, which is exactly the case a dense
// directory of only the claimed rules would get wrong.
func TestAbsenceInTheFamilyTableStaysSparse(t *testing.T) {
	families, opened := NewRuleFamilies[uint64, uint64](6)
	if !opened {
		t.Fatal("a claim table over a six-rule schema")
	}
	provider := &installedFamilyProvider{rule: 5, install: installedFamily{}}
	if !families.Install(5, provider) {
		t.Fatal("a claim after five unclaimed positions")
	}
	for ordinal := uint32(0); ordinal < 5; ordinal++ {
		if _, authored := families.Installer(ordinal); authored {
			t.Fatalf("unclaimed ordinal %d reports an authority", ordinal)
		}
	}
	installer, authored := families.Installer(5)
	if !authored || installer != RuleFamilyInstaller[uint64, uint64](provider) {
		t.Fatal("the claimed rule is not resolved at its own ordinal")
	}
	if families.Count() != 1 {
		t.Fatalf("table holds %d claims, want the one made", families.Count())
	}
}

// TestThePlanRowCarriesTheRuleCoordinateNotTheDescriptor is the one-assigner
// law at the seam a family is built through.
//
// One descriptor value can stand at more than one position of the sealed rule
// table, because a descriptor is geometry and a coordinate is a position. Two
// plan rows sharing a descriptor therefore route by the ordinal each ROW
// carries: the one whose rule an installer claimed reaches that installer, and
// the one whose rule nothing claimed builds through its form. A descriptor
// that answered its own ordinal could not express this, and would send both
// rows to the same authority.
func TestThePlanRowCarriesTheRuleCoordinateNotTheDescriptor(t *testing.T) {
	fixture := newExecutionFixture(t)
	exactRule := planCompiledExactRule(t)
	provider := &installedFamilyProvider{rule: lawExactRuleOrdinal, install: installedFamily{}}
	families := ruleFamilyTable(provider)
	plane := newLawFormPlane(t, fixture, families)

	claimed := FormRow{Member: 0, Form: FormExact, Input: 0, Unit: fixture.unit, Target: fixture.target, Rule: exactRule, RuleOrdinal: lawExactRuleOrdinal}
	unclaimed := FormRow{Member: 1, Form: FormExact, Input: 0, Unit: fixture.unit, Target: fixture.target, Rule: exactRule, RuleOrdinal: lawExactRuleOrdinal + 1}
	families2, addresses, _, built := BuildForms(plane, []FormRow{claimed, unclaimed})
	if !built || len(addresses) != 2 {
		t.Fatalf("two rows of one descriptor did not build: built=%t addresses=%d", built, len(addresses))
	}
	if len(provider.rows) != 1 || provider.rows[0].Member != claimed.Member {
		t.Fatalf("the installer authored %+v, want only the row whose ordinal it claimed", provider.rows)
	}
	if len(families2) != 2 {
		t.Fatalf("families = %d, want the installed one and the generic one", len(families2))
	}
}

// TestAnInstallerHoldsNoRuleCoordinate states the installer half. Which rule
// an installer authors is the claim it was installed under, and the table
// resolves it only for that claim; an installer that kept its own copy would
// be a second answer to a question the table already answers, and one that
// goes stale on its own the moment the sealed table renumbers.
func TestAnInstallerHoldsNoRuleCoordinate(t *testing.T) {
	fixture := newExecutionFixture(t)
	exactRule := planCompiledExactRule(t)
	provider := &coordinatelessProvider{}
	families := newLawRuleFamilies()
	if !families.Install(lawExactRuleOrdinal, provider) {
		t.Fatal("a coordinateless installer could not claim its rule")
	}
	plane := newLawFormPlane(t, fixture, families)
	row := FormRow{Member: 0, Form: FormExact, Input: 0, Unit: fixture.unit, Target: fixture.target, Rule: exactRule, RuleOrdinal: lawExactRuleOrdinal}
	if _, addresses, _, built := BuildForms(plane, []FormRow{row}); !built || len(addresses) != 1 {
		t.Fatalf("a coordinateless installer did not author its claimed row: built=%t addresses=%d", built, len(addresses))
	}
	if provider.calls != 1 || provider.rule != lawExactRuleOrdinal {
		t.Fatalf("installer was asked %d times for rule %d, want once for %d", provider.calls, provider.rule, lawExactRuleOrdinal)
	}
}

// coordinatelessProvider is an installer that keeps no rule ordinal at all. It
// records the ordinal the TABLE handed it, which is the only place that
// coordinate comes from.
type coordinatelessProvider struct {
	calls int
	rule  uint32
}

func (provider *coordinatelessProvider) InstallRuleFamily(plane FormPlane[uint64, uint64], rule uint32, rows []FormRow) (Family, []FormAddress, bool) {
	if provider == nil || !plane.Valid() || len(rows) == 0 {
		return nil, nil, false
	}
	provider.calls++
	provider.rule = rule
	addresses := make([]FormAddress, 0, len(rows))
	for index, row := range rows {
		addresses = append(addresses, FormAddress{Member: row.Member, Local: uint32(index)})
	}
	return installedFamily{}, addresses, true
}
