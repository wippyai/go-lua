package state

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
)

func sealTestPathEvidenceAuthority(
	t *testing.T,
	domain ProductDomain,
	keys *keyspace.KeySpace,
	valueReads, valueWrites []statekey.Value,
	coordinateReads, coordinateWrites []CoordinateSlot,
	pathMutation, writeSkeleton bool,
) CoordinatePathEvidenceAuthority[statekey.Value] {
	t.Helper()
	reads, err := domain.SealCoordinateFactorInventory(keys, coordinateReads)
	if err != nil {
		t.Fatal(err)
	}
	writes, err := domain.SealCoordinateFactorInventory(keys, coordinateWrites)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := SealCoordinatePathEvidenceAuthority(
		domain, keys, valueReads, valueWrites, reads, writes, pathMutation, writeSkeleton,
		func(slot statekey.Value) bool { return slot != 0 },
	)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}
