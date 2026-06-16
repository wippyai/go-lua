package state

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestSnapshotsCloneFiniteLanes(t *testing.T) {
	reg := standard.Registry()
	valueDomain := product.Domain(reg)
	present := presentValue(reg)
	absent := absentValue(reg)

	pathKey := pathdom.PathKey("sym130@1.path")
	memberKey := pathdom.PathKey("sym130@1.member")
	dynamicKey := dynamicindex.Key{Table: pathdom.PathKey("sym130@1.table"), Site: "dyn"}
	effectKey := effectdelta.Key{Target: pathdom.PathKey("sym130@1.table"), Site: "effect", Kind: effectdelta.Mutation}
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: pathKey, Presence: presence.Present()}
	otherProof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathNotEqual, Path: pathKey, Other: memberKey}
	implication := pathevidence.PathPresenceImplication{
		Trigger:         pathKey,
		TriggerPresence: presence.Absent(),
		Target:          memberKey,
		TargetPresence:  presence.Present(),
	}
	otherImplication := pathevidence.PathPresenceImplication{
		Trigger:         memberKey,
		TriggerPresence: presence.Present(),
		Target:          pathKey,
		TargetPresence:  presence.Absent(),
	}
	selectFact := channelselectfact.Fact{Select: "select-snapshot", Kind: channelselectfact.FactSelect, Result: pathKey}
	otherSelectFact := channelselectfact.Fact{Select: "select-snapshot", Kind: channelselectfact.FactCase, Case: memberKey, Index: 1}
	dynamicFact := dynamicindex.Fact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
		Admission:   dynamicindex.AdmissionAdmitted,
	}
	otherDynamicFact := dynamicindex.Fact{
		KeyPresence: presence.Absent(),
		KeyValue:    absent,
		Value:       absent,
		Admission:   dynamicindex.AdmissionRejected,
	}
	heapID := identity.ID{Kind: "table", Site: "snapshot", Index: 1}
	frozenID := identity.ID{Kind: "table", Site: "snapshot-freeze", Index: 1}
	otherFrozenID := identity.ID{Kind: "table", Site: "snapshot-freeze", Index: 2}
	effectDelta := effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeChanged}
	otherEffectDelta := effectdelta.Value{Before: absent, After: absent, Change: effectdelta.ChangeNone}

	s := State{}.
		WritePathKey(reg, pathKey, present).
		WritePathStaticMember(memberKey, present).
		WriteDynamicIndexFact(reg, dynamicKey, dynamicFact).
		WriteHeapTableObject(reg, heapID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          present,
			StaticMembers: map[pathdom.PathKey]product.Value{memberKey: present},
		})).
		AddBranchProof(proof).
		AddPathPresenceImplication(implication).
		AddChannelSelectFact(selectFact).
		FreezeTable(frozenID).
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
	if got := s.ReadDynamicIndexFact(reg, dynamicKey); !dynamicindex.Domain(reg).Equal(got, dynamicFact) {
		t.Fatalf("dynamic-index snapshot mutation changed state to %#v", got)
	}
	if got := s.DynamicIndexFactsSnapshot().Facts[dynamicKey]; !dynamicindex.Domain(reg).Equal(got, dynamicFact) {
		t.Fatalf("fresh dynamic-index snapshot = %#v, want original fact", got)
	}

	heapSnapshot := s.HeapTableObjectsSnapshot()
	if heapSnapshot.Top || len(heapSnapshot.Objects) != 1 {
		t.Fatalf("heap snapshot = %#v, want one finite object", heapSnapshot)
	}
	heapObject := heapSnapshot.Objects[heapID]
	heapMembers := heapObject.StaticMembers()
	heapMembers[memberKey] = absent
	heapSnapshot.Objects[heapID] = heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          absent,
		StaticMembers: heapMembers,
	})
	if got := s.ReadHeapTableObject(reg, heapID); !product.Equal(reg, got.Root(), present) {
		t.Fatalf("heap snapshot mutation changed state root to %#v", got.Root())
	}
	if got := s.HeapTableObjectsSnapshot().Objects[heapID]; !product.Equal(reg, got.Root(), present) {
		t.Fatalf("fresh heap snapshot root = %#v, want original", got.Root())
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

	implicationSnapshot := s.PathPresenceImplicationsSnapshot()
	if implicationSnapshot.Bottom || implicationSnapshot.Top || len(implicationSnapshot.Implications) != 1 {
		t.Fatalf("path-presence implication snapshot = %#v, want one finite implication", implicationSnapshot)
	}
	implicationSnapshot.Implications[0] = otherImplication
	if !s.HasPathPresenceImplication(implication) || s.HasPathPresenceImplication(otherImplication) {
		t.Fatalf("path-presence implication snapshot mutation changed state")
	}
	if got := s.PathPresenceImplicationsSnapshot().Implications[0]; got != implication {
		t.Fatalf("fresh path-presence implication snapshot = %#v, want original implication", got)
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

	frozenSnapshot := s.FrozenTablesSnapshot()
	if frozenSnapshot.Bottom || frozenSnapshot.Top || len(frozenSnapshot.Tables) != 1 {
		t.Fatalf("frozen-table snapshot = %#v, want one finite identity", frozenSnapshot)
	}
	frozenSnapshot.Tables[0] = otherFrozenID
	if !s.IsTableFrozen(frozenID) || s.IsTableFrozen(otherFrozenID) {
		t.Fatalf("frozen-table snapshot mutation changed state")
	}
	if got := s.FrozenTablesSnapshot().Tables[0]; got != frozenID {
		t.Fatalf("fresh frozen-table snapshot = %#v, want original identity", got)
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
	if topPaths.Bottom || !topPaths.Top || len(topPaths.Refinements) != 0 {
		t.Fatalf("top path snapshot = %#v, want top with no finite facts", topPaths)
	}
	topDynamic := top.DynamicIndexFactsSnapshot()
	if !topDynamic.Top || len(topDynamic.Facts) != 0 {
		t.Fatalf("top dynamic-index snapshot = %#v, want top with no finite facts", topDynamic)
	}
	topHeap := top.HeapTableObjectsSnapshot()
	if !topHeap.Top || len(topHeap.Objects) != 0 {
		t.Fatalf("top heap snapshot = %#v, want top with no finite objects", topHeap)
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
	topImplications := top.PathPresenceImplicationsSnapshot()
	if topImplications.Bottom || !topImplications.Top || len(topImplications.Implications) != 0 {
		t.Fatalf("top path-presence implication snapshot = %#v, want top with no finite facts", topImplications)
	}
	topChannel := top.ChannelSelectFactsSnapshot()
	if topChannel.Bottom || !topChannel.Top || len(topChannel.Facts) != 0 {
		t.Fatalf("top channel-select snapshot = %#v, want top with no finite facts", topChannel)
	}
	topFrozen := top.FrozenTablesSnapshot()
	if topFrozen.Bottom || !topFrozen.Top || len(topFrozen.Tables) != 0 {
		t.Fatalf("top frozen-table snapshot = %#v, want top with no finite facts", topFrozen)
	}

	bottomPaths := bottom.PathRefinementsSnapshot()
	if !bottomPaths.Bottom || bottomPaths.Top || len(bottomPaths.Refinements) != 0 {
		t.Fatalf("bottom path snapshot = %#v, want explicit bottom", bottomPaths)
	}
	bottomDynamic := bottom.DynamicIndexFactsSnapshot()
	if bottomDynamic.Top || len(bottomDynamic.Facts) != 0 {
		t.Fatalf("bottom dynamic-index snapshot = %#v, want finite empty pointwise lane", bottomDynamic)
	}
	bottomHeap := bottom.HeapTableObjectsSnapshot()
	if bottomHeap.Top || len(bottomHeap.Objects) != 0 {
		t.Fatalf("bottom heap snapshot = %#v, want finite empty pointwise lane", bottomHeap)
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
	bottomImplications := bottom.PathPresenceImplicationsSnapshot()
	if !bottomImplications.Bottom || bottomImplications.Top || len(bottomImplications.Implications) != 0 {
		t.Fatalf("bottom path-presence implication snapshot = %#v, want explicit bottom", bottomImplications)
	}
	bottomChannel := bottom.ChannelSelectFactsSnapshot()
	if !bottomChannel.Bottom || bottomChannel.Top || len(bottomChannel.Facts) != 0 {
		t.Fatalf("bottom channel-select snapshot = %#v, want explicit bottom", bottomChannel)
	}
	bottomFrozen := bottom.FrozenTablesSnapshot()
	if !bottomFrozen.Bottom || bottomFrozen.Top || len(bottomFrozen.Tables) != 0 {
		t.Fatalf("bottom frozen-table snapshot = %#v, want explicit bottom", bottomFrozen)
	}

	emptyPaths := empty.PathRefinementsSnapshot()
	if emptyPaths.Bottom || !emptyPaths.Top || len(emptyPaths.Refinements) != 0 {
		t.Fatalf("empty path snapshot = %#v, want reachable top/empty", emptyPaths)
	}
	emptyDynamic := empty.DynamicIndexFactsSnapshot()
	if emptyDynamic.Top || len(emptyDynamic.Facts) != 0 {
		t.Fatalf("empty dynamic-index snapshot = %#v, want finite empty pointwise lane", emptyDynamic)
	}
	emptyHeap := empty.HeapTableObjectsSnapshot()
	if emptyHeap.Top || len(emptyHeap.Objects) != 0 {
		t.Fatalf("empty heap snapshot = %#v, want finite empty pointwise lane", emptyHeap)
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
	emptyImplications := empty.PathPresenceImplicationsSnapshot()
	if emptyImplications.Bottom || !emptyImplications.Top || len(emptyImplications.Implications) != 0 {
		t.Fatalf("empty path-presence implication snapshot = %#v, want reachable top/empty", emptyImplications)
	}
	emptyChannel := empty.ChannelSelectFactsSnapshot()
	if emptyChannel.Bottom || !emptyChannel.Top || len(emptyChannel.Facts) != 0 {
		t.Fatalf("empty channel-select snapshot = %#v, want reachable top/empty", emptyChannel)
	}
	emptyFrozen := empty.FrozenTablesSnapshot()
	if emptyFrozen.Bottom || !emptyFrozen.Top || len(emptyFrozen.Tables) != 0 {
		t.Fatalf("empty frozen-table snapshot = %#v, want reachable top/empty", emptyFrozen)
	}
}
