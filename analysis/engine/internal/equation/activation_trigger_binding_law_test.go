package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

func triggerBindingKey(value byte) composition.Key {
	var id composition.ID
	id[0], id[1] = 0xa7, value
	return composition.Key{ID: id, Version: 1}
}

// triggerBindingFixture is one sealed composition holding a single activation
// trigger rule, plus the source batch its one instance is admitted from.
type triggerBindingFixture struct {
	cold        *composition.Composition
	batch       *Batch
	site        Site
	rule        composition.Key
	family      composition.Key
	application composition.Key
	factor      composition.Key
	export      composition.Key
	instance    RuleInstance
	group       Group
	points      []PointSpec
}

func newTriggerBindingFixture(t testing.TB) triggerBindingFixture {
	t.Helper()
	factor, rule, family := triggerBindingKey(1), triggerBindingKey(2), triggerBindingKey(3)
	operandFamily, export := triggerBindingKey(4), triggerBindingKey(10)
	cold, coldOK := composition.Seal(composition.Candidate{
		Factors:            []composition.Factor{{Key: factor}, {Key: export}},
		ActivationFamilies: []composition.ActivationFamily{{Semantic: family}},
		Rules: []composition.Rule{{
			Key:           rule,
			OperandFamily: operandFamily,
			OutputKind:    composition.StructuralOutput,
			Activations:   []composition.ActivationRange{{Family: family}},
		}},
	})
	if !coldOK || cold == nil {
		t.Fatal("trigger binding composition")
	}
	scope := EmptyScope()
	batch := NewBatch()
	site, siteOK := batch.AdmitSite(triggerBindingKey(6), scope, TrueExpr(), InitPresent)
	occurrence, occurrenceOK := batch.At(site)
	operand, operandOK := batch.AdmitOperand(occurrence, triggerBindingKey(7))
	if !siteOK || !occurrenceOK || !operandOK || !batch.Seal() {
		t.Fatal("trigger binding batch")
	}
	return triggerBindingFixture{
		cold: cold, batch: batch, site: site, rule: rule, family: family, application: triggerBindingKey(8),
		factor: factor, export: export,
		instance: RuleInstance{Schema: rule, OperandFamily: operandFamily, Occurrence: occurrence, Operand: operand},
		group:    Group{Members: []RuleRef{RuleAt(0)}, Output: PointAt(0)},
		points:   []PointSpec{{Site: site}},
	}
}

func (fixture triggerBindingFixture) seal(bindings []ActivationTriggerBinding) (*Topology, bool) {
	return fixture.sealWithRows(bindings, nil)
}

func (fixture triggerBindingFixture) sealWithRows(bindings []ActivationTriggerBinding, rows []ActivationRowSpec) (*Topology, bool) {
	return SealTopology(fixture.cold, TopologySpec{
		Batch:              fixture.batch,
		Rules:              []RuleInstance{fixture.instance},
		Points:             fixture.points,
		Groups:             []Group{fixture.group},
		ActivationRows:     rows,
		ActivationTriggers: bindings,
	})
}

// row builds one direct activation row anchored on the single trigger
// instance, under the locator the caller states.
func (fixture triggerBindingFixture) row(application, target, endpoint composition.Key) ActivationRowSpec {
	return ActivationRowSpec{
		TriggerOrdinal: 0,
		Family:         fixture.family,
		Application:    application,
		Target:         target,
		Endpoint:       endpoint,
		Trigger:        PointAt(0),
		Entries:        []PointRef{PointAt(0)},
		Exits:          []PointRef{PointAt(0)},
		Imports:        []composition.Key{fixture.factor},
		Exports:        []composition.Key{fixture.export},
	}
}

// TestDirectActivationTransportCarriesEachDeclaredDirection proves the sealed
// transport ABI is a direction declaration, not a one-export special case. A
// factor present in both sets produces one ingress and one egress edge, while
// an export-only factor produces only egress.
func TestDirectActivationTransportCarriesEachDeclaredDirection(t *testing.T) {
	fixture := newTriggerBindingFixture(t)
	rows, ok := activationTransportRows(fixture.cold, ActivationRowSpec{
		Trigger: PointAt(1), Entries: []PointRef{PointAt(2)}, Exits: []PointRef{PointAt(3)},
		Imports: []composition.Key{fixture.factor}, Exports: []composition.Key{fixture.factor, fixture.export},
	})
	if !ok || len(rows) != 3 {
		t.Fatalf("direct activation transport rows=%d ok=%t, want three declared directional edges", len(rows), ok)
	}
	want := []struct {
		source, target PointRef
		factor         composition.Key
	}{
		{PointAt(1), PointAt(2), fixture.factor},
		{PointAt(3), PointAt(1), fixture.factor},
		{PointAt(3), PointAt(1), fixture.export},
	}
	for _, expected := range want {
		found := false
		for _, row := range rows {
			if row.source == expected.source && row.target == expected.target && row.factor == expected.factor {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing transport %v -> %v for %v", expected.source, expected.target, expected.factor)
		}
	}
}

// TestActivationTriggerIsBoundWithoutACandidate proves the trigger's family and
// application come from its own declaration. A trigger that reaches no
// candidate is bound, projects its application, and publishes its activation
// member row, because none of those distinctions live in a candidate receipt.
func TestActivationTriggerIsBoundWithoutACandidate(t *testing.T) {
	fixture := newTriggerBindingFixture(t)
	topology, sealed := fixture.seal([]ActivationTriggerBinding{{TriggerOrdinal: 0, Family: fixture.family, Application: fixture.application}})
	if !sealed || topology == nil {
		t.Fatal("a candidate-free activation trigger refused its topology")
	}
	trigger := topology.rows.members[0]
	if !topology.TriggerBound(trigger, fixture.family) {
		t.Fatal("declared trigger is not bound")
	}
	application, projected := topology.ActivationApplication(trigger, fixture.family)
	if !projected || application != fixture.application {
		t.Fatalf("trigger projected application %v, declared %v", application, fixture.application)
	}
	if _, addressed := topology.ActivationMemberRow(RuleAt(0)); !addressed {
		t.Fatal("candidate-free trigger has no activation member row")
	}
}

// TestUndeclaredActivationTriggerIsNotBound proves the binding is the only
// authority: an instance the declaration did not name as a trigger answers
// nothing, whatever its schema permits.
func TestUndeclaredActivationTriggerIsNotBound(t *testing.T) {
	fixture := newTriggerBindingFixture(t)
	topology, sealed := fixture.seal(nil)
	if !sealed || topology == nil {
		t.Fatal("topology without a declared trigger refused")
	}
	trigger := topology.rows.members[0]
	if topology.TriggerBound(trigger, fixture.family) {
		t.Fatal("an undeclared instance answered as a bound trigger")
	}
	if _, projected := topology.ActivationApplication(trigger, fixture.family); projected {
		t.Fatal("an undeclared instance projected an application")
	}
	if _, addressed := topology.ActivationMemberRow(RuleAt(0)); addressed {
		t.Fatal("an undeclared instance published an activation member row")
	}
}

// TestActivationTriggerBindingRejectsMalformedDeclarations keeps the binding
// exact: it names a real instance, a family that instance carries, an
// available application, and it names each instance once.
func TestActivationTriggerBindingRejectsMalformedDeclarations(t *testing.T) {
	fixture := newTriggerBindingFixture(t)
	good := ActivationTriggerBinding{TriggerOrdinal: 0, Family: fixture.family, Application: fixture.application}
	for name, bindings := range map[string][]ActivationTriggerBinding{
		"out-of-range":       {{TriggerOrdinal: 1, Family: fixture.family, Application: fixture.application}},
		"negative":           {{TriggerOrdinal: -1, Family: fixture.family, Application: fixture.application}},
		"unknown-family":     {{TriggerOrdinal: 0, Family: triggerBindingKey(9), Application: fixture.application}},
		"no-application":     {{TriggerOrdinal: 0, Family: fixture.family}},
		"bound-twice":        {good, good},
		"unavailable-family": {{TriggerOrdinal: 0, Application: fixture.application}},
	} {
		if topology, sealed := fixture.seal(bindings); sealed || topology != nil {
			t.Fatalf("%s trigger binding was sealed", name)
		}
	}
}

// TestActivationRowCannotDisagreeWithItsTrigger keeps the row plane downstream
// of the trigger declaration: a row indexes transports of a bound trigger
// under that trigger's family and application, and cannot introduce a trigger
// or application of its own.
func TestActivationRowCannotDisagreeWithItsTrigger(t *testing.T) {
	fixture := newTriggerBindingFixture(t)
	declared := []ActivationTriggerBinding{{TriggerOrdinal: 0, Family: fixture.family, Application: fixture.application}}
	agreeing := fixture.row(fixture.application, triggerBindingKey(11), triggerBindingKey(12))
	topology, sealed := fixture.sealWithRows(declared, []ActivationRowSpec{agreeing})
	if !sealed || topology == nil {
		t.Fatal("a row agreeing with its declared trigger was refused")
	}
	for name, row := range map[string]ActivationRowSpec{
		"foreign-application": fixture.row(triggerBindingKey(13), triggerBindingKey(11), triggerBindingKey(12)),
		"unbound-trigger":     agreeing,
	} {
		bindings := declared
		if name == "unbound-trigger" {
			bindings = nil
		}
		if topology, sealed := fixture.sealWithRows(bindings, []ActivationRowSpec{row}); sealed || topology != nil {
			t.Fatalf("%s row was sealed", name)
		}
	}
}
