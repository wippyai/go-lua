package witness_test

import (
	"bytes"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

type finiteRegion struct {
	id      identity.ContentID
	members map[identity.ContentID]struct{}
}

func (region finiteRegion) Identity() (identity.ContentID, bool) {
	return region.id, region.id.Available()
}

func (region finiteRegion) Conjoin(other witness.Region) (witness.Region, bool) {
	if other == nil {
		return nil, false
	}
	otherID, ok := other.Identity()
	if !ok {
		return nil, false
	}
	result := finiteRegion{members: make(map[identity.ContentID]struct{}, len(region.members)+1)}
	for member := range region.members {
		result.members[member] = struct{}{}
	}
	if otherFinite, ok := other.(finiteRegion); ok {
		for member := range otherFinite.members {
			result.members[member] = struct{}{}
		}
	}
	if len(result.members) == 1 {
		result.id = otherID
		for member := range result.members {
			result.id = member
		}
	} else {
		ids := make([]identity.ContentID, 0, len(result.members))
		for member := range result.members {
			ids = append(ids, member)
		}
		sort.Slice(ids, func(left, right int) bool { return bytes.Compare(ids[left][:], ids[right][:]) < 0 })
		parts := make([][]byte, len(ids))
		for index, member := range ids {
			parts[index] = append([]byte(nil), member[:]...)
		}
		result.id, _ = identity.DeriveContentID("witness/test/region/conjoin/v1", parts...)
	}
	return result, true
}

func (region finiteRegion) Entails(other witness.Region) bool {
	otherFinite, ok := other.(finiteRegion)
	if !ok {
		return false
	}
	for member := range otherFinite.members {
		if _, exists := region.members[member]; !exists {
			return false
		}
	}
	return true
}

func finite(label string) finiteRegion {
	id, _ := identity.DeriveContentID("witness/test/region", []byte(label))
	return finiteRegion{id: id, members: map[identity.ContentID]struct{}{id: {}}}
}

func TestRegionLawsConjoinAndEntailment(t *testing.T) {
	left, right := finite("left"), finite("right")
	joined, ok := left.Conjoin(right)
	if !ok {
		t.Fatal("finite conjoin refused")
	}
	if !joined.Entails(left) || !joined.Entails(right) {
		t.Fatal("conjoin did not entail both operands")
	}
	if left.Entails(joined) {
		t.Fatal("narrow region unexpectedly entailed its conjunction")
	}
	reversed, ok := right.Conjoin(left)
	reversedID, reversedOK := reversed.Identity()
	joinedID, joinedOK := joined.Identity()
	if !ok || !reversedOK || !joinedOK || reversedID != joinedID {
		t.Fatal("conjoin was not commutative")
	}
	idempotent, ok := left.Conjoin(left)
	idempotentID, idempotentOK := idempotent.Identity()
	leftID, leftOK := left.Identity()
	if !ok || !idempotentOK || !leftOK || idempotentID != leftID {
		t.Fatal("conjoin was not idempotent")
	}
	if !left.Entails(left) || !joined.Entails(joined) {
		t.Fatal("entailment was not reflexive")
	}
	if !joined.Entails(left) || !left.Entails(finite("left")) {
		t.Fatal("entailment lost reflexive/transitive membership")
	}
	third := finite("third")
	all, allOK := joined.(finiteRegion).Conjoin(third)
	if !allOK || !all.Entails(joined) || !joined.Entails(left) || !all.Entails(left) {
		t.Fatal("entailment was not transitive")
	}
	if _, ok := left.Conjoin(nil); ok {
		t.Fatal("conjoin accepted nil region")
	}
}
