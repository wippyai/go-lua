package callboundary

import (
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

// richNormalReturnFactsCorpus builds NormalReturnFacts populating every storage
// lane with a mix of matching, non-matching, and dual-path facts, so the parity
// oracle exercises append, path filtering, and path dropping on every lane.
func richNormalReturnFactsCorpus() (facts NormalReturnFacts, match, other pathdom.Path) {
	match = pathdom.Path{Root: "ret[0]"}.Field("value")
	other = pathdom.Path{Root: "ret[1]"}.Field("value")
	facts = NormalReturnFacts{
		PathRefinements:          []PathValueFact{{Path: match}, {Path: other}},
		PersistentPathWrites:     []PathValueFact{{Path: match}, {Path: other}},
		PathStaticMembers:        []PathStaticMemberFact{{Path: match}, {Path: other}},
		PathInvalidations:        []PathInvalidationFact{{Path: match}, {Path: other}},
		DynamicIndexFacts:        []DynamicIndexFact{{Table: match}, {Table: other}},
		KeyMemberships:           []KeyMembershipFact{{Key: other, Table: match}, {Key: other, Table: other}},
		DynamicValueKeys:         []DynamicValueKeyMembershipFact{{Container: match, Table: other}, {Container: other, Table: other}},
		DynamicAllValues:         []DynamicAllValueKeyMembershipFact{{Container: other, Table: match}, {Container: other, Table: other}},
		BranchProofs:             []BranchProof{{Path: other, Other: match}, {Path: other, Other: other}},
		PathPresenceImplications: []PathPresenceImplicationFact{{Trigger: match, Target: other}, {Trigger: other, Target: other}},
		ChannelSelects:           []ChannelSelectFact{{Result: match}, {Result: other}},
		FrozenTables:             []FrozenTableFact{{Target: match}, {Target: other}},
		EffectDeltas:             []EffectDelta{{Target: match}, {Target: other}},
		EscapeEvents:             []EscapeEventFact{{Target: match}, {Target: other}},
		StoreRelations:           []StoreRelationFact{{Source: other, Into: match}, {Source: other, Into: other}},
		LifecycleFacts:           []LifecycleFact{{Target: match}, {Target: other}},
		NumFloors:                []NumFloorFact{{Path: match}, {Path: other}},
		RelConstraints: []RelConstraintFact{
			{A: RelOperand{Path: other}, B: RelOperand{Path: match}, C: RelOperand{Path: other}},
			{A: RelOperand{Path: other}, C: RelOperand{Path: other}},
		},
	}
	return facts, match, other
}

// TestNormalReturnFactDescriptorsDeriveLiveLanes proves the descriptor
// table derives lanes that are structurally identical (id, field name, and
// path-filter participation) to the live storage lane registry, in the
// same order.
func TestNormalReturnFactDescriptorsDeriveLiveLanes(t *testing.T) {
	derived := derivedNormalReturnFactLanes()
	hand := NormalReturnFactLanes()
	if len(derived) != len(hand) {
		t.Fatalf("derived lanes = %d, want live = %d", len(derived), len(hand))
	}
	for i := range hand {
		if derived[i].ID() != hand[i].ID() {
			t.Fatalf("lane[%d] id = %q, want %q", i, derived[i].ID(), hand[i].ID())
		}
		if derived[i].FieldName() != hand[i].FieldName() {
			t.Fatalf("lane[%d] field = %q, want %q", i, derived[i].FieldName(), hand[i].FieldName())
		}
		if derived[i].FiltersByPath() != hand[i].FiltersByPath() {
			t.Fatalf("lane[%d] filtersByPath = %v, want %v", i, derived[i].FiltersByPath(), hand[i].FiltersByPath())
		}
	}
}

func appendDerivedNormalReturnFacts(f, other NormalReturnFacts) NormalReturnFacts {
	for _, lane := range derivedNormalReturnFactLanes() {
		f = lane.Append(f, other)
	}
	return f
}

// TestNormalReturnFactDescriptorsAppendParity proves descriptor-derived Append
// produces a struct identical to the public NormalReturnFacts.Append over the
// rich corpus, including the empty-side fast paths.
func TestNormalReturnFactDescriptorsAppendParity(t *testing.T) {
	left, _, _ := richNormalReturnFactsCorpus()
	right, _, _ := richNormalReturnFactsCorpus()

	cases := []struct {
		name        string
		left, right NormalReturnFacts
	}{
		{"both-rich", left, right},
		{"left-empty", NormalReturnFacts{}, right},
		{"right-empty", left, NormalReturnFacts{}},
		{"both-empty", NormalReturnFacts{}, NormalReturnFacts{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.left.Append(tc.right)
			got := appendDerivedNormalReturnFacts(tc.left, tc.right)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("descriptor append mismatch:\n got=%#v\nwant=%#v", got, want)
			}
		})
	}
}

func filterDerivedNormalReturnFacts(f NormalReturnFacts, keep NormalReturnPathPredicate) NormalReturnFacts {
	if f.Empty() || keep == nil {
		return NormalReturnFacts{}
	}
	var out NormalReturnFacts
	for _, lane := range derivedNormalReturnFactLanes() {
		lane.filter(&out, f, keep)
	}
	return out
}

// TestNormalReturnFactDescriptorsFilterParity proves descriptor-derived
// FilterPaths is byte-identical to the public FilterPaths across match/other/all
// predicates.
func TestNormalReturnFactDescriptorsFilterParity(t *testing.T) {
	facts, match, other := richNormalReturnFactsCorpus()
	predicates := map[string]NormalReturnPathPredicate{
		"match": func(p pathdom.Path) bool { return p.Equal(match) },
		"other": func(p pathdom.Path) bool { return p.Equal(other) },
		"all":   func(pathdom.Path) bool { return true },
		"none":  func(pathdom.Path) bool { return false },
	}
	for name, keep := range predicates {
		t.Run(name, func(t *testing.T) {
			want := facts.FilterPaths(keep)
			got := filterDerivedNormalReturnFacts(facts, keep)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("descriptor filter mismatch:\n got=%#v\nwant=%#v", got, want)
			}
		})
	}
}

func dropDerivedNormalReturnFacts(f NormalReturnFacts, shouldDrop NormalReturnPathPredicate) NormalReturnFacts {
	if f.Empty() || shouldDrop == nil {
		return f
	}
	var out NormalReturnFacts
	for _, lane := range derivedNormalReturnFactLanes() {
		lane.drop(&out, f, shouldDrop)
	}
	return out
}

// TestNormalReturnFactDescriptorsDropParity proves descriptor-derived
// DropFactsTouchingPaths is byte-identical to the public method.
func TestNormalReturnFactDescriptorsDropParity(t *testing.T) {
	facts, match, other := richNormalReturnFactsCorpus()
	predicates := map[string]NormalReturnPathPredicate{
		"drop-match": func(p pathdom.Path) bool { return p.Equal(match) },
		"drop-other": func(p pathdom.Path) bool { return p.Equal(other) },
		"drop-all":   func(pathdom.Path) bool { return true },
		"drop-none":  func(pathdom.Path) bool { return false },
	}
	for name, drop := range predicates {
		t.Run(name, func(t *testing.T) {
			want := facts.DropFactsTouchingPaths(drop)
			got := dropDerivedNormalReturnFacts(facts, drop)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("descriptor drop mismatch:\n got=%#v\nwant=%#v", got, want)
			}
		})
	}
}

// TestNormalReturnFactDescriptorsWireRefsMatchLowering pins the manifest wire
// lane cross-reference for every kind against the effectlowering ground truth:
// lanes lowered from a signature.OperationalEffects wire field carry that
// field; local-only lanes carry none.
func TestNormalReturnFactDescriptorsWireRefsMatchLowering(t *testing.T) {
	want := map[NormalReturnFactLaneID][]string{
		LanePathRefinements:          {"NormalReturnPresenceRefinements", "NormalReturnTypeRefinements"},
		LanePersistentPathWrites:     nil,
		LanePathStaticMembers:        {"PathStaticMembers"},
		LanePathPresenceImplications: {"PathPresenceImplications"},
		LanePathInvalidations:        {"PathInvalidations"},
		LaneDynamicIndexFacts:        {"DynamicIndexFacts"},
		LaneKeyMemberships:           {"KeyMemberships"},
		LaneDynamicValueKeys:         {"DynamicValueKeys"},
		LaneDynamicAllValues:         nil,
		LaneBranchProofs:             {"BranchProofs"},
		LaneChannelSelects:           nil,
		LaneFrozenTables:             {"FrozenTables"},
		LaneEffectDeltas:             nil,
		LaneEscapeEvents:             {"EscapeEvents"},
		LaneStoreRelations:           {"StoreRelations"},
		LaneLifecycleFacts:           {"LifecycleEffects"},
		LaneNumFloors:                nil,
		LaneRelConstraints:           nil,
	}
	descriptors := NormalReturnFactDescriptors()
	if len(descriptors) != len(want) {
		t.Fatalf("descriptor count = %d, want %d", len(descriptors), len(want))
	}
	for _, d := range descriptors {
		id := NormalReturnFactLaneID(d.Kind)
		expected, ok := want[id]
		if !ok {
			t.Fatalf("descriptor kind %q has no expected wire ref", d.Kind)
		}
		if !reflect.DeepEqual(d.WireRef, expected) {
			t.Fatalf("kind %q wire ref = %#v, want %#v", d.Kind, d.WireRef, expected)
		}
	}
}
