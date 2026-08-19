package composite

import (
	"testing"

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
		{1, axisKeyValue},
		{2, axisKeyPack},
		{3, axisKeyHeap},
		{4, axisKeyCall},
		{5, axisKeyEffect},
	}
}

// rulePositionPins is the sealed rule inventory in declaration order.
func rulePositionPins() []positionPin {
	return []positionPin{
		{1, "value-source"},
		{2, "pack-source"},
		{3, "heap-ingress"},
		{4, "value-allocation"},
		{5, "heap-empty"},
		{6, "heap-closed"},
		{7, "raw-get"},
		{8, "raw-set"},
		{9, "call-dispatch"},
		{10, "effect-selected"},
		{11, "effect-opaque"},
		{12, "effect-body"},
		{13, "call-activation"},
		{14, "value-runtime-kind-call"},
		{15, "value-bootstrap"},
		{16, "heap-bootstrap"},
		{17, "value-transfer"},
		{18, "value-binary-arithmetic"},
		{19, "value-binary-equality"},
		{20, "value-binary-order"},
		{21, "value-presence-refinement"},
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
		entry, declared := templateForKey(pin.key)
		if !declared || entry.Key() != pin.key {
			t.Fatalf("key %q resolves to no declaration", pin.key)
		}
		axisEntry, axisDeclared := axisForKey(entry.Writes())
		if !axisDeclared {
			t.Fatalf("rule %q writes %q, which addresses no declared axis", pin.key, entry.Writes())
		}
		if DiagnosticAxisForKey(axisEntry.Key()) != DiagnosticAxisForKey(entry.Writes()) {
			t.Fatalf("axis %q classifies away from Writes %q", axisEntry.Key(), entry.Writes())
		}
	}
}
