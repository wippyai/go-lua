package composite

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
)

// The artifact still numbers factor lanes. This law pins only that live
// external addressing contract. Rules no longer have a second Artifact role
// catalog, so their declaration table is not repeated here.

// positionPin is one authored agreement: the artifact ordinal a row is
// addressed by and the key the declaration at that position is declared under.
type positionPin struct {
	ordinal int
	key     schema.Key
}

// axisPositionPins is the agreement between the artifact's factor lane catalog
// and the axis inventory's declaration positions.
//
// The pin count is what the inventory declares with factor storage: nine axes
// carry axis.StorageFactor, and every one of them is declared ahead of the
// fourteen engine-published rows, so nine is the length of the addressed
// prefix rather than a number chosen to fit. Each ordinal is a lane a factor
// binding is addressed by, so a pin added here states that a coordinate space
// entered the addressed prefix, and the law below still rejects a member that
// moved within it.
func axisPositionPins() []positionPin {
	return []positionPin{
		{1, axisKeyValue},
		{2, axisKeyPack},
		{3, axisKeyHeap},
		{4, axisKeyCall},
		{5, axisKeyEffect},
		{6, axisKeyPlacement},
		{7, axisKeyPlacementEvidence},
		{8, axisKeyContext},
		{9, axisKeyStaticType},
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

func sealedAxisKeys(t *testing.T, compilation Compilation) []schema.Key {
	t.Helper()
	keys := make([]schema.Key, AxisCount(compilation))
	for position := range keys {
		key, ok := AxisKeyAt(compilation, position)
		if !ok {
			t.Fatalf("axis position %d publishes no key", position)
		}
		keys[position] = key
	}
	return keys
}

func sealedRuleKeys(t *testing.T, compilation Compilation) []schema.Key {
	t.Helper()
	keys := make([]schema.Key, RuleCount(compilation))
	for position := range keys {
		key, ok := RuleKeyAt(compilation, position)
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
func sealedFactorAxisKeys(t *testing.T, compilation Compilation) []schema.Key {
	t.Helper()
	keys := sealedAxisKeys(t, compilation)
	addressed := len(axisPositionPins())
	if len(keys) < addressed {
		t.Fatalf("the table declares %d axes, the artifact addresses %d factor lanes", len(keys), addressed)
	}
	for position, key := range keys {
		storage, storageOK := AxisStorage(compilation, key)
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
	compilation, compilationOK := Build()
	if !compilationOK {
		t.Fatal("compilation unavailable")
	}
	keys := sealedFactorAxisKeys(t, compilation)
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
