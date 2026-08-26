package link_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

func TestLinkMountIDIsPerShardAndOwnerFenced(t *testing.T) {
	p := source(t, `return 1`)
	target := contract(t)
	sealed := linked(t, target,
		linkproject.Module{Name: "first", Program: p},
		linkproject.Module{Name: "second", Program: p},
	)
	mounts := sealed.Project().Mounts()
	first, firstOK := mounts.At(0)
	second, secondOK := mounts.At(1)
	if !firstOK || !secondOK {
		t.Fatal("missing duplicate-Program mount shards")
	}
	firstID, firstIDOK := sealed.MountID(first)
	secondID, secondIDOK := sealed.MountID(second)
	if !firstIDOK || !secondIDOK || !firstID.Available() || !secondID.Available() {
		t.Fatal("Link did not issue both mount identities")
	}
	if firstID == secondID {
		t.Fatal("distinct Project mount shards share a MountID")
	}

	moduleKey, moduleKeyOK := sealed.Project().ModuleKey(first)
	linkID := sealed.ContentID()
	wantContent, wantOK := identity.DeriveContentID("analysis/program/link/mount/v1", linkID[:], moduleKey[:])
	want := identity.MountID(wantContent)
	if !moduleKeyOK || !wantOK || firstID != want {
		t.Fatal("MountID is not the full-width Link.ContentID+ModuleKey derivation")
	}

	foreign := linked(t, target, linkproject.Module{Name: "first", Program: p})
	foreignShard, foreignShardOK := foreign.Project().Mounts().At(0)
	if !foreignShardOK {
		t.Fatal("missing foreign Project shard")
	}
	if got, ok := sealed.MountID(foreignShard); ok || got.Available() {
		t.Fatal("foreign Project shard crossed the Link Project owner fence")
	}

	resealed := linked(t, target,
		linkproject.Module{Name: "first", Program: p},
		linkproject.Module{Name: "second", Program: p},
	)
	resealedShard, resealedShardOK := resealed.Project().Mounts().At(0)
	if !resealedShardOK {
		t.Fatal("missing resealed Project shard")
	}
	resealedID, resealedIDOK := resealed.MountID(resealedShard)
	if !resealedIDOK || resealedID != firstID {
		t.Fatal("identical sealed Link inputs changed the MountID")
	}
}

func TestLinkMountIDRejectsNilAndZeroShards(t *testing.T) {
	var nilLink *link.Link
	if got, ok := nilLink.MountID(linkproject.Shard{}); ok || got.Available() {
		t.Fatal("nil Link issued a MountID")
	}

	p := source(t, `return 1`)
	sealed := linked(t, contract(t), linkproject.Module{Name: "main", Program: p})
	if got, ok := sealed.MountID(linkproject.Shard{}); ok || got.Available() {
		t.Fatal("zero Project shard issued a MountID")
	}
}
