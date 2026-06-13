package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
)

func TestSnapshotsCloneFiniteLanes(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	present := presentValue(reg)
	absent := absentValue(reg)

	pathKey := pathdom.PathKey("sym130@1.path")
	memberKey := pathdom.PathKey("sym130@1.member")
	dynamicKey := DynamicIndexKey{Table: pathdom.PathKey("sym130@1.table"), Site: "dyn"}
	effectKey := effectdelta.Key{Target: pathdom.PathKey("sym130@1.table"), Site: "effect", Kind: effectdelta.Mutation}
	proof := BranchProof{Kind: BranchProofPathPresence, Path: pathKey, Presence: presence.Present()}
	otherProof := BranchProof{Kind: BranchProofPathNotEqual, Path: pathKey, Other: memberKey}
	selectFact := ChannelSelectFact{Select: "select-snapshot", Kind: ChannelSelectFactSelect, Result: pathKey}
	otherSelectFact := ChannelSelectFact{Select: "select-snapshot", Kind: ChannelSelectFactCase, Case: memberKey, Index: 1}
	dynamicFact := DynamicIndexFact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
		Admission:   DynamicIndexAdmissionAdmitted,
	}
	otherDynamicFact := DynamicIndexFact{
		KeyPresence: presence.Absent(),
		KeyValue:    absent,
		Value:       absent,
		Admission:   DynamicIndexAdmissionRejected,
	}
	effectDelta := effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeChanged}
	otherEffectDelta := effectdelta.Value{Before: absent, After: absent, Change: effectdelta.ChangeNone}

	s := State{}.
		WritePathKey(reg, pathKey, present).
		WritePathStaticMember(memberKey, present).
		WriteDynamicIndexFact(reg, dynamicKey, dynamicFact).
		AddBranchProof(proof).
		AddChannelSelectFact(selectFact).
		WriteEffectDelta(effectKey, effectDelta)

	pathSnapshot := s.PathRefinementsSnapshot()
	if pathSnapshot.Top || len(pathSnapshot.Refinements) != 1 {
		t.Fatalf("path snapshot = %#v, want one finite refinement", pathSnapshot)
	}
	pathSnapshot.Refinements[pathKey] = absent
	if got := s.ReadPathKey(reg, pathKey); !valueDomain.Equal(got, present) {
		t.Fatalf("path snapshot mutation changed state to %s", formatValue(reg, got))
	}
	if got := s.PathRefinementsSnapshot().Refinements[pathKey]; !valueDomain.Equal(got, present) {
		t.Fatalf("fresh path snapshot = %s, want present", formatValue(reg, got))
	}

	memberSnapshot := s.PathStaticMembersSnapshot()
	if memberSnapshot.Bottom || memberSnapshot.Top || len(memberSnapshot.Members) != 1 {
		t.Fatalf("static-member snapshot = %#v, want one finite member", memberSnapshot)
	}
	memberSnapshot.Members[memberKey] = absent
	if got, ok := s.ReadPathStaticMember(memberKey); !ok || !valueDomain.Equal(got, present) {
		t.Fatalf("static-member snapshot mutation changed state to %s ok=%v", formatValue(reg, got), ok)
	}
	if got := s.PathStaticMembersSnapshot().Members[memberKey]; !valueDomain.Equal(got, present) {
		t.Fatalf("fresh static-member snapshot = %s, want present", formatValue(reg, got))
	}

	dynamicSnapshot := s.DynamicIndexFactsSnapshot()
	if dynamicSnapshot.Top || len(dynamicSnapshot.Facts) != 1 {
		t.Fatalf("dynamic-index snapshot = %#v, want one finite fact", dynamicSnapshot)
	}
	dynamicSnapshot.Facts[dynamicKey] = otherDynamicFact
	if got := s.ReadDynamicIndexFact(reg, dynamicKey); !dynamicIndexFactDomain(reg).Equal(got, dynamicFact) {
		t.Fatalf("dynamic-index snapshot mutation changed state to %#v", got)
	}
	if got := s.DynamicIndexFactsSnapshot().Facts[dynamicKey]; !dynamicIndexFactDomain(reg).Equal(got, dynamicFact) {
		t.Fatalf("fresh dynamic-index snapshot = %#v, want original fact", got)
	}

	branchSnapshot := s.BranchProofsSnapshot()
	if branchSnapshot.Bottom || branchSnapshot.Top || len(branchSnapshot.Proofs) != 1 {
		t.Fatalf("branch-proof snapshot = %#v, want one finite proof", branchSnapshot)
	}
	branchSnapshot.Proofs[0] = otherProof
	if !s.HasBranchProof(proof) || s.HasBranchProof(otherProof) {
		t.Fatalf("branch-proof snapshot mutation changed state")
	}
	if got := s.BranchProofsSnapshot().Proofs[0]; got != proof {
		t.Fatalf("fresh branch-proof snapshot = %#v, want original proof", got)
	}

	channelSnapshot := s.ChannelSelectFactsSnapshot()
	if channelSnapshot.Bottom || channelSnapshot.Top || len(channelSnapshot.Facts) != 1 {
		t.Fatalf("channel-select snapshot = %#v, want one finite fact", channelSnapshot)
	}
	channelSnapshot.Facts[0] = otherSelectFact
	if !s.HasChannelSelectFact(selectFact) || s.HasChannelSelectFact(otherSelectFact) {
		t.Fatalf("channel-select snapshot mutation changed state")
	}
	if got := s.ChannelSelectFactsSnapshot().Facts[0]; got != selectFact {
		t.Fatalf("fresh channel-select snapshot = %#v, want original fact", got)
	}

	effectSnapshot := s.EffectDeltasSnapshot()
	if effectSnapshot.Top || len(effectSnapshot.Deltas) != 1 {
		t.Fatalf("effect-delta snapshot = %#v, want one finite delta", effectSnapshot)
	}
	effectSnapshot.Deltas[effectKey] = otherEffectDelta
	if got := s.ReadEffectDelta(effectKey); !effectdelta.Domain(reg).Equal(got, effectDelta) {
		t.Fatalf("effect-delta snapshot mutation changed state to %#v", got)
	}
	if got := s.EffectDeltasSnapshot().Deltas[effectKey]; !effectdelta.Domain(reg).Equal(got, effectDelta) {
		t.Fatalf("fresh effect-delta snapshot = %#v, want original delta", got)
	}
}

func TestSnapshotTopBottomAndEmptyLanes(t *testing.T) {
	reg := standard.Registry()
	top := Domain(reg).Top()
	bottom := Domain(reg).Bottom()
	empty := State{}

	topPaths := top.PathRefinementsSnapshot()
	if !topPaths.Top || len(topPaths.Refinements) != 0 {
		t.Fatalf("top path snapshot = %#v, want top with no finite facts", topPaths)
	}
	topDynamic := top.DynamicIndexFactsSnapshot()
	if !topDynamic.Top || len(topDynamic.Facts) != 0 {
		t.Fatalf("top dynamic-index snapshot = %#v, want top with no finite facts", topDynamic)
	}
	topEffects := top.EffectDeltasSnapshot()
	if !topEffects.Top || len(topEffects.Deltas) != 0 {
		t.Fatalf("top effect-delta snapshot = %#v, want top with no finite facts", topEffects)
	}
	topMembers := top.PathStaticMembersSnapshot()
	if topMembers.Bottom || !topMembers.Top || len(topMembers.Members) != 0 {
		t.Fatalf("top static-member snapshot = %#v, want top with no finite facts", topMembers)
	}
	topProofs := top.BranchProofsSnapshot()
	if topProofs.Bottom || !topProofs.Top || len(topProofs.Proofs) != 0 {
		t.Fatalf("top branch-proof snapshot = %#v, want top with no finite facts", topProofs)
	}
	topChannel := top.ChannelSelectFactsSnapshot()
	if topChannel.Bottom || !topChannel.Top || len(topChannel.Facts) != 0 {
		t.Fatalf("top channel-select snapshot = %#v, want top with no finite facts", topChannel)
	}

	bottomPaths := bottom.PathRefinementsSnapshot()
	if bottomPaths.Top || len(bottomPaths.Refinements) != 0 {
		t.Fatalf("bottom path snapshot = %#v, want finite empty pointwise lane", bottomPaths)
	}
	bottomDynamic := bottom.DynamicIndexFactsSnapshot()
	if bottomDynamic.Top || len(bottomDynamic.Facts) != 0 {
		t.Fatalf("bottom dynamic-index snapshot = %#v, want finite empty pointwise lane", bottomDynamic)
	}
	bottomEffects := bottom.EffectDeltasSnapshot()
	if bottomEffects.Top || len(bottomEffects.Deltas) != 0 {
		t.Fatalf("bottom effect-delta snapshot = %#v, want finite empty pointwise lane", bottomEffects)
	}
	bottomMembers := bottom.PathStaticMembersSnapshot()
	if !bottomMembers.Bottom || bottomMembers.Top || len(bottomMembers.Members) != 0 {
		t.Fatalf("bottom static-member snapshot = %#v, want explicit bottom", bottomMembers)
	}
	bottomProofs := bottom.BranchProofsSnapshot()
	if !bottomProofs.Bottom || bottomProofs.Top || len(bottomProofs.Proofs) != 0 {
		t.Fatalf("bottom branch-proof snapshot = %#v, want explicit bottom", bottomProofs)
	}
	bottomChannel := bottom.ChannelSelectFactsSnapshot()
	if !bottomChannel.Bottom || bottomChannel.Top || len(bottomChannel.Facts) != 0 {
		t.Fatalf("bottom channel-select snapshot = %#v, want explicit bottom", bottomChannel)
	}

	emptyPaths := empty.PathRefinementsSnapshot()
	if emptyPaths.Top || len(emptyPaths.Refinements) != 0 {
		t.Fatalf("empty path snapshot = %#v, want finite empty pointwise lane", emptyPaths)
	}
	emptyDynamic := empty.DynamicIndexFactsSnapshot()
	if emptyDynamic.Top || len(emptyDynamic.Facts) != 0 {
		t.Fatalf("empty dynamic-index snapshot = %#v, want finite empty pointwise lane", emptyDynamic)
	}
	emptyEffects := empty.EffectDeltasSnapshot()
	if emptyEffects.Top || len(emptyEffects.Deltas) != 0 {
		t.Fatalf("empty effect-delta snapshot = %#v, want finite empty pointwise lane", emptyEffects)
	}
	emptyMembers := empty.PathStaticMembersSnapshot()
	if emptyMembers.Bottom || !emptyMembers.Top || len(emptyMembers.Members) != 0 {
		t.Fatalf("empty static-member snapshot = %#v, want reachable top/empty", emptyMembers)
	}
	emptyProofs := empty.BranchProofsSnapshot()
	if emptyProofs.Bottom || !emptyProofs.Top || len(emptyProofs.Proofs) != 0 {
		t.Fatalf("empty branch-proof snapshot = %#v, want reachable top/empty", emptyProofs)
	}
	emptyChannel := empty.ChannelSelectFactsSnapshot()
	if emptyChannel.Bottom || !emptyChannel.Top || len(emptyChannel.Facts) != 0 {
		t.Fatalf("empty channel-select snapshot = %#v, want reachable top/empty", emptyChannel)
	}
}
