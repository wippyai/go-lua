package composite

import (
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
)

// The declaration surfaces name no artifact catalog: an axis is its own writer
// principal and a rule's role is its declaration position. The compiled
// artifact still numbers both catalogs itself, and the composition resolves an
// artifact-addressed row through that numbering, so the agreement between the
// two is the one thing that must hold while the artifact's own catalogs are
// still standing.
//
// These laws pin that agreement member by member: the sealed declaration
// position of a row and the artifact ordinal of the member it is addressed as
// are the same number, and each pair is named by the declared key rather than
// counted. A table whose members move relative to the artifact's is rejected
// here, naming the first member that disagrees, which is what lets the
// artifact-side catalogs be deleted later against a proven map.

// positionPin is one authored agreement: the artifact ordinal a row is
// addressed by and the key the declaration at that position is declared under.
type positionPin struct {
	ordinal int
	key     schema.Key
}

// axisPositionPins is the agreement between the artifact's factor lane catalog
// and the axis inventory's declaration positions.
func axisPositionPins() []positionPin {
	return []positionPin{
		{int(programartifact.RuleOutputValue), axisKeyValue},
		{int(programartifact.RuleOutputPack), axisKeyPack},
		{int(programartifact.RuleOutputHeap), axisKeyHeap},
		{int(programartifact.RuleOutputCall), axisKeyCall},
		{int(programartifact.RuleOutputEffect), axisKeyEffect},
	}
}

// rulePositionPins is the agreement between the artifact's rule role catalog
// and the rule inventory's declaration positions.
func rulePositionPins() []positionPin {
	return []positionPin{
		{int(programartifact.RuleRoleValueSource), "value-source"},
		{int(programartifact.RuleRolePackSource), "pack-source"},
		{int(programartifact.RuleRoleHeapIngress), "heap-ingress"},
		{int(programartifact.RuleRoleValueAllocation), "value-allocation"},
		{int(programartifact.RuleRoleHeapEmpty), "heap-empty"},
		{int(programartifact.RuleRoleHeapClosed), "heap-closed"},
		{int(programartifact.RuleRoleRawGet), "raw-get"},
		{int(programartifact.RuleRoleRawSet), "raw-set"},
		{int(programartifact.RuleRoleCallDispatch), "call-dispatch"},
		{int(programartifact.RuleRoleEffectSelected), "effect-selected"},
		{int(programartifact.RuleRoleEffectOpaque), "effect-opaque"},
		{int(programartifact.RuleRoleEffectBody), "effect-body"},
		{int(programartifact.RuleRoleCallActivation), "call-activation"},
		{int(programartifact.RuleRoleValueBootstrap), "value-bootstrap"},
		{int(programartifact.RuleRoleHeapBootstrap), "heap-bootstrap"},
		{int(programartifact.RuleRoleValueStorageTransfer), "value-transfer"},
		{int(programartifact.RuleRoleValueBinaryArithmetic), "value-binary-arithmetic"},
		{int(programartifact.RuleRoleValueBinaryEquality), "value-binary-equality"},
		{int(programartifact.RuleRoleValueBinaryOrder), "value-binary-order"},
		{int(programartifact.RuleRoleValuePresenceRefinement), "value-presence-refinement"},
	}
}

// positionAgreement states one inventory's agreement with an authored pin set
// and names the first member that disagrees. The inventory is given as the
// declared keys in table order, so the law reads the same over the production
// table and over a copy whose members have been moved.
func positionAgreement(keys []schema.Key, pins []positionPin) (schema.Key, bool) {
	if len(keys) != len(pins) {
		return "", false
	}
	for _, pin := range pins {
		if pin.ordinal <= 0 || pin.ordinal > len(keys) {
			return pin.key, false
		}
		if keys[pin.ordinal-1] != pin.key {
			return pin.key, false
		}
	}
	return "", true
}

func sealedAxisKeys(t *testing.T) []schema.Key {
	t.Helper()
	keys := make([]schema.Key, AxisCount())
	for position := range keys {
		key, ok := AxisKeyAt(position)
		if !ok {
			t.Fatalf("axis position %d publishes no key", position)
		}
		keys[position] = key
	}
	return keys
}

func sealedRuleKeys(t *testing.T) []schema.Key {
	t.Helper()
	keys := make([]schema.Key, RuleCount())
	for position := range keys {
		key, ok := RuleKeyAt(position)
		if !ok {
			t.Fatalf("rule position %d publishes no key", position)
		}
		keys[position] = key
	}
	return keys
}

// sealedFactorAxisKeys is the prefix of the axis inventory the artifact's
// factor lane catalog addresses, and it states that the prefix is one: the
// lane-addressed axes are declared first, so a factor axis's declaration
// position is its lane ordinal and an axis no lane numbers cannot displace one.
// An engine-published axis is such a row - the artifact numbers factor lanes,
// and that axis is not one - so it is declared after them and the pins below
// stay a member-for-member agreement rather than a count.
func sealedFactorAxisKeys(t *testing.T) []schema.Key {
	t.Helper()
	keys := sealedAxisKeys(t)
	addressed := len(axisPositionPins())
	if len(keys) < addressed {
		t.Fatalf("the table declares %d axes, the artifact addresses %d factor lanes", len(keys), addressed)
	}
	for position, key := range keys {
		storage, storageOK := AxisStorage(key)
		if !storageOK {
			t.Fatalf("axis %q declares no storage", key)
		}
		if factor := storage == axis.StorageFactor; factor != (position < addressed) {
			t.Fatalf("axis %q declares storage %d at position %d, outside the artifact-addressed prefix", key, storage, position)
		}
	}
	return keys[:addressed]
}

// TestAxisDeclarationPositionAgreesWithTheArtifactLaneOrdinal states the axis
// half of the agreement, and states it as a law rather than as an observation:
// a table whose members have moved is rejected, naming the member.
func TestAxisDeclarationPositionAgreesWithTheArtifactLaneOrdinal(t *testing.T) {
	keys := sealedFactorAxisKeys(t)
	if blamed, agreed := positionAgreement(keys, axisPositionPins()); !agreed {
		t.Fatalf("axis %q is not declared at the position its artifact lane ordinal addresses", blamed)
	}
	swapped := append([]schema.Key(nil), keys...)
	swapped[0], swapped[1] = swapped[1], swapped[0]
	blamed, agreed := positionAgreement(swapped, axisPositionPins())
	if agreed {
		t.Fatal("a moved axis agreed with the artifact lane ordinals")
	}
	if blamed != keys[0] {
		t.Fatalf("a moved axis was blamed on %q, not on the first member that disagrees", blamed)
	}
}

// TestRuleDeclarationPositionAgreesWithTheArtifactRoleOrdinal states the rule
// half of the same agreement.
func TestRuleDeclarationPositionAgreesWithTheArtifactRoleOrdinal(t *testing.T) {
	keys := sealedRuleKeys(t)
	if blamed, agreed := positionAgreement(keys, rulePositionPins()); !agreed {
		t.Fatalf("rule %q is not declared at the position its artifact role ordinal addresses", blamed)
	}
	swapped := append([]schema.Key(nil), keys...)
	swapped[0], swapped[1] = swapped[1], swapped[0]
	blamed, agreed := positionAgreement(swapped, rulePositionPins())
	if agreed {
		t.Fatal("a moved rule agreed with the artifact role ordinals")
	}
	if blamed != keys[0] {
		t.Fatalf("a moved rule was blamed on %q, not on the first member that disagrees", blamed)
	}
}

// TestArtifactAddressedRowsResolveThroughTheSealedTable states that the
// agreement is what the composition actually resolves through: every artifact
// role addresses the declaration at its own position, and every rule's artifact
// lane addresses a declared axis, so no artifact-addressed row reaches a
// projection the table does not hold.
func TestArtifactAddressedRowsResolveThroughTheSealedTable(t *testing.T) {
	for _, pin := range rulePositionPins() {
		entry, declared := templateForRole(programartifact.RuleRole(pin.ordinal))
		if !declared || entry.Key() != pin.key {
			t.Fatalf("artifact role %d resolves to no declaration of %q", pin.ordinal, pin.key)
		}
		lane := programartifact.RuleOutputKindFor(programartifact.RuleRole(pin.ordinal))
		axisEntry, axisDeclared := axisAtSlot(int(lane))
		if !axisDeclared {
			t.Fatalf("rule %q writes artifact lane %d, which addresses no declared axis", pin.key, lane)
		}
		if DiagnosticAxisForKey(axisEntry.Key()) != DiagnosticAxis(lane) {
			t.Fatalf("axis %q classifies away from the slot the artifact lane addresses", axisEntry.Key())
		}
	}
}
