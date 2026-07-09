package manifest

import (
	"encoding/json"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/module/signature"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// oracleRichOperationalEffects populates every OperationalEffects fact lane with
// at least one entry, and multiple entries where ordering matters, so the codec
// oracle exercises each lane's encode, decode, and canonical ordering.
func oracleRichOperationalEffects() signature.OperationalEffects {
	return signature.OperationalEffects{
		MaySuspend: true,
		ReturnPresenceRelations: []signature.ReturnPresenceRelation{
			{TriggerIndex: 1, TriggerPresence: presence.Present(), TargetIndex: 0, TargetPresence: presence.Absent()},
			{TriggerIndex: 0, TriggerPresence: presence.Maybe(), TargetIndex: 2, TargetPresence: presence.Present()},
		},
		NormalReturnPresenceRefinements: []signature.PathPresenceRefinement{
			{Path: pathdom.NewPlaceholder(0).Field("ready"), Presence: presence.Present()},
			{Path: pathdom.NewPlaceholder(0).Field("done"), Presence: presence.Absent()},
		},
		NormalReturnTypeRefinements: []signature.PathTypeRefinement{
			{Path: pathdom.NewPlaceholder(0), Type: typ.String, Assertion: assertion.Runtime()},
			{Path: pathdom.NewPlaceholder(1).Field("count"), Type: typ.Integer, Assertion: assertion.Top()},
		},
		PathPresenceImplications: []signature.PathPresenceImplication{
			{
				Trigger:         pathdom.NewPlaceholder(0).Field("status"),
				TriggerPresence: presence.Present(),
				TriggerType:     typ.String,
				HasTriggerType:  true,
				Target:          pathdom.Path{Root: "ret[0]"}.Field("value"),
				TargetPresence:  presence.Present(),
			},
			{
				Trigger:         pathdom.NewPlaceholder(1).Field("flag"),
				TriggerPresence: presence.Absent(),
				Target:          pathdom.NewPlaceholder(0).Field("value"),
				TargetPresence:  presence.Absent(),
			},
		},
		PathStaticMembers: []signature.PathStaticMemberFact{
			{Path: pathdom.NewPlaceholder(1).Field("kind"), Type: typ.String},
			{Path: pathdom.NewPlaceholder(1).Field("size"), Type: typ.Integer},
		},
		PathInvalidations: []signature.PathInvalidation{
			{Path: pathdom.NewPlaceholder(1).Field("items")},
			{Path: pathdom.NewPlaceholder(0).Field("cache")},
		},
		BranchProofs: []signature.BranchProof{
			{Kind: signature.BranchProofPathNotEqual, Path: pathdom.NewPlaceholder(0).Field("channel"), Other: pathdom.NewPlaceholder(1)},
			{Kind: signature.BranchProofPathPresence, Path: pathdom.NewPlaceholder(0).Field("opt"), Presence: presence.Present()},
		},
		DynamicIndexFacts: []signature.DynamicIndexFact{
			{
				Table:       pathdom.Path{Root: "ret[0]"},
				Site:        "example.returned.array",
				KeyPresence: presence.Present(),
				Key:         signature.DynamicIndexOperand{Type: typ.Integer},
				Value:       signature.DynamicIndexOperand{Type: typ.String},
				Admission:   signature.DynamicIndexAdmissionAdmitted,
			},
			{
				Table:       pathdom.NewPlaceholder(1),
				Site:        "example.param.map",
				KeyPresence: presence.Maybe(),
				Key:         signature.DynamicIndexOperand{Path: pathdom.NewPlaceholder(0).Field("key")},
				Value:       signature.DynamicIndexOperand{Type: typ.String},
				Admission:   signature.DynamicIndexAdmissionUnknown,
			},
		},
		KeyMemberships: []signature.KeyMembership{
			{Key: pathdom.NewPlaceholder(0).Field("key"), Table: pathdom.NewPlaceholder(1)},
			{Key: pathdom.NewPlaceholder(0).Field("id"), Table: pathdom.Path{Root: "ret[0]"}},
		},
		DynamicValueKeys: []signature.DynamicValueKeyMembership{
			{Container: pathdom.Path{Root: "ret[0]"}, Site: "example.returned.keys", Table: pathdom.NewPlaceholder(1)},
			{Container: pathdom.NewPlaceholder(0), Site: "example.param.keys", Table: pathdom.NewPlaceholder(1)},
		},
		FrozenTables: []signature.FrozenTable{
			{Target: pathdom.NewPlaceholder(0).Field("sealed")},
			{Target: pathdom.NewPlaceholder(1).Field("frozen")},
		},
		EscapeEvents: []signature.EscapeEvent{
			{Target: pathdom.NewPlaceholder(0).Field("payload"), Kind: signature.EscapeSend, Recursive: true},
			{Target: pathdom.NewPlaceholder(0).Field("payload"), Kind: signature.EscapeStore, Recursive: false},
		},
		StoreRelations: []signature.StoreRelation{
			{Source: pathdom.NewPlaceholder(0).Field("payload"), Into: pathdom.NewPlaceholder(1).Field("items")},
			{Source: pathdom.NewPlaceholder(0).Field("head"), Into: pathdom.NewPlaceholder(1).Field("tail")},
		},
		ParamRelations: []signature.ParamRelation{
			{
				Param:                1,
				EscapeClass:          signature.EscapeStore,
				PlacementConsequence: signature.PlacementConsequenceOwnedHeap,
				ThroughReturn:        true,
				StoredInto:           0,
				HasStoredInto:        true,
			},
			{
				Param:                0,
				EscapeClass:          signature.EscapeNone,
				PlacementConsequence: signature.PlacementConsequenceKeep,
			},
		},
		ReturnFlows: []signature.ReturnFlow{
			{ReturnIndex: 1, Kind: signature.ReturnFlowParam, Param: 0},
			{
				ReturnIndex: 0,
				Kind:        signature.ReturnFlowParamMember,
				Param:       1,
				Path:        []segment.Segment{{Kind: segment.SegmentField, Name: "meta"}},
			},
		},
		LifecycleEffects: []signature.LifecycleEffect{
			{
				Target:     pathdom.NewPlaceholder(0).Field("tx"),
				Kind:       signature.LifecycleAcquire,
				Protocol:   typestate.Protocol("transaction"),
				To:         typestate.State("active"),
				Obligation: typestate.Obligation{Final: typestate.State("finished")},
			},
			{
				Target:   pathdom.NewPlaceholder(0).Field("tx"),
				Kind:     signature.LifecycleTransition,
				Protocol: typestate.Protocol("transaction"),
				From:     typestate.State("active"),
				To:       typestate.State("finished"),
			},
		},
		ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
			ReturnIndex: 0,
			Root:        "example.transform:return:0:root",
			Objects: []signature.AllocationObjectTemplate{
				{
					ID:           "example.transform:return:0:root",
					Type:         typetable.NewRecord().Build(),
					PrefixStable: true,
					StaticMembers: []signature.AllocationStaticMemberTemplate{{
						Suffix: []segment.Segment{{Kind: segment.SegmentField, Name: "child"}},
						Value:  "example.transform:return:0:root.child",
					}},
					DynamicEntries: []signature.AllocationDynamicEntryTemplate{{
						KeyType: typ.String,
						Value:   "example.transform:return:0:root.entry",
					}},
				},
				{ID: "example.transform:return:0:root.child", Type: typetable.NewRecord().Build()},
				{ID: "example.transform:return:0:root.entry", Type: typetable.NewRecord().Build()},
			},
		}},
	}
}

// oracleSingleLaneCases returns one OperationalEffects per lane, populated only
// on that lane, so a byte divergence pinpoints the offending lane.
func oracleSingleLaneCases(rich signature.OperationalEffects) []struct {
	name string
	e    signature.OperationalEffects
} {
	return []struct {
		name string
		e    signature.OperationalEffects
	}{
		{"MaySuspend", signature.OperationalEffects{MaySuspend: true}},
		{"ReturnPresenceRelations", signature.OperationalEffects{ReturnPresenceRelations: rich.ReturnPresenceRelations}},
		{"NormalReturnPresenceRefinements", signature.OperationalEffects{NormalReturnPresenceRefinements: rich.NormalReturnPresenceRefinements}},
		{"NormalReturnTypeRefinements", signature.OperationalEffects{NormalReturnTypeRefinements: rich.NormalReturnTypeRefinements}},
		{"PathPresenceImplications", signature.OperationalEffects{PathPresenceImplications: rich.PathPresenceImplications}},
		{"PathStaticMembers", signature.OperationalEffects{PathStaticMembers: rich.PathStaticMembers}},
		{"PathInvalidations", signature.OperationalEffects{PathInvalidations: rich.PathInvalidations}},
		{"BranchProofs", signature.OperationalEffects{BranchProofs: rich.BranchProofs}},
		{"DynamicIndexFacts", signature.OperationalEffects{DynamicIndexFacts: rich.DynamicIndexFacts}},
		{"KeyMemberships", signature.OperationalEffects{KeyMemberships: rich.KeyMemberships}},
		{"DynamicValueKeys", signature.OperationalEffects{DynamicValueKeys: rich.DynamicValueKeys}},
		{"FrozenTables", signature.OperationalEffects{FrozenTables: rich.FrozenTables}},
		{"EscapeEvents", signature.OperationalEffects{EscapeEvents: rich.EscapeEvents}},
		{"StoreRelations", signature.OperationalEffects{StoreRelations: rich.StoreRelations}},
		{"ParamRelations", signature.OperationalEffects{ParamRelations: rich.ParamRelations}},
		{"ReturnFlows", signature.OperationalEffects{ReturnFlows: rich.ReturnFlows}},
		{"LifecycleEffects", signature.OperationalEffects{LifecycleEffects: rich.LifecycleEffects}},
		{"ReturnAllocationTemplates", signature.OperationalEffects{ReturnAllocationTemplates: rich.ReturnAllocationTemplates}},
	}
}

// reverseOperationalEffectSlices reverses every lane slice in place so a permuted
// input can be checked for canonical-order invariance.
func reverseOperationalEffectSlices(e *signature.OperationalEffects) {
	reverseSlice(e.ReturnPresenceRelations)
	reverseSlice(e.NormalReturnPresenceRefinements)
	reverseSlice(e.NormalReturnTypeRefinements)
	reverseSlice(e.PathPresenceImplications)
	reverseSlice(e.PathStaticMembers)
	reverseSlice(e.PathInvalidations)
	reverseSlice(e.BranchProofs)
	reverseSlice(e.DynamicIndexFacts)
	reverseSlice(e.KeyMemberships)
	reverseSlice(e.DynamicValueKeys)
	reverseSlice(e.FrozenTables)
	reverseSlice(e.EscapeEvents)
	reverseSlice(e.StoreRelations)
	reverseSlice(e.ParamRelations)
	reverseSlice(e.ReturnFlows)
	reverseSlice(e.LifecycleEffects)
	reverseSlice(e.ReturnAllocationTemplates)
}

func reverseSlice[T any](s []T) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

func mustMarshalWire(t *testing.T, w *operationalEffectsWire) []byte {
	t.Helper()
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal wire: %v", err)
	}
	return data
}

// TestOperationalEffectsDescriptorCodecRoundTrips is the descriptor codec oracle.
// Across a fully-populated corpus, single-lane isolation, permuted input order,
// empty, and nil inputs it asserts the descriptor-driven wire codec round-trips
// facts (decode of the encoded wire is Equal to the source) and produces stable,
// permutation-invariant canonical wire bytes.
func TestOperationalEffectsDescriptorCodecRoundTrips(t *testing.T) {
	rich := oracleRichOperationalEffects()

	reversed := rich.Clone()
	reverseOperationalEffectSlices(&reversed)

	cases := []struct {
		name string
		e    *signature.OperationalEffects
	}{
		{"rich", &rich},
		{"reversed", &reversed},
		{"empty", &signature.OperationalEffects{}},
		{"nil", nil},
	}
	for _, sc := range oracleSingleLaneCases(rich) {
		e := sc.e
		cases = append(cases, struct {
			name string
			e    *signature.OperationalEffects
		}{"single/" + sc.name, &e})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := encodeOperationalEffects(tc.e)
			if err != nil {
				t.Fatalf("encodeOperationalEffects: %v", err)
			}
			bytesFirst := mustMarshalWire(t, wire)

			facts, err := decodeOperationalEffects(wire)
			if err != nil {
				t.Fatalf("decodeOperationalEffects: %v", err)
			}

			// Round-trip stability: re-encoding the decoded facts reproduces
			// the same canonical wire bytes.
			reWire, err := encodeOperationalEffects(&facts)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			if got := mustMarshalWire(t, reWire); string(got) != string(bytesFirst) {
				t.Fatalf("round-trip bytes differ:\nfirst:  %s\nsecond: %s", bytesFirst, got)
			}

			// Decoding the re-encoded wire yields Equal facts.
			reFacts, err := decodeOperationalEffects(reWire)
			if err != nil {
				t.Fatalf("re-decode: %v", err)
			}
			if !facts.Equals(reFacts) {
				t.Fatalf("decoded facts differ across round-trip:\nfirst:  %#v\nsecond: %#v", facts, reFacts)
			}
		})
	}

	// Permutation invariance: reversed input canonicalizes to the same bytes.
	richWire, err := encodeOperationalEffects(&rich)
	if err != nil {
		t.Fatalf("encode rich: %v", err)
	}
	reversedWire, err := encodeOperationalEffects(&reversed)
	if err != nil {
		t.Fatalf("encode reversed: %v", err)
	}
	if a, b := mustMarshalWire(t, richWire), mustMarshalWire(t, reversedWire); string(a) != string(b) {
		t.Fatalf("permutation not canonicalized:\nrich:     %s\nreversed: %s", a, b)
	}
}
